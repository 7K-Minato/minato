# SFTP file access

minato can give operators direct file access to a game server's world volume
over SFTP. The feature is opt-in per **GameProfile**:

```yaml
spec:
  capabilities:
    sftp: true
```

Every GameServer of that profile then gets an SFTP sidecar, a credentials
Secret, a service port on the player-facing LoadBalancer, and an `sftp`
status endpoint. Connection info is retrievable via the control plane API.

## Architecture

```
                 ┌──────────────────────────── pod ───────────────────────────┐
                 │  minato-game   minato-agent   minato-sftp (sftpgo)         │
                 │       │             │               │                      │
                 │       └─────────────┴───── data (PVC, storage.mountPath)   │
                 └──────────────────────────────┬─────────────────────────────┘
                                                │
   sftp -P 2022 minato@<host>  ◄──  Service <name>-game (type LoadBalancer)
                                        ports: game..., sftp: 2022/TCP
```

- **Sidecar**: `drakkan/sftpgo:v2.6.6-alpine`, injected by the operator into
  the game server pod. It mounts the *same* world PVC as the game container at
  the profile's `storage.mountPath` (the SFTP home directory) and serves SFTP
  on port **2022**. The HTTP admin UI and telemetry are disabled.
- **PSS restricted**: the sidecar runs as UID/GID 1000 with
  `runAsNonRoot: true`, `allowPrivilegeEscalation: false`, all capabilities
  dropped, `seccompProfile: RuntimeDefault`, and a read-only root filesystem.
  It needs no root and no `NET_BIND_SERVICE` (port 2022 > 1024). The pod gets
  `fsGroup: 1000` with `fsGroupChangePolicy: OnRootMismatch` so the non-root
  sidecar can write its config dir and access the volume.
- **Credentials**: per-server Secret `minato-<server>-sftp` in the server
  namespace (owner-referenced to the GameServer, garbage-collected with it).
  Keys: `username` (always `minato`), `password` (32 random alphanumeric
  characters), `users.json` (SFTPGo user template).
- **Exposure**: port 2022/TCP is added to the existing per-server
  `<name>-game` LoadBalancer service — the same service, loadBalancerIP
  pinning and external-dns hostname apply. No second LoadBalancer is created.
- **Status**: `status.endpoints` gains an `sftp` entry with the same address
  as the game endpoint (external-dns hostname when configured, otherwise the
  LB ingress IP) and port 2022.

## Retrieving connection info

```
GET /api/v1/gameservers/{namespace}/{name}/sftp
```

```json
{
  "host": "mc-1.games.example.com",
  "port": 2022,
  "username": "minato",
  "password": "…"
}
```

Requires the **operator** role or higher (same as the console endpoint).
Namespace-scoped API keys only work within their namespaces. Every request is
recorded by the audit middleware; credentials are never logged. Returns 404
when the server does not exist, the profile does not enable SFTP, or the
credentials/endpoint are not provisioned yet.

Go SDK: `client.GetSFTPInfo(ctx, namespace, name)`.

## Credential rotation

The operator **never rotates** the password on reconcile: if the Secret
exists, it is reused. To rotate manually:

```sh
kubectl delete secret minato-<server>-sftp -n <namespace>
```

The next reconcile recreates the Secret with a fresh password; restart the
pod (or let the next rollout pick it up) for the sidecar to use it.

## Security posture

- SFTP runs on the player-facing LoadBalancer. Restrict access with your load
  balancer / firewall (`loadBalancerSourceRanges` or cloud security groups)
  if players and admins share the service.
- Passwords are 192-bit random, alphanumeric-only, stored in a
  namespace-scoped Secret. The control plane ClusterRole can read Secrets —
  treat API access to the `sftp` endpoint as credential disclosure (it is
  audit-logged).
- The sidecar cannot write its own root filesystem and runs unprivileged.
  World files created by the game process keep the game container's UID: when
  the game runs as UID 1000 (e.g. itzg images) the SFTP user has full
  read/write; otherwise the sidecar has group-level access (fsGroup 1000) and
  some game-owned subtrees may be read-only depending on the game's umask.

## Follow-ups (not in v1)

- GameServer-level override (`spec.capabilities.sftp: false`) to disable SFTP
  for individual servers of an SFTP-enabled profile.
- `capabilities.files` (filebrowser) is intentionally not implemented; SFTP
  is the only file-access capability.

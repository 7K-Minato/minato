# Security Baseline

Minato is designed with enterprise security in mind from day one.

## Pod Security Standards

All Minato components run under the `restricted` Pod Security Standard:

- **runAsNonRoot**: All containers run as non-root user (UID 65532)
- **readOnlyRootFilesystem**: Root filesystem is read-only where possible
- **drop ALL capabilities**: No Linux capabilities are granted
- **seccompProfile: RuntimeDefault**: Uses the runtime default seccomp profile
- **allowPrivilegeEscalation: false**: Prevents privilege escalation

## Network Security

### Inter-Component Communication

All inter-component traffic should be encrypted:

- Operator → Agent: mTLS via cert-manager-issued certificates (Phase 2)
- Control Plane → Operator: Via Kubernetes API with TLS
- Client → Control Plane: HTTPS/TLS

### Network Policies

Default NetworkPolicy:

- Ingress: Only from within the same namespace
- Egress: Allowed to Kubernetes API (443, 6443)
- Game server ports are exposed via Services with proper selectors

## Authentication and Authorization

### Control Plane Authentication

The control plane uses Kubernetes tokens for authentication:

```text
Authorization: Bearer <k8s-token>
```

Tokens are validated via TokenReview API.

### RBAC

Three tenant roles are provided:

- `minato:tenant-viewer`: Read-only access
- `minato:tenant-operator`: Can manage GameServers and execute actions
- `minato:tenant-admin`: Full namespace access including NetworkPolicies

## Secrets Management

### RCON Passwords

RCON passwords should be stored as Kubernetes Secrets and mounted as environment variables:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: minecraft-rcon
type: Opaque
stringData:
  RCON_PASSWORD: "secure-password-here"
```

### External Secrets Operator

For production deployments, integrate with External Secrets Operator:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: minecraft-rcon
spec:
  secretStoreRef:
    name: vault-backend
    kind: SecretStore
  target:
    name: minecraft-rcon
  data:
    - secretKey: RCON_PASSWORD
      remoteRef:
        key: secret/data/minecraft/rcon
        property: password
```

## Image Security

### Image Signing

All official Minato images are signed with cosign:

```bash
cosign verify --certificate-oidc-issuer https://token.actions.githubusercontent.com harbor.7kgroup.org/7kminato/minato-operator:v1.0.0
```

### Admission Policy

Use Kyverno or OPA Gatekeeper to enforce image signing:

```yaml
apiVersion: kyverno.io/v1
kind: ClusterPolicy
metadata:
  name: verify-minato-images
spec:
  validationFailureAction: Enforce
  rules:
    - name: verify-signature
      match:
        resources:
          kinds:
            - Pod
      validate:
        message: "Minato images must be signed"
        pattern:
          spec:
            containers:
              - name: "*"
                image: "harbor.7kgroup.org/7kminato/minato-*"
```

## API Key Rotation Runbook

Control plane API keys are stored as Kubernetes Secrets (`minato-apikey-*`)
and support optional `expiresAt` and namespace scoping (see
[multi-tenancy.md](multi-tenancy.md#namespace-scoped-api-keys)). Rotate keys
regularly and whenever a consumer is decommissioned or a key may have leaked.

### Standard rotation (create → update consumers → delete)

```bash
# 1. Create the replacement key (mirror the old key's role/scope/expiry)
curl -X POST https://minato.example.com/api/v1/apikeys \
  -H "X-API-Key: $ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"name":"ci-cd-2026-09","role":"operator","namespaces":["tenant-alpha"],"expiresAt":"2026-12-01T00:00:00Z"}'
#    → store the returned "key" value; it is shown only once.

# 2. Update every consumer (CI secrets, minato-cloud tenant config, registrars)
#    to use the NEW key value.

# 3. Verify consumers work with the new key, then revoke the old one.
curl -X DELETE https://minato.example.com/api/v1/apikeys/ci-cd-2026-06 \
  -H "X-API-Key: $ADMIN_KEY"
```

Every create/delete is recorded as an `apikey.created` / `apikey.deleted`
audit event with the key name and the acting user (never the key material).

### Dual-key overlap pattern

For consumers that cannot tolerate an outage during rotation, run both keys
in parallel:

1. Create the new key (step 1 above).
2. Roll out the new key to consumers gradually; the old key keeps working.
3. Once all consumers are migrated, delete the old key.

Use short `expiresAt` values as a safety net so a forgotten old key expires
on its own instead of living forever.

### Out of scope here: registrar → minato-cloud token rotation

The registrar authenticates to **minato-cloud** (sibling repo) with
`REGISTER_TOKEN`. Graceful dual-token rotation of that token is implemented
and documented on the cloud side; this repo's control plane does not consume
`REGISTER_TOKEN`. Control-plane-facing credentials (basic auth, OIDC, API
keys) are covered by the sections above.

## Audit Logging

Every state-changing operation is logged:

```json
{
  "timestamp": "2026-05-27T10:00:00Z",
  "level": "info",
  "caller": "user@example.com",
  "action": "actionexecution.create",
  "target": {
    "kind": "GameServer",
    "namespace": "minato",
    "name": "minecraft-server-1"
  },
  "result": "success"
}
```

## Compliance Mapping

| Control | Implementation |
| --------- | --------------- |
| SOC 2 CC6.1 | Logical access controls via RBAC |
| SOC 2 CC6.2 | Authentication via Kubernetes tokens |
| SOC 2 CC6.3 | Authorization via RBAC roles |
| SOC 2 CC7.1 | Monitoring via Prometheus metrics |
| SOC 2 CC7.2 | Audit logging of all operations |
| ISO 27001 A.9.1.1 | Access control policy via RBAC |
| ISO 27001 A.9.4.1 | Information access restriction |
| ISO 27001 A.12.3.1 | Information backup via snapshots |

## Security Checklist

- [ ] All pods run as non-root
- [ ] NetworkPolicies are enabled
- [ ] ResourceQuotas are configured per tenant
- [ ] Image signatures are verified
- [ ] Secrets are stored in Kubernetes Secrets or External Secrets
- [ ] Audit logging is enabled
- [ ] Prometheus metrics are secured
- [ ] TLS is enabled for all external communication
- [ ] RBAC roles are configured for tenants
- [ ] Pod Security Standards are enforced

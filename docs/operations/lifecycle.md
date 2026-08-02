# Server lifecycle and fleet writes

The control plane exposes lifecycle control for individual GameServers and
write endpoints for GameServerFleets. All endpoints below are
namespace-scoped: callers with namespace-restricted API keys can only use
them inside their allowed namespaces.

## GameServer lifecycle: `PATCH /api/v1/gameservers/{namespace}/{name}`

Strict merge-patch for lifecycle control. Only two fields are accepted;
anything else is rejected with `400`:

```json
{ "spec": { "lifecycle": { "autoStart": false, "idleTimeoutSeconds": 300 } } }
```

| Field | Role | Effect |
|---|---|---|
| `spec.lifecycle.autoStart` | operator, admin | `false` gracefully stops a running server; `true` starts a stopped server |
| `spec.lifecycle.idleTimeoutSeconds` | operator, admin | Seconds of player-idle time before auto-shutdown (`0` = never) |

`spec.lifecycle.autoStart` defaults to `true` when unset — pre-existing
GameServers that never set the field keep running.

### Stop/start semantics (operator reconcile)

- **`autoStart: false` on a running server**: the operator calls the pod
  agent's `PrepareShutdown` gRPC (save world, notify players), then scales
  the StatefulSet to 0 and reports `status.state: Stopped`. If the agent is
  unreachable or errors, the scale-down still proceeds (logged, plus a
  Kubernetes event).
- **`autoStart: true` on a stopped server**: the StatefulSet is scaled back
  to 1 and the state moves through `Provisioning` to `Running`.

### Suspension (minato-cloud dunning)

minato-cloud suspends servers for dunning (unpaid invoices) by calling this
endpoint with `{"spec":{"lifecycle":{"autoStart":false}}}` — this is the real
data-plane stop, not just a flag. Unsuspension patches `autoStart: true`.
The PVC and all world data are retained; only the pod is stopped.

## Fleet writes

| Endpoint | Role | Purpose |
|---|---|---|
| `POST /api/v1/gameserverfleets/{namespace}` | admin | Create a fleet; namespace is auto-created and hardened like GameServer creation. The referenced GameProfile must exist, `replicas >= 0` |
| `PATCH /api/v1/gameserverfleets/{namespace}/{name}` | operator, admin | Strict merge-patch: only `spec.replicas` (`>= 0`) is accepted |
| `DELETE /api/v1/gameserverfleets/{namespace}/{name}` | admin | Delete the fleet and all of its GameServers |

### Graceful drain on scale-down

When a fleet scales down (or cleans up on deletion), the operator calls the
agent's `PrepareShutdown` gRPC on each running GameServer before deleting it.
The per-server drain budget is
`spec.updateStrategy.rollingUpdate.drainTimeoutSeconds` (default 30s); on
timeout or agent error the deletion proceeds and a warning event
(`AgentShutdownFailed` / `AgentDialFailed`) is emitted on the GameServer.

## SDK (sdk-go)

```go
c.UpdateGameServerLifecycle(ctx, ns, name, ptr.To(false), nil) // suspend
c.UpdateGameServerLifecycle(ctx, ns, name, ptr.To(true), nil)  // resume
c.CreateGameServerFleet(ctx, ns, fleet)
c.ScaleGameServerFleet(ctx, ns, name, 5)
c.DeleteGameServerFleet(ctx, ns, name)
```

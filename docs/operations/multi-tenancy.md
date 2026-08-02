# Multi-Tenancy

Minato supports multi-tenant deployments where each tenant operates in their own namespace.

## Model

- **One namespace = one tenant**
- GameProfiles are cluster-scoped (shared catalog)
- GameServers, Fleets, ActionExecutions, and Snapshots are namespace-scoped
- RBAC controls what each tenant can do
- ResourceQuotas and LimitRanges enforce resource limits per tenant
- NetworkPolicies isolate tenant traffic

## Automatic Tenant Provisioning

When the control plane receives `POST /api/v1/gameservers/{namespace}` for a
namespace that does not exist, it provisions the namespace with hardening
applied **at creation time only**:

1. **Labels**: `minato.io/tenant: <namespace>`,
   `app.kubernetes.io/managed-by: minato-controlplane`, and restricted Pod
   Security Standards labels (`pod-security.kubernetes.io/{enforce,audit,warn}: restricted`,
   mirroring the chart's `security.podSecurityStandards` values).
2. **ResourceQuota** `minato-tenant-quota`, driven by control plane env vars:

   | Env var | Default | Quota key | Empty/`0` |
   | ------- | ------- | --------- | --------- |
   | `TENANT_QUOTA_MAX_SERVERS` | `10` | `count/gameservers.operator.minato.io` | skip constraint |
   | `TENANT_QUOTA_CPU` | `8` | `requests.cpu` | skip constraint |
   | `TENANT_QUOTA_MEMORY` | `32Gi` | `requests.memory` | skip constraint |
   | `TENANT_QUOTA_STORAGE` | `200Gi` | `requests.storage` | skip constraint |

   If all constraints are disabled, no quota is created. Invalid values fail
   control plane startup with a clear error.

3. **NetworkPolicy** `minato-tenant-default` (default-deny):

   - **Ingress**: allowed from pods in the same namespace; the control plane
     namespace (`TENANT_CONTROLPLANE_NAMESPACE`, default `minato`) may reach
     the agent gRPC port (TCP 9876); the configured game ports
     (`TENANT_GAME_PORTS`, comma-separated, default `25565`, TCP+UDP) are open
     to **any source** — game traffic arrives directly from the internet via
     NodePort/LoadBalancer Services, and NetworkPolicies cannot distinguish
     "internet" from "other namespaces" on allowed ports. Cross-tenant
     pod-to-pod traffic on all other ports is denied.
   - **Egress**: DNS (53 TCP/UDP), the agent port, and the game ports. A game
     needing broader egress (updates, master server lists) must be granted it
     by the operator via an additional GitOps-managed policy.

**Creation-time-only semantics**: if the namespace already exists, the control
plane does not reconcile or overwrite labels, quotas, or policies. Operators
manage existing namespaces via GitOps; changing the env vars only affects
newly created tenant namespaces.

## Tenant Roles

### minato:tenant-viewer

Read-only access to all Minato resources in their namespace.

### minato:tenant-operator

Can create and manage GameServers and ActionExecutions.
Cannot modify GameProfiles (platform-managed).

### minato:tenant-admin

Full access within their namespace, including managing NetworkPolicies.

## Platform Admin

ClusterRole `minato:platform-admin` for managing GameProfiles cluster-wide.

All four roles ship as ClusterRole manifests in `config/rbac/tenant_roles.yaml`
and in the Helm chart (`chart/templates/tenant-rbac.yaml`, rendered when
`rbac.create=true`). `minato:tenant-operator` aggregates
`minato:tenant-viewer`, and `minato:tenant-admin` aggregates
`minato:tenant-operator`, via ClusterRole aggregation. They are intended for
human access via `kubectl` (bound per tenant namespace with RoleBindings);
the control plane and operator do not use them.

## Setting Up a Tenant

New tenant namespaces are provisioned automatically by the control plane (see
above). The manual steps below remain valid for pre-creating namespaces
yourself — namespaces created this way are NOT modified by the control plane.

```bash
# Create namespace
kubectl create namespace tenant-alpha

# Apply ResourceQuota
kubectl apply -f - <<EOF
apiVersion: v1
kind: ResourceQuota
metadata:
  name: tenant-alpha-quota
  namespace: tenant-alpha
spec:
  hard:
    requests.cpu: "10"
    requests.memory: 40Gi
    limits.cpu: "20"
    limits.memory: 80Gi
    persistentvolumeclaims: "10"
    services.loadbalancers: "2"
EOF

# Apply LimitRange
kubectl apply -f - <<EOF
apiVersion: v1
kind: LimitRange
metadata:
  name: tenant-alpha-limits
  namespace: tenant-alpha
spec:
  limits:
  - default:
      cpu: "1"
      memory: 4Gi
    defaultRequest:
      cpu: 100m
      memory: 256Mi
    type: Container
EOF

# Apply NetworkPolicy (default deny with game port exceptions)
kubectl apply -f - <<EOF
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: tenant-alpha-default
  namespace: tenant-alpha
spec:
  podSelector: {}
  policyTypes:
  - Ingress
  - Egress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: tenant-alpha
  - ports:
    - protocol: TCP
      port: 25565
    - protocol: UDP
      port: 25565
  egress:
  - {}
EOF

# Bind tenant-operator role
kubectl create rolebinding tenant-alpha-operator \
  --clusterrole=minato:tenant-operator \
  --user=tenant-alpha@example.com \
  --namespace=tenant-alpha
```

## Tenant Isolation

- **Namespace isolation**: Standard Kubernetes namespace boundaries
- **Network isolation**: NetworkPolicies block cross-tenant pod traffic
- **Resource isolation**: ResourceQuotas prevent one tenant from consuming all cluster resources
- **RBAC isolation**: Roles restrict what tenants can see and modify

## Cross-Tenant Visibility

By default, tenants cannot:

- List GameServers in other namespaces
- View ActionExecutions in other namespaces
- Access agent gRPC endpoints in other namespaces

## Namespace-Scoped API Keys

Control plane API keys can be restricted to a set of namespaces so that
automation for one tenant cannot read or modify another tenant's servers —
the API-level counterpart of the RBAC isolation above.

### How it works

- Each API key Secret (`minato-apikey-*`, see
  `internal/controlplane/auth/storage.go`) carries an optional `namespaces`
  data field: a comma-separated list of exact namespace names or
  trailing-`*` prefix globs (e.g. `tenant-a,tenant-*`).
- **Empty/absent = cluster-wide** (the previous behavior; appropriate for
  platform admin keys).
- Enforcement:
  - Routes with a `{namespace}` path segment (GameServer/Fleet CRUD, actions,
    snapshots, console) return **403** when the key's patterns don't match.
  - List-across-namespaces endpoints (`GET /api/v1/gameservers`,
    `GET /api/v1/gameserverfleets`) **filter** their results to matching
    namespaces instead of rejecting.
  - GameProfiles are cluster-scoped and always readable.
  - The `/api/v1/apikeys` admin endpoints require a **cluster-wide** key, so
    a tenant-scoped key cannot mint or revoke keys.
- Keys may also carry an optional `expiresAt` (RFC 3339); expired keys are
  rejected with **401**. See the rotation runbook in
  [security.md](security.md#api-key-rotation-runbook).

### Creating a scoped key

```bash
curl -X POST https://minato.example.com/api/v1/apikeys \
  -H "X-API-Key: $ADMIN_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "tenant-alpha-ci",
    "role": "operator",
    "namespaces": ["tenant-alpha"],
    "expiresAt": "2026-09-01T00:00:00Z"
  }'
```

Operators can also scope an existing key by editing its Secret directly —
the top-level `namespaces` / `expiresAt` data fields take precedence over the
embedded metadata JSON:

```bash
kubectl -n minato patch secret minato-apikey-tenant-alpha-ci --type=merge \
  -p '{"stringData":{"namespaces":"tenant-alpha"}}'
```

### minato-cloud integration note

`minato-cloud` (sibling repo) currently calls the control plane with a single
cluster-wide admin API key. The intended end state is **one tenant-scoped key
per tenant** (e.g. `namespaces: ["tenant-<id>"]`, role `operator`, with
`expiresAt` set and rotated per the runbook), so a leaked tenant credential
cannot affect other tenants and list endpoints naturally scope to that
tenant's namespace. Until cloud-side support lands, the single admin key
keeps working unchanged (cluster-wide keys are unaffected by scoping).
Dual-token grace for registrar→cloud `REGISTER_TOKEN` rotation is a
minato-cloud concern and intentionally out of scope for this repo.

## Future Enhancements (Phase 2)

- Tenant-managed GameProfiles
- Cross-tenant GameServer migration
- Shared game world hosting

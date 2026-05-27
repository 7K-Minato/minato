# Multi-Tenancy

Minato supports multi-tenant deployments where each tenant operates in their own namespace.

## Model

- **One namespace = one tenant**
- GameProfiles are cluster-scoped (shared catalog)
- GameServers, Fleets, ActionExecutions, and Snapshots are namespace-scoped
- RBAC controls what each tenant can do
- ResourceQuotas and LimitRanges enforce resource limits per tenant
- NetworkPolicies isolate tenant traffic

## Tenant Roles

### minato:tenant-viewer

Read-only access to all Minato resources in their namespace.

### minato:tenant-operator

Can create and manage GameServers and ActionExecutions.
Cannot modify GameProfiles (platform-managed).

### minato:tenant-admin

Full access within their namespace, including managing NetworkPolicies and ServiceMonitors.

## Platform Admin

ClusterRole `minato:platform-admin` (to be created) for managing GameProfiles cluster-wide.

## Setting Up a Tenant

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

## Future Enhancements (Phase 2)

- Tenant-managed GameProfiles
- Cross-tenant GameServer migration
- Shared game world hosting

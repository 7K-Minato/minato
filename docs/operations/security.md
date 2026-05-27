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

```
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
cosign verify --key minato.pub ghcr.io/7k-group/minato-operator:v1.0.0
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
                image: "ghcr.io/7k-group/minato-*"
```

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
|---------|---------------|
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

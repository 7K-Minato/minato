# Compliance Documentation

This document maps Minato controls to common compliance frameworks.

## SOC 2

### CC6.1 - Logical and Physical Access Controls

**Minato Implementation:**
- RBAC-based access control for all resources
- Namespace isolation for multi-tenant deployments
- NetworkPolicies restrict pod-to-pod communication

**Evidence:**
- `config/rbac/tenant/` contains tenant role definitions
- `docs/operations/multi-tenancy.md` documents isolation model

### CC6.2 - Prior to Access

**Minato Implementation:**
- Kubernetes token-based authentication
- TokenReview API validates all requests
- OIDC integration supported via Kubernetes

### CC6.3 - During Access

**Minato Implementation:**
- RBAC roles enforce least privilege
- ServiceAccounts with minimal permissions
- Regular access reviews via Kubernetes audit logs

### CC7.1 - System Operations Monitoring

**Minato Implementation:**
- Prometheus metrics for all components
- Agent metrics exposed on `/metrics` endpoints for scraping
- Standard metrics: `minato_operator_reconciliations_total`, `minato_gameservers`, etc.

### CC7.2 - System Operations Evaluation

**Minato Implementation:**
- Structured audit logging
- ActionExecution resources record all actions
- Kubernetes Events for state changes

## ISO 27001

### A.9.1.1 - Access Control Policy

**Minato Implementation:**
- Documented RBAC policy in `config/rbac/`
- Three-tier tenant model (viewer, operator, admin)
- Platform admin role for cluster-wide management

### A.9.4.1 - Information Access Restriction

**Minato Implementation:**
- GameProfiles are cluster-scoped but read-only for tenants
- GameServers are namespace-scoped
- Cross-tenant access blocked by RBAC and NetworkPolicies

### A.12.3.1 - Information Backup

**Minato Implementation:**
- GameSnapshot CRD for declarative backups
- VolumeSnapshot integration
- Retention policies (count + duration based)

### A.12.4.1 - Event Logging

**Minato Implementation:**
- All API operations logged
- ActionExecution audit trail
- Kubernetes Events for resource lifecycle

## Data Residency

Minato stores all data in Kubernetes:
- CRDs stored in etcd
- Game data in PVCs (location depends on storage class)
- Backups in VolumeSnapshots (location depends on CSI driver)

To ensure data residency:
1. Use storage classes backed by local storage
2. Configure VolumeSnapshot classes with local targets
3. Backup etcd to compliant storage

## Encryption

### Encryption at Rest

- etcd encryption: Enable Kubernetes etcd encryption provider
- PVC encryption: Use storage class with encryption (e.g., LUKS, cloud provider encryption)
- Backup encryption: VolumeSnapshot encryption depends on CSI driver

### Encryption in Transit

- Agent gRPC: mTLS via cert-manager (Phase 2)
- Control Plane API: HTTPS/TLS
- Kubernetes API: TLS (always enabled)

## Key Management

### RCON Passwords

- Stored in Kubernetes Secrets
- Can be integrated with External Secrets Operator
- Rotation via Secret updates + GameServer restart

### TLS Certificates

- cert-manager integration for webhook certs
- Automatic renewal
- CA managed by cert-manager or external PKI

## Audit Requirements

### What is Logged

All state-changing operations:
- GameServer creation/deletion/updates
- ActionExecution creation and completion
- GameSnapshot creation and deletion
- Fleet scaling operations

### Log Format

```json
{
  "timestamp": "2026-05-27T10:00:00Z",
  "level": "info",
  "component": "controlplane",
  "request_id": "uuid",
  "user": "user@example.com",
  "action": "gameserver.create",
  "resource": {
    "kind": "GameServer",
    "namespace": "minato",
    "name": "minecraft-server-1"
  },
  "result": "success"
}
```

### Log Retention

- Kubernetes audit logs: Configured via cluster audit policy
- Application logs: Use log aggregation (e.g., Fluent Bit → Loki/Elasticsearch)
- ActionExecution resources: TTL cleanup (7 days success, 30 days failure)

# Installation Guide

This guide covers installing Minato on a Kubernetes cluster.

## Prerequisites

- Kubernetes 1.28+ cluster
- kubectl configured to communicate with your cluster
- Helm 3.12+ (for Helm installation)
- VolumeSnapshot CRD installed (for backup functionality)
- Prometheus Operator (optional, for agent metrics scraping)
- Gateway API CRDs (optional, for external ingress)

## Quick Start

### Option 1: Helm Installation (Recommended)

```bash
# Add the Minato Helm repository
helm repo add minato https://7k-group.github.io/minato
helm repo update

# Install with default values
helm install minato minato/minato \
  --namespace minato-system \
  --create-namespace

# Verify installation
kubectl get pods -n minato-system
```

### Option 2: Kustomize Installation

```bash
# Clone the repository
git clone https://github.com/7k-group/minato.git
cd minato

# Install CRDs
make install

# Deploy operator
make deploy

# Verify
kubectl get pods -n minato-system
```

### Option 3: Manual Installation

```bash
# Build binaries
make build

# Install CRDs
kubectl apply -f config/crd/bases/

# Deploy operator
kubectl apply -f config/manager/
kubectl apply -f config/rbac/
```

## Configuration

### Helm Values

Key configuration options:

```yaml
# values.yaml
operator:
  replicas: 2  # High availability
  resources:
    requests:
      cpu: 100m
      memory: 128Mi
    limits:
      cpu: 500m
      memory: 256Mi

controlPlane:
  enabled: true
  ingress:
    enabled: true
    host: api.minato.example.com

# External resources managed by separate chart
monitoring:
  enabled: true
  serviceMonitor:
    enabled: true
    # Creates ServiceMonitor for agent metrics
    # Agent exposes /metrics on port 9090
```

Install with custom values:

```bash
helm install minato minato/minato -f values.yaml
```

### Namespace Isolation

For multi-tenant deployments:

```bash
# Create tenant namespace
kubectl create namespace tenant-a

# Apply RBAC for tenant
kubectl apply -f config/rbac/tenant-role.yaml -n tenant-a
```

## Post-Installation Verification

### Check Operator Status

```bash
# Check operator pods
kubectl get pods -n minato-system -l app.kubernetes.io/name=minato

# Check operator logs
kubectl logs -n minato-system -l app.kubernetes.io/name=minato -f

# Check CRDs
kubectl get crds | grep minato
```

### Create a Test GameServer

```bash
# Apply Minecraft profile
kubectl apply -f profiles/minecraft-paper/profile.yaml

# Create a GameServer
kubectl apply -f - <<EOF
apiVersion: operator.minato.io/v1
kind: GameServer
metadata:
  name: test-server
  namespace: default
spec:
  profile: minecraft-paper
  env:
    EULA: "true"
EOF

# Check status
kubectl get gameserver test-server -n default
kubectl describe gameserver test-server -n default
```

### Verify Resources Created

```bash
# Check StatefulSet
kubectl get statefulset test-server -n default

# Check Service
kubectl get service test-server -n default

# Check PVC
kubectl get pvc test-server -n default

# Check agent metrics (if agent is running)
kubectl port-forward pod/test-server-0 9090:9090 -n default
curl http://localhost:9090/metrics
```

## Upgrading

### Helm Upgrade

```bash
# Update repository
helm repo update

# Upgrade release
helm upgrade minato minato/minato \
  --namespace minato-system \
  --values values.yaml
```

### Manual Upgrade

```bash
# Pull latest changes
git pull origin main

# Rebuild and redeploy
make build
make docker-build IMG=minato:v0.2.0
make deploy IMG=minato:v0.2.0
```

## Uninstallation

### Helm

```bash
helm uninstall minato -n minato-system
kubectl delete namespace minato-system
```

### Manual

```bash
make undeploy
make uninstall
```

## Troubleshooting

### Operator Not Starting

```bash
# Check events
kubectl get events -n minato-system --sort-by='.lastTimestamp'

# Check pod logs
kubectl logs -n minato-system deployment/minato-operator -f
```

### GameServer Stuck in Provisioning

```bash
# Check StatefulSet
kubectl describe statefulset test-server -n default

# Check PVC binding
kubectl describe pvc test-server -n default

# Check events
kubectl get events -n default --field-selector involvedObject.name=test-server
```

### VolumeSnapshot Issues

Ensure the VolumeSnapshot CRD is installed:

```bash
kubectl get crd volumesnapshots.snapshot.storage.k8s.io
```

If not installed, install the CSI snapshotter:

```bash
kubectl apply -f https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/master/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
```

## Next Steps

- [Configuration Reference](configuration.md)
- [Troubleshooting Guide](troubleshooting.md)
- [Agent Development](../agent-developers/quickstart.md)

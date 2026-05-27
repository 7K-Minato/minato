# Minato (南)

Minato is a Kubernetes-native platform for hosting persistent, multi-game dedicated game servers, designed for enterprise use cases: hosting providers running many games for many tenants, and operators running large fleets of persistent worlds for a single game.

## Features

- **Persistent-first**: One GameServer = one StatefulSet = one PVC = one stable identity
- **Game-agnostic operator**: All game knowledge lives in agents, not the operator
- **Multi-game support**: Minecraft Paper, CS2, Palworld (extensible via SDK)
- **Fleet management**: GameServerFleet for managing N servers of the same game
- **Action dispatch**: Execute actions (restart, save, kick, etc.) via CRDs or API
- **Console streaming**: Real-time console access via WebSocket
- **Observability**: Prometheus ServiceMonitor integration, standard metric schema
- **Snapshots**: Declarative backup with retention policies
- **Multi-tenancy**: Namespace isolation with RBAC roles
- **Enterprise-ready**: PSS:restricted, non-root, HA, leader election

## Architecture

```
┌─────────────────┐     ┌──────────────────┐     ┌─────────────────┐
│   Control Plane  │────▶│   Minato Operator │────▶│   Game Servers  │
│   (HTTP/gRPC)    │     │   (controller)    │     │   (StatefulSet) │
└─────────────────┘     └──────────────────┘     └─────────────────┘
                                │                          │
                                ▼                          ▼
                       ┌──────────────────┐     ┌─────────────────┐
                       │   Kubernetes     │     │   Per-Game      │
                       │   API Server     │     │   Agents        │
                       └──────────────────┘     └─────────────────┘
```

## Quick Start

### Prerequisites

- Kubernetes 1.28+ cluster
- kubectl configured
- Go 1.22+ (for building from source)

### Install CRDs

```bash
git clone https://github.com/7k-group/minato.git
cd minato
export PATH=$PATH:$HOME/go/bin
make install
```

### Deploy Operator

```bash
# Build and deploy operator
make build
./bin/operator --leader-elect=false

# Or use Helm
helm install minato ./deploy/helm/minato
```

### Create a Minecraft Server

```bash
# Apply the Minecraft Paper profile
kubectl apply -f profiles/minecraft-paper/profile.yaml

# Create a game server
kubectl apply -f profiles/minecraft-paper/gameserver-example.yaml

# Check status
kubectl get gameserver minecraft-server-1 -n minato
```

### Use the CLI

```bash
# Build CLI
make build

# List servers
./bin/minato-ctl server list

# Execute an action
./bin/minato-ctl server action minecraft-server-1 save-world

# Open console
./bin/minato-ctl console minecraft-server-1
```

## CRDs

### GameProfile (Cluster-scoped)

Defines a reusable game configuration:

```yaml
apiVersion: operator.minato.io/v1
kind: GameProfile
metadata:
  name: minecraft-paper
spec:
  displayName: "Minecraft Paper"
  image: "itzg/minecraft-server:latest"
  ports:
    - name: game
      containerPort: 25565
  storage:
    mountPath: /data
    sizeDefault: 10Gi
  agent:
    image: "ghcr.io/7k-group/minato-agent-minecraft:v1.0.0"
```

### GameServer (Namespace-scoped)

A single running game server instance:

```yaml
apiVersion: operator.minato.io/v1
kind: GameServer
metadata:
  name: my-server
  namespace: minato
spec:
  profile: minecraft-paper
  env:
    EULA: "true"
```

### GameServerFleet (Namespace-scoped)

Manages N GameServers of the same profile:

```yaml
apiVersion: operator.minato.io/v1
kind: GameServerFleet
metadata:
  name: production-fleet
spec:
  profile: minecraft-paper
  replicas: 5
```

### ActionExecution (Namespace-scoped)

Audit trail for action execution:

```yaml
apiVersion: operator.minato.io/v1
kind: ActionExecution
metadata:
  name: restart-action
spec:
  targetRef:
    kind: GameServer
    name: my-server
  actionName: restart
```

### GameSnapshot (Namespace-scoped)

Declarative backups:

```yaml
apiVersion: operator.minato.io/v1
kind: GameSnapshot
metadata:
  name: daily-backup
spec:
  gameServerRef: my-server
  schedule: "0 2 * * *"
  retention:
    count: 7
    duration: "168h"
```

## Supported Games

| Game | Profile | Agent | Status |
|------|---------|-------|--------|
| Minecraft Paper | ✅ | ✅ | Production-ready |
| Counter-Strike 2 | ✅ | 🚧 | Profile ready, agent stub |
| Palworld | ✅ | 🚧 | Profile ready, agent stub |
| Generic (YAML actions) | ✅ | ✅ | Production-ready |

## Development

```bash
# Run tests
make test

# Run integration tests
make test-integration

# Generate code
make generate

# Generate manifests
make manifests

# Run operator locally
make run-operator
```

## Documentation

- [Architecture Overview](docs/architecture/overview.md)
- [Metrics Schema](docs/operations/metrics-schema.md)
- [Multi-Tenancy](docs/operations/multi-tenancy.md)
- [Security](docs/operations/security.md)
- [CLI](docs/operations/cli.md)
- [Runbooks](docs/operations/runbooks/)
- [Compliance](docs/operations/compliance/)
- [Agent Quickstart](docs/agent-developers/quickstart.md)
- [SDK Reference](docs/agent-developers/sdk-reference.md)

## Helm Installation

```bash
helm repo add minato https://7k-group.github.io/minato
helm install minato minato/minato \
  --set operator.replicas=2 \
  --set controlPlane.enabled=true
```

## Enterprise Features

- ✅ High Availability: Leader election, multi-replica operator
- ✅ Security: PSS:restricted, non-root, no privileged containers
- ✅ RBAC: Three-tier tenant roles
- ✅ Audit: ActionExecution audit trail
- ✅ Monitoring: Prometheus metrics, ServiceMonitor support
- ✅ Snapshots: VolumeSnapshot integration
- ✅ Network Isolation: NetworkPolicies
- ✅ Resource Quotas: Per-tenant limits

## Milestones

- [x] Milestone 1: Project Scaffold, Core CRDs, and Agent Contract
- [x] Milestone 2: Operator Reconciler and Agent Sidecar Injection
- [x] Milestone 3: Agent SDK and Generic Agent
- [x] Milestone 4: Action Dispatch from Operator
- [x] Milestone 5: Observability (ServiceMonitor, Metrics)
- [x] Milestone 6: Three Real Agents (Minecraft, CS2, Palworld)
- [x] Milestone 7: GameServerFleet and Multi-Tenancy
- [x] Milestone 8: Lifecycle, Snapshots, Control Plane API
- [x] Milestone 9: Console Streaming and CLI
- [x] Milestone 10: Enterprise Hardening and Release

## License

Licensed under the Apache License, Version 2.0.

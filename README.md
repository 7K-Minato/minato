# Minato (南)

[![Build Status](https://img.shields.io/github/actions/workflow/status/7k-group/minato/ci.yml?branch=main)](https://github.com/7k-minato/minato/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/7k-minato/minato)](https://goreportcard.com/report/github.com/7k-minato/minato)
[![Coverage](https://img.shields.io/badge/coverage-80%25-brightgreen)](https://github.com/7k-minato/minato)
[![Go Version](https://img.shields.io/badge/go-1.23-blue)](https://golang.org)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

Minato is a Kubernetes-native platform for hosting persistent, multi-game dedicated game servers, designed for enterprise use cases: hosting providers running many games for many tenants, and operators running large fleets of persistent worlds for a single game.

## Features

- **Persistent-first**: One GameServer = one StatefulSet = one PVC = one stable identity
- **Game-agnostic operator**: All game knowledge lives in agents, not the operator
- **Multi-game support**: Minecraft Paper, CS2, Palworld (extensible via SDK)
- **Fleet management**: GameServerFleet for managing N servers of the same game
- **Action dispatch**: Execute actions (restart, save, kick, etc.) via CRDs or API
- **Console streaming**: Real-time console access via WebSocket
- **Agent Metrics**: Agents expose `/metrics` endpoints for Prometheus scraping
- **Snapshots**: Declarative backup with retention policies using VolumeSnapshots
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

The operator reconciles CRDs into Kubernetes resources (StatefulSets, PVCs, Services). Per-game agents run as sidecars and implement the gRPC agent API. The control plane provides an HTTP API for user-facing operations.

**Operator Scope**: The operator manages only minato-native resources (GameServer, GameProfile, GameServerFleet, ActionExecution, GameSnapshot). External resources like ServiceMonitors, Gateways, and Ingress are managed by a separate Helm chart.

## Quick Start

### Prerequisites

- Kubernetes 1.28+ cluster
- kubectl configured
- Go 1.23+ (for building from source)
- VolumeSnapshot CRD (for backups)

### Install with Helm

```bash
helm repo add minato https://7k-group.github.io/minato
helm repo update
helm install minato minato/minato \
  --namespace minato-system \
  --create-namespace
```

### Install from Source

```bash
git clone https://github.com/7k-minato/minato.git
cd minato
make install
make deploy
```

### Create a Minecraft Server

Game profiles are maintained in the [minato-games](https://github.com/7k-minato/minato-games) repository.

```bash
# Clone the games repository and apply the Minecraft Paper profile
git clone https://github.com/7k-minato/minato-games.git
cd minato-games
kubectl apply -f profiles/minecraft-paper/profile.yaml

# Create a game server
kubectl apply -f - <<EOF
apiVersion: operator.minato.io/v1
kind: GameServer
metadata:
  name: minecraft-server-1
  namespace: default
spec:
  profile: minecraft-paper
  env:
    EULA: "true"
EOF

# Check status
kubectl get gameserver minecraft-server-1 -n default
```

### Use the Control Plane API

```bash
# List servers
curl http://localhost:8080/api/v1/gameservers

# Execute an action
curl -X POST http://localhost:8080/api/v1/gameservers/default/minecraft-server-1/actions/save-world

# List snapshots
curl http://localhost:8080/api/v1/gameservers/default/minecraft-server-1/snapshots
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
    image: "harbor.7kgroup.com/minato-games/minato-agent-minecraft:v0.1.0"
```

### GameServer (Namespace-scoped)

A single running game server instance:

```yaml
apiVersion: operator.minato.io/v1
kind: GameServer
metadata:
  name: my-server
  namespace: default
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

Declarative backups with retention:

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
| Counter-Strike 2 | ✅ | ✅ | Production-ready |
| Palworld | ✅ | ✅ | Production-ready |
| Generic (YAML actions) | ✅ | ✅ | Production-ready |

## Documentation

- [Installation Guide](docs/operations/installation.md)
- [Configuration Reference](docs/operations/configuration.md)
- [Troubleshooting Guide](docs/operations/troubleshooting.md)
- [Architecture Overview](docs/architecture/overview.md)
- [Controller Flow](docs/architecture/controller-flow.md)
- [Metrics Schema](docs/operations/metrics-schema.md)
- [Multi-Tenancy](docs/operations/multi-tenancy.md)
- [Security](docs/operations/security.md)
- [CLI](docs/operations/cli.md)
- [Runbooks](docs/operations/runbooks/)
- [Compliance](docs/operations/compliance/)
- [Agent Quickstart](docs/agent-developers/quickstart.md)
- [SDK Reference](docs/agent-developers/sdk-reference.md)

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

### Project Structure

```
minato/
├── api/                    # CRD Go types and protobuf definitions
├── cmd/                    # Binaries (operator, control plane, agents, CLI)
├── internal/controllers/   # Operator reconcilers
├── sdk/agent/              # Public Agent SDK
├── deploy/helm/            # Helm chart
├── deploy/helm/            # Helm chart
├── config/                 # Kustomize configs, CRDs, RBAC
└── docs/                   # Documentation
```

## Enterprise Features

- ✅ **High Availability**: Leader election, multi-replica operator
- ✅ **Security**: PSS:restricted, non-root, no privileged containers
- ✅ **RBAC**: Three-tier tenant roles
- ✅ **Audit**: ActionExecution audit trail
- ✅ **Monitoring**: Agent metrics endpoints for Prometheus (external ServiceMonitor chart)
- ✅ **Snapshots**: VolumeSnapshot integration with retention
- ✅ **Network Isolation**: NetworkPolicies
- ✅ **Resource Quotas**: Per-tenant limits

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## Security

For security concerns, please see [SECURITY.md](SECURITY.md).

## License

Licensed under the Apache License, Version 2.0.

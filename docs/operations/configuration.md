# Configuration Reference

This document provides a comprehensive reference for configuring Minato resources.

## GameProfile

A `GameProfile` defines a reusable game configuration.

### Full Example

```yaml
apiVersion: operator.minato.io/v1
kind: GameProfile
metadata:
  name: minecraft-paper
spec:
  displayName: "Minecraft Paper"
  image: "itzg/minecraft-server:latest"
  imagePullPolicy: IfNotPresent
  
  ports:
    - name: game
      containerPort: 25565
      protocol: TCP
    - name: rcon
      containerPort: 25575
      protocol: TCP
  
  environment:
    - key: EULA
      default: "true"
      required: true
    - key: TYPE
      default: "PAPER"
      required: false
    - key: MEMORY
      default: "2G"
      required: false
  
  resources:
    requests:
      cpu: 500m
      memory: 2Gi
    limits:
      cpu: 2
      memory: 4Gi
  
  storage:
    mountPath: /data
    sizeDefault: 20Gi
  
  agent:
    image: "ghcr.io/7k-group/minato-agent-minecraft:v0.1.0"
    version: "0.1.0"
  
  actions:
    - name: restart
      description: "Gracefully restart the server"
      concurrency: serialize
      timeout: 5m
    - name: send-message
      description: "Broadcast a message"
      params:
        message:
          type: string
          required: true
      concurrency: allow
      timeout: 30s
  
  capabilities:
    files: true
    sftp: true
    backup: true
    restoreFromSnapshot: true
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `displayName` | string | Yes | Human-friendly name |
| `image` | string | Yes | Game container image |
| `imagePullPolicy` | string | No | Image pull policy (Always/IfNotPresent/Never) |
| `ports` | []PortSpec | No | Game ports to expose |
| `environment` | []EnvironmentSpec | No | Configurable environment variables |
| `resources` | ResourceRequirements | No | Default container resources |
| `storage` | StorageSpec | Yes | Persistent storage configuration |
| `agent` | AgentSpec | Yes | Per-game agent configuration |
| `actions` | []ActionDecl | No | Declared action catalog |
| `capabilities` | CapabilitiesSpec | No | Optional sidecar capabilities |

### CapabilitiesSpec

```yaml
capabilities:
  files: true      # Filebrowser sidecar
  sftp: true       # SFTP sidecar
  backup: true     # Supports backup action
  restoreFromSnapshot: true
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `files` | bool | No | Inject filebrowser sidecar |
| `sftp` | bool | No | Inject SFTP sidecar |
| `backup` | bool | No | Enable backup action |
| `restoreFromSnapshot` | bool | No | Enable restore from snapshot |

## GameServer

### PortSpec

```yaml
ports:
  - name: game
    containerPort: 25565
    protocol: TCP
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Port name |
| `containerPort` | int32 | Yes | Container port |
| `protocol` | string | No | TCP or UDP (default: TCP) |

### EnvironmentSpec

```yaml
environment:
  - key: EULA
    default: "true"
    required: true
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `key` | string | Yes | Environment variable name |
| `default` | string | No | Default value |
| `required` | bool | No | Whether user must provide a value |

### StorageSpec

```yaml
storage:
  mountPath: /data
  sizeDefault: 20Gi
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `mountPath` | string | Yes | Volume mount path |
| `sizeDefault` | string | Yes | Default PVC size |

### AgentSpec

```yaml
agent:
  image: "ghcr.io/7k-group/minato-agent-minecraft:v0.1.0"
  version: "0.1.0"
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `image` | string | Yes | Agent container image |
| `version` | string | Yes | Agent version for compatibility |

### ActionDecl

```yaml
actions:
  - name: restart
    description: "Restart the server"
    params:
      delay:
        type: int
        required: false
        default: "0"
    concurrency: serialize
    timeout: 5m
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Yes | Action identifier |
| `description` | string | No | Human-readable description |
| `params` | map[string]ActionParamSchema | No | Parameter schema |
| `concurrency` | string | No | allow/serialize/exclusive |
| `timeout` | string | No | Maximum duration |

### CapabilitiesSpec

```yaml
capabilities:
  files: true      # Filebrowser sidecar
  sftp: true       # SFTP sidecar
  backup: true     # Supports backup action
  restoreFromSnapshot: true
```

## GameServer

A `GameServer` represents a single running game server instance.

### Full Example

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
    MEMORY: "4G"
  lifecycle:
    idleTimeoutSeconds: 3600
    autoStart: true
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `profile` | string | Yes | GameProfile name |
| `env` | map[string]string | No | Environment overrides |
| `lifecycle` | LifecycleSpec | No | Lifecycle settings |

### LifecycleSpec

```yaml
lifecycle:
  idleTimeoutSeconds: 3600  # 0 = never auto-shutdown
  autoStart: true
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `idleTimeoutSeconds` | int32 | No | Auto-shutdown timeout (0 = disabled) |
| `autoStart` | bool | No | Start automatically on creation |

## GameServerFleet

A `GameServerFleet` manages multiple GameServers of the same profile.

### Full Example

```yaml
apiVersion: operator.minato.io/v1
kind: GameServerFleet
metadata:
  name: production-fleet
  namespace: default
spec:
  profile: minecraft-paper
  replicas: 5
  template:
    metadata:
      labels:
        tier: production
    spec:
      env:
        EULA: "true"
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `profile` | string | Yes | GameProfile name |
| `replicas` | int32 | Yes | Number of GameServers |
| `template` | GameServerTemplateSpec | No | Template for created GameServers |
| `updateStrategy` | UpdateStrategy | No | Update strategy |

### UpdateStrategy

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `type` | string | No | RollingUpdate or OnDelete |
| `rollingUpdate` | RollingUpdateConfig | No | Rolling update parameters |

## ActionExecution

An `ActionExecution` dispatches an action to a game server.

### Example

```yaml
apiVersion: operator.minato.io/v1
kind: ActionExecution
metadata:
  name: restart-action
  namespace: default
spec:
  targetRef:
    apiVersion: operator.minato.io/v1
    kind: GameServer
    name: my-server
  actionName: restart
  params:
    delay: "10"
  caller: "admin"
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `targetRef` | TargetRef | Yes | Target GameServer or Fleet |
| `actionName` | string | Yes | Action to execute |
| `params` | map[string]string | No | Action parameters |
| `caller` | string | No | Identity that initiated the action |

## GameSnapshot

A `GameSnapshot` manages backups of game server data.

### Example

```yaml
apiVersion: operator.minato.io/v1
kind: GameSnapshot
metadata:
  name: daily-backup
  namespace: default
spec:
  gameServerRef: my-server
  schedule: "0 2 * * *"
  retention:
    count: 7
    duration: "168h"
```

### Field Reference

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `gameServerRef` | string | Yes | GameServer name |
| `schedule` | string | No | Cron expression for periodic snapshots |
| `retention` | SnapshotRetention | No | Retention policy |

### SnapshotRetention

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `count` | int | No | Maximum number of snapshots |
| `duration` | string | No | Maximum age (e.g., "168h") |

## Environment Variables

### Operator

| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_BIND_ADDRESS` | `0` | Metrics endpoint address |
| `HEALTH_PROBE_BIND_ADDRESS` | `:8081` | Health probe address |
| `LEADER_ELECT` | `true` | Enable leader election |

### Agent

| Variable | Description |
|----------|-------------|
| `MINATO_GAMESERVER_NAME` | GameServer name |
| `MINATO_GAMESERVER_NAMESPACE` | GameServer namespace |
| `MINATO_GAME_CONTAINER` | Game container name |
| `RCON_HOST` | RCON server host |
| `RCON_PORT` | RCON server port |
| `RCON_PASSWORD` | RCON password |

### Control Plane

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `8080` | HTTP server port |

## Common Configurations

### Minecraft Paper

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
    - name: rcon
      containerPort: 25575
  environment:
    - key: EULA
      default: "true"
      required: true
    - key: TYPE
      default: "PAPER"
    - key: VERSION
      default: "1.20.4"
    - key: MEMORY
      default: "2G"
    - key: ENABLE_RCON
      default: "true"
    - key: RCON_PORT
      default: "25575"
    - key: RCON_PASSWORD
      default: "minato-rcon"
  storage:
    mountPath: /data
    sizeDefault: 20Gi
  agent:
    image: "ghcr.io/7k-group/minato-agent-minecraft:v0.1.0"
    version: "0.1.0"
```

### Counter-Strike 2

```yaml
apiVersion: operator.minato.io/v1
kind: GameProfile
metadata:
  name: cs2
spec:
  displayName: "Counter-Strike 2"
  image: "cm2network/cs2:latest"
  ports:
    - name: game
      containerPort: 27015
      protocol: UDP
    - name: rcon
      containerPort: 27015
      protocol: TCP
  environment:
    - key: SRCDS_TOKEN
      required: true
    - key: SRCDS_RCONPW
      required: true
  storage:
    mountPath: /home/steam/cs2-dedicated
    sizeDefault: 50Gi
  agent:
    image: "ghcr.io/7k-group/minato-agent-cs2:v0.1.0"
    version: "0.1.0"
```

### Palworld

```yaml
apiVersion: operator.minato.io/v1
kind: GameProfile
metadata:
  name: palworld
spec:
  displayName: "Palworld"
  image: "thijsvanloef/palworld-server-docker:latest"
  ports:
    - name: game
      containerPort: 8211
      protocol: UDP
    - name: rcon
      containerPort: 25575
      protocol: TCP
  environment:
    - key: PLAYERS
      default: "32"
    - key: PORT
      default: "8211"
    - key: RCON_ENABLED
      default: "true"
    - key: RCON_PORT
      default: "25575"
  storage:
    mountPath: /palworld
    sizeDefault: 20Gi
  agent:
    image: "ghcr.io/7k-group/minato-agent-palworld:v0.1.0"
    version: "0.1.0"
```

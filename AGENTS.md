````
You are helping me build "minato" (南) — a Kubernetes-native platform for hosting persistent, multi-game dedicated game servers, designed for enterprise use cases: hosting providers running many games for many tenants, and operators running large fleets of persistent worlds for a single game.

## Architecture overview

minato is composed of four parts:

1. **minato Operator** (Go, controller-runtime): owns the CRDs. Reconciles them into Kubernetes resources (StatefulSets, PVCs, Services, ServiceMonitors, Secrets). **Game-agnostic** — never imports game-specific code. Highly available, leader-elected.

2. **minato Agent SDK** (Go module, public and versioned): the contract and toolkit for writing per-game agents. Defines the gRPC interface every agent implements. Provides reusable components (RCON client library, lifecycle hooks, metrics scaffolding, action interpreter for declarative cases). Third parties can use this SDK to write agents for their own games.

3. **Per-game agents** (separate container images, one per game): sidecars injected into every game server pod. Implement the agent gRPC API. Encapsulate all per-game knowledge — RCON dialects, command syntax, output parsing, lifecycle quirks. minato ships agents for Minecraft, CS2, Palworld; others can be contributed.

4. **minato Control Plane** (Go HTTP/gRPC service): the user-facing API for action execution, console streaming, and aggregated operations. Validates requests, enforces auth/RBAC, audits everything, dispatches to agents via the agent gRPC API.

## Core design principles

- **Persistent-first**: designed for long-lived, stateful game worlds. One GameServer = one StatefulSet replicas:1 = one PVC = one stable identity.
- **Game knowledge lives in agents, never in the operator**: the central operator has zero game-specific code paths. Adding a new game means publishing an agent image and a GameProfile, not modifying minato.
- **Public, stable agent API**: third parties can write agents. The agent gRPC interface is versioned (semver) and supports multiple versions concurrently.
- **K8s-native everything**: all state in CRDs and K8s resources. Auth via K8s tokens + OIDC. RBAC via standard ClusterRoles/Roles. Metrics via ServiceMonitor (or PodMonitor). No custom databases.
- **GitOps-friendly**: every concept is a Kubernetes resource. ArgoCD/Flux manages profiles and servers naturally.
- **Enterprise from day one**: HA, leader election, validating webhooks, audit logs, ResourceQuota integration, multi-tenant isolation, security baselines (PSS:restricted, non-root, no privileged containers).

## CRDs (target shapes — refine as we build)

### `GameProfile` (cluster-scoped, reusable game definition)

```yaml
spec:
  displayName: string
  category: string  # sandbox, fps, mmo, etc.
  
  image: string             # game container image
  imagePullPolicy: string
  
  agent:
    image: string           # the per-game agent image
    version: string         # for compatibility checks against the operator
    config: object          # agent-specific config (passed via env or mount)
    resources: ResourceRequirements
  
  ports: []PortSpec         # name, containerPort, protocol, exposeAs (ClusterIP|NodePort|LoadBalancer|HostPort)
  
  environment: []EnvSpec    # key, type, default, required, validation
  
  resources:
    tiers: map[string]ResourceRequirements   # small, medium, large, ...
    default: string
  
  storage:
    mountPath: string
    sizeDefault: string
    sizeMin: string
    sizeMax: string
    accessModes: []string
    storageClassDefault: string
  
  capabilities:
    files: bool             # inject filebrowser sidecar
    sftp: bool              # inject sftp sidecar
    backup: bool            # this game supports the backup action
    restoreFromSnapshot: bool
  
  observability:
    agentMetrics:
      port: int
      path: string
    gameExporter:           # optional — for games with native or upstream exporters
      port: int
      path: string
    serviceMonitor:
      enabled: bool
      interval: duration
      labels: map[string]string
      metricRelabelings: []object
    podMonitor:
      enabled: bool
  
  actions: []ActionDecl     # declared catalog (executed by the agent, not the operator)
                            # each entry: name, description, params schema, returns, concurrency, timeout
```

### `GameServer` (namespace-scoped, one running instance)

```yaml
spec:
  profile: string           # GameProfile name
  tier: string              # tier from the profile
  
  env: map[string]string    # overrides
  envFrom: []object         # secret/configmap refs
  
  storage:
    size: string
    storageClass: string
  
  networking:
    exposeMode: string      # ClusterIP|NodePort|LoadBalancer|HostPort
    requestedPorts: []int   # optional — request specific external ports
  
  lifecycle:
    idleTimeoutSeconds: int # 0 = never auto-shutdown
    autoStart: bool

status:
  state: string             # Provisioning | Running | Idle | Stopped | Error
  endpoints: map[string]Endpoint    # game, agent, filebrowser, sftp
  agentVersion: string
  players: int
  playerCapacity: int
  lastActivity: timestamp
  conditions: []Condition
```

### `GameServerFleet` (namespace-scoped, manages N GameServers of the same profile)

```yaml
spec:
  profile: string
  replicas: int
  template:
    metadata: { labels, annotations }
    spec: GameServerSpec    # all fields except profile (inherited)
  updateStrategy:
    type: string            # RollingUpdate | OnDelete
    rollingUpdate:
      maxUnavailable: int
      maxSurge: int
      drainTimeoutSeconds: int

status:
  replicas: int
  readyReplicas: int
  updatedReplicas: int
  conditions: []Condition
```

### `ActionExecution` (namespace-scoped, audit + concurrency control)

```yaml
spec:
  targetRef: ObjectReference  # GameServer or GameServerFleet
  actionName: string
  params: map[string]string
  caller: string

status:
  state: string             # Pending | Running | Succeeded | Failed | TimedOut | Rejected
  startedAt: timestamp
  endedAt: timestamp
  agentResponse: string     # JSON-encoded
  error: string
```

### `GameSnapshot` (namespace-scoped, world data backup)

```yaml
spec:
  gameServerRef: string
  schedule: string          # cron, optional
  retention:
    count: int
    duration: duration

status:
  snapshots: []SnapshotEntry  # name, createdAt, volumeSnapshotRef, sizeBytes
  lastSnapshotAt: timestamp
```

## Agent gRPC API (target — refine in Milestone 3)

```protobuf
service Agent {
  // Identity and capabilities
  rpc Info(InfoRequest) returns (InfoResponse);  
    // returns: agent name, version, supported actions, supported metrics
  
  // Health and lifecycle
  rpc HealthCheck(HealthRequest) returns (HealthResponse);  
    // ready/not-ready, optional game-specific health detail
  rpc PrepareShutdown(ShutdownRequest) returns (ShutdownResponse);  
    // run graceful shutdown sequence; agent decides the steps
  
  // Player visibility
  rpc GetPlayers(PlayersRequest) returns (PlayersResponse);  
    // standardized: online count, capacity, optional list of names/ids
  
  // Actions
  rpc ExecuteAction(ExecuteActionRequest) returns (ExecuteActionResponse);  
    // dispatch a named action; agent implements per-game logic
  
  // Console streaming (bidi stream)
  rpc Console(stream ConsoleClientMessage) returns (stream ConsoleServerMessage);
}
```

The API is versioned via package name (`minato.agent.v1`). New versions are added side-by-side; deprecated versions are supported for at least one minor minato release after announcement.

## Tech stack

- Go 1.22+
- `sigs.k8s.io/controller-runtime` for the operator
- `kubebuilder` for project scaffolding
- `connectrpc.com/connect` or `google.golang.org/grpc` for the agent API (propose which in Milestone 3)
- `chi` for the HTTP control plane API
- Buf for protobuf management
- envtest + kind for integration testing
- `monitoring.coreos.com/v1` types for ServiceMonitor/PodMonitor (Prometheus Operator)

## Project layout (target)

```
minato/
├── api/
│   ├── operator/v1/         # minato CRD Go types
│   └── agent/v1/            # Agent gRPC protobuf definitions
├── cmd/
│   ├── operator/            # minato operator binary
│   ├── controlplane/        # Control plane API binary
│   └── agents/              # Bundled agents
│       ├── generic/         # YAML-action-interpreting agent
│       ├── minecraft/
│       ├── csgo/            # (or cs2/)
│       └── palworld/
├── internal/
│   ├── controllers/         # operator reconcilers
│   ├── webhook/             # validating + conversion webhooks
│   ├── controlplane/        # HTTP handlers, auth
│   └── observability/       # ServiceMonitor generation, metrics schemas
├── sdk/
│   └── agent/               # Agent SDK (publicly importable)
│       ├── server/          # gRPC scaffolding
│       ├── rcon/            # RCON client library (Source, Minecraft, Palworld dialects)
│       ├── actions/         # declarative action interpreter
│       ├── lifecycle/       # graceful shutdown helpers
│       ├── metrics/         # standard metric naming + registration
│       └── testing/         # test harness for agents
├── proto/                   # .proto sources; generated code lives in api/agent/v1
├── config/
│   ├── crd/
│   ├── rbac/
│   ├── webhook/
│   ├── manager/
│   ├── controlplane/
│   ├── samples/
│   └── tests/               # kustomizations for e2e tests
├── profiles/                # Curated GameProfile YAMLs
│   ├── minecraft-paper/
│   ├── cs2/                 # or csgo/
│   └── palworld/
├── deploy/
│   ├── helm/
│   │   └── minato/          # Helm chart
│   └── kustomize/
├── docs/
│   ├── architecture/
│   ├── operations/
│   ├── agent-developers/    # how to write a new agent
│   └── adrs/
└── Makefile
```

## Working agreement

- Each subprompt focuses on one milestone. Don't get ahead.
- Show file tree changes before writing code.
- Prefer many small files over few large ones.
- Write tests as you go — unit, envtest, table-driven.
- Surface design questions before committing to a choice; propose options.
- When you find ambiguity in my instructions, ask before guessing.
- Don't add features outside the current milestone. MVP discipline matters.
- Every milestone closes with: tests passing, docs updated, sample manifests applied successfully on a local cluster.

Confirm you understand this brief and ask any clarifying questions before we begin Milestone 1.
````

# minato Server Sharding Design

## Status: Planning / Under Discussion

---

## Problem Statement

As minato scales to support larger games (MMOs, massive SMP servers), a single GameServer may not be sufficient:

- **Player limits:** A single Minecraft server caps at ~100-200 players. An MMO might need 10,000+ concurrent players.
- **Geographic distribution:** Players in EU and US need low-latency servers. One server in EU means 150ms+ for US players.
- **World size:** A single persistent world can grow to terabytes. Splitting the world into shards reduces per-server storage.
- **Failure domains:** If one server crashes, all players are disconnected. Sharding limits the blast radius.

**Question:** How can minato support sharding while staying game-agnostic?

---

## Core Principle

> **Sharding is a deployment pattern, not a game mechanic.**

The operator provides infrastructure for sharding. The game/agent decides what a "shard" means:
- **Minecraft:** A shard could be a server in a BungeeCord/Velocity network
- **MMO:** A shard could be a geographic zone (EU-1, US-West-1)
- **FPS:** A shard could be a match instance (each match = one shard)
- **SMP:** A shard could be a dimension (overworld, nether, end as separate servers)

The operator **never** implements game-specific sharding logic (no "shard map", no "player migration", no "world splitting").

---

## Use Cases

### Use Case 1: Minecraft Server Network (BungeeCord/Velocity)

```
┌─────────────────────────────────────────┐
│           Proxy (BungeeCord)            │
│  - Routes players to shards             │
│  - Handles authentication               │
│  - Manages cross-server chat            │
└─────────────────┬───────────────────────┘
                  │
    ┌─────────────┼─────────────┐
    ▼             ▼             ▼
┌────────┐  ┌────────┐  ┌────────┐
│Shard-1 │  │Shard-2 │  │Shard-3 │
│Lobby   │  │SMP-1   │  │SMP-2   │
│(hub)   │  │(survival)│  │(creative)│
└────────┘  └────────┘  └────────┘
```

**minato role:** Deploy 3 GameServers + 1 Proxy. Inject shard topology so proxy knows where to route.

### Use Case 2: MMO Zone Sharding

```
┌─────────────────────────────────────────┐
│           Game World (10k players)      │
│                                         │
│  ┌─────────┐  ┌─────────┐  ┌─────────┐ │
│  │ EU-West │  │ US-East │  │ APAC    │ │
│  │ Zone-1  │  │ Zone-2  │  │ Zone-3  │ │
│  │ 3k plyr │  │ 4k plyr │  │ 3k plyr │ │
│  └─────────┘  └─────────┘  └─────────┘ │
│                                         │
│  Shared: Guilds, Auction House, Chat    │
└─────────────────────────────────────────┘
```

**minato role:** Deploy 3 GameServers as shards. Each shard handles one zone. Shared state managed by external services (Redis, DB).

### Use Case 3: Match-Based Games (CS2, Valorant-style)

```
┌─────────────────────────────────────────┐
│           Matchmaker Service            │
│  - Creates match = 1 shard              │
│  - Destroys shard when match ends       │
└─────────────────┬───────────────────────┘
                  │
    ┌─────────────┼─────────────┬──────────┐
    ▼             ▼             ▼          ▼
┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐
│Match-1 │  │Match-2 │  │Match-3 │  │Match-4 │
│10 plyr │  │10 plyr │  │10 plyr │  │10 plyr │
│de_dust2│  │de_nuke │  │de_mirage│  │de_inferno│
└────────┘  └────────┘  └────────┘  └────────┘
```

**minato role:** Fleet with replicas=N, each replica = one match. Matchmaker creates/destroys GameServers via API.

---

## Design Options

### Option A: Fleet as Shard Controller (What we almost built)

Add `shardMode` to GameServerFleet. When enabled:
- Each GameServer gets `SHARD_ID`, `SHARD_COUNT` env vars
- A ConfigMap is created with shard topology (addresses of all shards)
- The game/agent reads the ConfigMap to discover other shards

```yaml
apiVersion: operator.minato.io/v1
kind: GameServerFleet
metadata:
  name: minecraft-network
spec:
  profile: minecraft-paper
  replicas: 3
  shardMode:
    enabled: true
    sharedConfig:
      PROXY_ADDRESS: "minecraft-proxy.default.svc.cluster.local"
      REDIS_HOST: "redis.default.svc.cluster.local"
```

**Pros:**
- Simple: One field enables sharding
- Game-agnostic: Just injects env vars and ConfigMap
- Works with any game that can read env vars

**Cons:**
- Limited: Only provides static topology. No dynamic shard creation.
- Doesn't handle proxy deployment (BungeeCord is a separate GameServer)
- Shard identity is tied to fleet index (shard-1, shard-2...), not game concepts ("EU-West", "Lobby")

### Option B: Separate GameServerShard CRD

Introduce a new CRD that represents a logical shard:

```yaml
apiVersion: operator.minato.io/v1
kind: GameServerShard
metadata:
  name: minecraft-smp-1
  labels:
    minato.io/shard-group: minecraft-network
spec:
  # References the fleet/parent
  fleetRef:
    name: minecraft-network
  
  # Shard identity (game-defined)
  shardId: "smp-1"
  shardType: "survival"  # lobby | survival | creative | ...
  
  # Overrides from fleet template
  env:
    WORLD_NAME: "survival-world-1"
    DIFFICULTY: "hard"
```

The GameServerShard controller creates the underlying GameServer with the shard identity.

**Pros:**
- Flexible: Shards can have different configs (one is lobby, one is survival)
- Game-defined identity: Shard ID is meaningful ("EU-West", "Lobby")
- Can be created/deleted independently (for match-based games)

**Cons:**
- More complex: New CRD, new controller
- User must manage shard lifecycle explicitly
- Overlap with GameServerFleet (both create GameServers)

### Option C: Higher-Level GameServerNetwork CRD

A parent CRD that manages the entire network (shards + proxy):

```yaml
apiVersion: operator.minato.io/v1
kind: GameServerNetwork
metadata:
  name: minecraft-smp
spec:
  # The proxy that routes to shards
  proxy:
    profile: minecraft-bungeecord
    replicas: 2  # HA proxy
  
  # Shard definitions
  shards:
    - name: lobby
      profile: minecraft-paper
      replicas: 1
      env:
        SERVER_TYPE: lobby
    
    - name: survival
      profile: minecraft-paper
      replicas: 3
      env:
        SERVER_TYPE: survival
    
    - name: creative
      profile: minecraft-paper
      replicas: 1
      env:
        SERVER_TYPE: creative
  
  # Shared services (redis, db)
  sharedServices:
    - name: redis
      type: Redis
    - name: mysql
      type: MySQL
```

**Pros:**
- Complete solution: Proxy + shards + shared services in one manifest
- Game-agnostic: Just defines topology, no game logic
- GitOps-friendly: One YAML = entire game network

**Cons:**
- Very complex: New CRD, lots of controllers
- Shared services (Redis, MySQL) are outside minato scope — how do we manage them?
- Over-engineered for simple use cases (one shard = one GameServerFleet)

---

## Recommendation

**Do NOT implement sharding in the operator yet.** Here's why:

1. **Current Fleet is sufficient for 80% of cases:** Most games need 1-3 servers, not 100 shards
2. **Sharding is game-specific:** What works for Minecraft (BungeeCord) doesn't work for an MMO (zone-based)
3. **Agent/SDK can handle it:** The agent gRPC API can expose shard topology to the game. The game decides what to do with it.
4. ** premature optimization:** We don't have users asking for 10,000-player worlds yet

**Instead, enhance the existing Fleet with shard-friendly features:**

### Phase 1: Shard-Aware Fleet (Minimal Changes)

Add optional shard metadata to GameServerFleet (NO new CRDs):

```yaml
apiVersion: operator.minato.io/v1
kind: GameServerFleet
metadata:
  name: minecraft-network
spec:
  profile: minecraft-paper
  replicas: 3
  # NEW: Optional shard metadata (just labels/env, no logic)
  template:
    metadata:
      labels:
        minato.io/shard-group: minecraft-network
    spec:
      env:
        SHARD_COUNT: "3"  # Agent can read this
```

The fleet controller already injects stable names (`fleet-0`, `fleet-1`). The agent can derive `SHARD_ID` from the pod name or hostname.

**No code changes needed in operator.** Just document the pattern.

### Phase 2: Shard Topology API (Agent SDK)

Add an API to the agent SDK for shard discovery:

```protobuf
service Agent {
  // Existing methods...
  
  // NEW: Shard topology (implemented by game agent)
  rpc GetShardTopology(ShardTopologyRequest) returns (ShardTopologyResponse);
}

message ShardTopologyRequest {}

message ShardTopologyResponse {
  string shard_id = 1;
  int32 shard_count = 2;
  repeated ShardPeer peers = 3;
}

message ShardPeer {
  string shard_id = 1;
  string address = 2;
  string status = 3;
}
```

The agent reads the fleet labels/env to discover its shard identity and peers. Game-specific agents (Minecraft, MMO) implement this differently.

### Phase 3: GameServerNetwork CRD (Future)

When we have a real use case (e.g., a customer running a 5,000-player MMO), then design the GameServerNetwork CRD. Until then, document the pattern but don't implement.

---

## What We Should Do Now

1. **Document the pattern:** Add a doc showing how to use GameServerFleet for sharding (env vars, labels, DNS-based discovery)
2. **Enhance agent SDK:** Add `GetShardTopology` to the agent gRPC API (optional, no-op default)
3. **Profile examples:** Add a Minecraft network example (fleet + proxy) to `profiles/`
4. **NO CRD changes:** The existing GameServerFleet is sufficient

---

## Open Questions

1. **Should the operator create proxy/load-balancer pods?** Or is that external (e.g., Ingress, Istio)?
2. **How do shards discover each other?** DNS (headless service), ConfigMap, or agent API?
3. **What about shared state?** Redis, MySQL — should minato deploy these or assume they exist?
4. **Cross-shard player migration:** Who handles this? Agent? Game server? Control plane?
5. **Auto-sharding:** Should the fleet auto-create shards based on player load? Or is that game-specific?

---

## Related Concepts

| Concept | minato Approach |
|---------|----------------|
| **Shard** | One GameServer in a Fleet |
| **Shard Group** | One GameServerFleet |
| **Shard Identity** | Pod name (`fleet-0`, `fleet-1`) or env var |
| **Shard Discovery** | DNS (headless service) or agent API |
| **Proxy/Router** | Separate GameServer with its own profile |
| **Shared State** | External Redis/DB (not managed by minato) |

---

*Status: Planning*
*Date: 2026-05-28*

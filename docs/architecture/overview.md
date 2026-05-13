# Minami Architecture Overview

Minami is a Kubernetes-native platform for hosting persistent, multi-game dedicated game servers. It is designed for hosting providers running many games for many tenants, and operators running large fleets of persistent worlds for a single game.

## Core components

1. **Minami Operator**: owns CRDs and reconciles them into Kubernetes resources. Game-agnostic by design.
2. **Minami Agent SDK**: the public Go module for building per-game agents and implementing the gRPC contract.
3. **Per-game Agents**: sidecars injected into every game server pod, encapsulating game-specific knowledge.
4. **Minami Control Plane**: user-facing API that validates requests, enforces auth/RBAC, audits actions, and dispatches to agents.

## Design principles

- Persistent-first: one GameServer = one StatefulSet replica + one PVC + stable identity.
- Game knowledge lives in agents, never in the operator.
- Public, stable agent API with versioned protobuf definitions.
- Kubernetes-native primitives for auth, RBAC, and state.
- GitOps-friendly: all concepts are Kubernetes resources.

## Architecture diagram

```text
                +------------------------+
                |   Minami Control Plane |
                |  (HTTP/gRPC, RBAC,     |
                |   audit, dispatch)     |
                +-----------+------------+
                            |
                            | gRPC (Agent API)
                            v
  +-------------------------+-------------------------+
  |                   Kubernetes Cluster              |
  |                                                   |
  |  +-------------------+    +-------------------+   |
  |  | Minami Operator   |    | GameServer Pod    |   |
  |  | (CRD reconciler)  |    | +---------------+ |   |
  |  |                   |    | | Game Server   | |   |
  |  +---------+---------+    | +---------------+ |   |
  |            |              | | Agent Sidecar | |   |
  |            |              | +---------------+ |   |
  |            |              +-------------------+   |
  |            |  CRDs (GameProfile, GameServer)      |
  +------------+--------------------------------------+ 
```

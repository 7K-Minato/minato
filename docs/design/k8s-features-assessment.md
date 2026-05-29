# Modern Kubernetes Features Assessment for minato

## Game-Changers (High Impact)

### 1. Sidecar Containers (Kubernetes 1.29+ GA)

**What:** Native sidecar lifecycle management. Sidecars start before app containers and block pod termination until they exit.

**Impact for minato:**
- Agent container is guaranteed to start before game server
- Agent can gracefully shutdown (save world, notify players) before pod terminates
- No more race conditions during pod startup

```yaml
# Future pod spec
initContainers:
  - name: init-data
    # ...
containers:
  - name: minato-agent
    image: minato-agent-minecraft:v1
    restartPolicy: Always  # <-- NEW: sidecar behavior
    # Agent starts first, shuts down last
  - name: minato-game
    image: itzg/minecraft-server:latest
    # Game starts after agent is ready
```

**Status:** GA in 1.29. minato requires 1.28+ — close but not there yet.
**Action:** Document as Phase 2. Add feature gate check.

---

### 2. Volume Populators (Kubernetes 1.24+ GA)

**What:** Initialize PVCs from data sources (snapshots, other PVCs) at creation time.

**Impact for minato:**
- `GameServer.spec.storage.snapshotRef` could be implemented natively
- No manual "clone PVC then create server" workflow
- Faster restores (population happens before pod scheduling)

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
spec:
  dataSource:
    name: minecraft-world-backup
    kind: VolumeSnapshot
    apiGroup: snapshot.storage.k8s.io
  # Kubernetes handles the restore automatically
```

**Status:** GA since 1.24. Works today.
**Action:** Already partially implemented. Complete the PVC restore flow.

---

### 3. Pod Scheduling Readiness (Kubernetes 1.26+ alpha, 1.30+ beta)

**What:** Pods can opt-out of being considered for scheduling (for autoscaling, load balancing) until ready.

**Impact for minato:**
- Game servers can signal "not ready for players" during world loading
- Fleet autoscaler only counts truly-ready servers
- Prevents players joining while world is still loading

```yaml
apiVersion: v1
kind: Pod
spec:
  schedulingGates:
    - name: minato.io/world-loaded  # <-- Blocks scheduling
```

Agent removes the gate once the game world is fully loaded.

**Status:** Beta in 1.30. Stable in 1.32.
**Action:** Add to agent SDK. Implement in game agents.

---

### 4. Topology Spread Constraints

**What:** Distribute pods across topology domains (zones, nodes) for HA.

**Impact for minato:**
- Fleet shards spread across availability zones
- Prevents all game servers on same node (node failure = outage)
- Zone-aware player routing

```yaml
apiVersion: operator.minato.io/v1
kind: GameServerFleet
spec:
  replicas: 3
  topologySpreadConstraints:
    - maxSkew: 1
      topologyKey: topology.kubernetes.io/zone
      whenUnsatisfiable: DoNotSchedule
      labelSelector:
        matchLabels:
          minato.io/fleet: minecraft-network
```

**Status:** GA since 1.19. Works today.
**Action:** Add to GameServerFleet spec. Document best practices.

---

### 5. PriorityClass + Preemption

**What:** Ensure critical game servers get scheduled even under resource pressure.

**Impact for minato:**
- Live game servers: high priority (don't evict)
- Staging/test servers: low priority (evict first)
- Fleet scaling: new shards preempt lower-priority workloads

```yaml
apiVersion: scheduling.k8s.io/v1
kind: PriorityClass
metadata:
  name: minato-production
value: 1000000
globalDefault: false
preemptionPolicy: PreemptLowerPriority
description: "Production game servers - do not evict"
```

**Status:** GA since 1.14. Works today.
**Action:** Add `priorityClassName` to GameServer/GameServerFleet spec.

---

## Nice-to-Have (Medium Impact)

### 6. Ephemeral Containers

**What:** Debug running pods by attaching temporary containers.

**Impact for minato:**
- Debug a running game server without restarting
- Attach `tcpdump`, `strace`, or custom debug tools
- Inspect game files in a running pod

```bash
kubectl debug -it minecraft-smp-1 --image=nicolaka/netshoot --target=minato-game
```

**Status:** GA since 1.25.
**Action:** Document in troubleshooting guide. No code changes.

---

### 7. PodDisruptionBudget (Enhanced in 1.27+)

**What:** Control how many pods can be voluntarily disrupted (evicted, drained).

**Impact for minato:**
- Ensure at least N game servers remain during node upgrades
- Prevent cluster autoscaler from terminating player-active servers

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
spec:
  minAvailable: 80%
  selector:
    matchLabels:
      minato.io/managed-by: gameserverfleet
  # NEW in 1.27: unhealthyPodEvictionPolicy
  unhealthyPodEvictionPolicy: AlwaysAllow  # Evict crashed servers
```

**Status:** GA. Enhanced in 1.27.
**Action:** Fleet controller could create PDBs automatically.

---

### 8. Dynamic Resource Allocation (Kubernetes 1.26+ alpha)

**What:** Fine-grained GPU/resource scheduling beyond limits/requests.

**Impact for minato:**
- Games with GPU requirements (AI NPCs, physics simulation)
- Fractional GPU allocation (one GPU shared across multiple servers)

```yaml
resources:
  claims:
    - name: gpu
  limits:
    nvidia.com/gpu: 1
```

**Status:** Alpha. Not widely available.
**Action:** Monitor. Add when stable and users request it.

---

### 9. ReadWriteOncePod (Kubernetes 1.27+ beta, 1.29+ GA)

**What:** PVC access mode that restricts to a single pod (even stricter than RWO).

**Impact for minato:**
- Game server PVCs guaranteed single-writer
- Prevents accidental multi-attach (data corruption)

```yaml
spec:
  accessModes:
    - ReadWriteOncePod  # Only this exact pod can mount it
```

**Status:** GA in 1.29.
**Action:** Switch GameServer PVCs from RWO to RWOP when minato drops 1.28 support.

---

### 10. Vertical Pod Autoscaler (VPA)

**What:** Automatically adjust CPU/memory requests based on actual usage.

**Impact for minato:**
- Right-size game servers (many are over-provisioned)
- Reduce infrastructure costs
- Note: VPA requires pod restart (not in-place update)

**Status:** Mature. Available as separate component.
**Action:** Document as optional addon. Don't integrate into operator.

---

## Long-Term Considerations

### 11. Gateway API (Ingress v2)

**What:** Successor to Ingress. More flexible traffic routing.

**Impact for minato:**
- Multi-protocol routing (TCP/UDP for game traffic, HTTP for APIs)
- Weighted routing for canary deployments
- Direct integration with service mesh

**Status:** v1.0 GA. Widely supported.
**Action:** Add Gateway API resources to Helm chart (optional, alongside Ingress).

### 12. KEDA (Kubernetes Event-driven Autoscaling)

**What:** Scale workloads based on custom metrics (not just CPU/memory).

**Impact for minato:**
- Scale fleet based on player queue depth
- Scale based on matchmaking wait times
- Scale to zero when no players (already partially supported via idle timeout)

**Status:** Mature. CNCF incubating.
**Action:** Design integration. Fleet controller could create ScaledObjects.

### 13. Cilium Cluster Mesh

**What:** Multi-cluster pod-to-pod connectivity with service discovery.

**Impact for minato:**
- Players can migrate between clusters (EU → US) seamlessly
- Cross-cluster fleet management
- Global service discovery

**Status:** Production-ready.
**Action:** Future. Document as enterprise deployment option.

### 14. Kueue (Kubernetes-native Job Queueing)

**What:** Queue-based workload scheduling. Manage quotas and priorities.

**Impact for minato:**
- Queue server creation requests when quota is exhausted
- Fair sharing across tenants
- Batch action execution

**Status:** Beta.
**Action:** Future. Could manage GameServer creation queues.

---

## Summary Matrix

| Feature | K8s Version | Impact | Effort | Status |
|---------|-------------|--------|--------|--------|
| **Sidecar Containers** | 1.29+ | High | Low | Phase 2 |
| **Volume Populators** | 1.24+ | High | Low | ✅ Partial |
| **Scheduling Readiness** | 1.30+ | High | Medium | Phase 2 |
| **Topology Spread** | 1.19+ | High | Low | Phase 2 |
| **Priority/Preemption** | 1.14+ | Medium | Low | Phase 2 |
| **PDB Enhanced** | 1.27+ | Medium | Medium | Phase 3 |
| **Ephemeral Containers** | 1.25+ | Low | None | ✅ Docs |
| **Dynamic Resource Allocation** | 1.26+ alpha | Medium | High | Monitor |
| **ReadWriteOncePod** | 1.29+ | Low | Low | Future |
| **VPA** | Addon | Medium | None | Docs |
| **Gateway API** | v1.0 | Medium | Medium | Phase 3 |
| **KEDA** | Addon | High | High | Phase 3 |
| **Cilium Cluster Mesh** | Addon | High | High | Future |
| **Kueue** | Addon | Medium | High | Future |

---

## Immediate Recommendations

1. **Add `priorityClassName` to GameServer/GameServerFleet** — one field, huge operational value
2. **Add `topologySpreadConstraints` to GameServerFleet** — HA without complexity
3. **Document ephemeral containers** for troubleshooting — zero code, huge debugging value
4. **Monitor sidecar containers** — implement when minato bumps minimum K8s to 1.29+
5. **Complete Volume Populator flow** — the PVC restore from snapshot is 90% done

---

*Status: Assessment*
*Date: 2026-05-28*

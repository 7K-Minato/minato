# minato Communication Security Architecture

## Threat Model

### What We're Protecting

| Asset | Threat | Impact |
|-------|--------|--------|
| **Control Plane API** | Unauthorized access, MITM | Attacker can execute actions on any game server |
| **Agent gRPC** | Unauthorized calls, eavesdropping | Attacker can shutdown servers, steal player data |
| **Game Server State** | Tampering, DoS | Player data loss, service disruption |
| **RCON Credentials** | Credential theft | Full game server compromise |

### What We Are NOT Protecting

| Asset | Reason |
|-------|--------|
| **Player → Game traffic** | Game protocols (Minecraft, CS2, etc.) use raw TCP/UDP. Encryption would break game clients. DDoS protection must happen at the network edge. |
| **Agent → Game RCON** | RCON protocols (Source, Minecraft) don't support TLS. This is a protocol limitation we cannot fix. |

---

## Defense in Depth Strategy

```
┌─────────────────────────────────────────────────────────────────────┐
│                        EXTERNAL ACCESS                               │
│                                                                     │
│   Player → Internet → Edge (DDoS/WAF) → Game Server (raw UDP/TCP)  │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│                      MANAGEMENT PLANE                                │
│                                                                     │
│   Admin → TLS → Ingress/LoadBalancer → Control Plane (HTTPS)       │
│                                                                     │
│   Control Plane ─────mTLS────→ Agent gRPC (9876)                   │
│                                                                     │
│   Agent ─────plain RCON────→ Game Server (localhost)               │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│                      NETWORK SEGMENTATION                            │
│                                                                     │
│   Namespace-level NetworkPolicies restrict:                        │
│   - Who can reach the control plane API                            │
│   - Who can reach agent gRPC ports                                 │
│   - Game servers cannot reach each other                           │
│                                                                     │
├─────────────────────────────────────────────────────────────────────┤
│                      KUBERNETES SECURITY                             │
│                                                                     │
│   - Pod Security Standards (restricted)                            │
│   - RBAC (least privilege)                                         │
│   - Service account tokens (short-lived)                           │
│   - Auditing (audit logs)                                          │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Layer 1: Network Segmentation (NetworkPolicy)

**Responsibility:** Game Helm charts define NetworkPolicies; cluster CNI enforces them.

### Why in Game Charts?

NetworkPolicy belongs in the `minato-games` Helm charts, **not the operator**, because:

1. **Game-specific ports** — Each game exposes different ports (Minecraft 25565, CS2 27015, Palworld 8211). The operator is game-agnostic and doesn't know these ports.
2. **Deployment flexibility** — Clients customize NetworkPolicy rules per environment. Charts give them full control.
3. **Separation of concerns** — The operator manages game server lifecycle. Network security is an infrastructure concern managed by the chart.

### Default NetworkPolicy (per GameProfile)

Game charts in `minato-games` generate a default NetworkPolicy:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: {{ include "minato-games.fullname" . }}
  labels:
    {{- include "minato-games.labels" . | nindent 4 }}
spec:
  podSelector:
    matchLabels:
      minato.io/profile: {{ include "minato-games.profileName" . }}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    # Allow game traffic from anywhere (players need to connect)
    - from: []
      ports:
        {{- range .Values.game.ports }}
        - protocol: {{ .protocol }}
          port: {{ .containerPort }}
        {{- end }}
  egress:
    # Allow DNS resolution
    - to: []
      ports:
        - protocol: UDP
          port: 53
    # Allow Kubernetes API
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - protocol: TCP
          port: 443
```

### Strict Mode (Optional)

Enable strict isolation in `values.yaml`:

```yaml
security:
  networkPolicy:
    enabled: true
    strictMode: true  # Per-server isolation, block inter-pod traffic
```

In strict mode, each GameServer gets its own NetworkPolicy preventing lateral movement.

**Key principle:** Agents and game servers in the same pod share network namespace, so agent → game RCON is localhost and not subject to NetworkPolicy. This is actually a security benefit — RCON never leaves the pod.

---

## Layer 2: Agent gRPC Security

**Responsibility:** minato does not implement TLS for agent gRPC. Security is the cluster administrator's responsibility.

### Why No Built-in TLS?

minato is designed to be agnostic of the cluster's networking stack. Many production clusters already have:
- **Service mesh** (Istio, Linkerd, Cilium) providing transparent mTLS
- **CNI plugins** with built-in encryption (WireGuard, IPsec)
- **NetworkPolicies** restricting pod-to-pod traffic

Adding application-level mTLS would:
- Conflict with service mesh double-encryption
- Add complexity for simple deployments
- Force a specific certificate management strategy

### Current State

Agent gRPC runs on plaintext. This is acceptable when:
- The cluster uses a service mesh with mTLS
- NetworkPolicies restrict agent port access to the control plane namespace
- The cluster CNI encrypts pod-to-pod traffic

### Recommendations for Production

| Strategy | Implementation | minato Role |
|----------|---------------|-------------|
| **Service mesh mTLS** | Configure Istio/Linkerd/Cilium | Document integration |
| **CNI encryption** | Enable WireGuard/IPsec in Cilium/Calico | None — works transparently |
| **NetworkPolicy** | Restrict agent port to control plane namespace | Provide templates in game charts |
| **Application mTLS** | Use cert-manager + custom agent images | Support via agent SDK extensibility |

### If You Need Application-Level mTLS

Write a custom agent using the minato Agent SDK. The SDK provides gRPC server scaffolding; you can add TLS credentials in your agent implementation:

```go
// In your custom agent main.go
creds, err := credentials.NewServerTLSFromFile("/certs/tls.crt", "/certs/tls.key")
if err != nil {
    log.Fatal(err)
}
grpcServer := grpc.NewServer(grpc.Creds(creds))
```

Or use a service mesh and skip application-level TLS entirely.

---

## Layer 3: Control Plane API TLS

**Responsibility:** Cluster admin configures TLS termination; minato supports it.

### Options

#### A. Ingress TLS (Recommended for External Access)

```yaml
# Helm values
controlplane:
  ingress:
    enabled: true
    tls:
      - secretName: minato-controlplane-tls
        hosts:
          - minato-api.example.com
```

The control plane itself runs HTTP. TLS is terminated at the ingress controller.

#### B. Control Plane Native TLS

```yaml
# Helm values
controlplane:
  tls:
    enabled: true
    certSecret: minato-controlplane-tls
```

The control plane serves HTTPS directly. Useful when ingress is not available.

### minato Recommendation

**Use Ingress TLS.** The control plane is an internal service that should not be exposed directly to the internet. An ingress controller (nginx, traefik, etc.) provides:
- TLS termination
- Rate limiting
- WAF integration
- OAuth/OIDC offload

---

## Layer 4: RCON Security

**Responsibility:** minato generates strong passwords; cluster admin ensures they're not logged.

### Current State

RCON is a plaintext protocol. We cannot add TLS without breaking game compatibility.

### Mitigations

```yaml
# Generated by operator per GameServer
apiVersion: v1
kind: Secret
metadata:
  name: minecraft-1-rcon
type: Opaque
stringData:
  password: "<auto-generated-32-char-random>"
```

1. **Strong passwords:** 32-character random strings, rotated on server recreation
2. **Localhost-only:** Agent and game run in the same pod; RCON never traverses the network
3. **No logging:** RCON password is never logged by the operator or agent
4. **NetworkPolicy:** Block port 25575/27015 ingress at namespace level (game servers don't need external RCON)

### If You Must Expose RCON Externally

**Don't.** If you absolutely must:

```yaml
# Use an RCON proxy with TLS wrapping
apiVersion: v1
kind: Service
type: LoadBalancer
metadata:
  name: rcon-proxy
spec:
  selector:
    app: rcon-proxy
  ports:
    - port: 25575
      targetPort: 25575
```

Deploy a separate TLS-terminating RCON proxy (e.g., stunnel) that forwards to localhost:25575.

---

## Layer 5: Service Account Security

**Responsibility:** Kubernetes; minato uses least-privilege SA.

### Current State

All pods use the default service account.

### Recommendation

```yaml
# Per-game-server service account
apiVersion: v1
kind: ServiceAccount
metadata:
  name: minato-agent
  namespace: game-servers
automountServiceAccountToken: false  # Don't mount unless needed
---
# If agent needs K8s API access (for metrics, etc.)
apiVersion: v1
kind: ServiceAccount
metadata:
  name: minato-agent-with-api
  namespace: game-servers
automountServiceAccountToken: true
```

**Why:** Reduces blast radius if a game server is compromised.

---

## Implementation Roadmap

### Phase 1: NetworkPolicy (Immediate)

- [x] Document NetworkPolicy in game charts (not operator)
- [x] Create `_library` chart template for game-specific NetworkPolicy
- [ ] Add strict mode NetworkPolicy template to `_library`
- [ ] Update game charts: minecraft-paper, cs2, palworld

### Phase 2: mTLS for Agent gRPC (Future)

- [ ] Add cert-manager Certificate resources to Helm chart
- [ ] Mount TLS secrets into agent pods
- [ ] Update agent gRPC server to use TLS
- [ ] Update control plane gRPC client to use mTLS
- [ ] Document certificate rotation

### Phase 3: Control Plane TLS (Future)

- [ ] Support native TLS in control plane HTTP server
- [ ] Document Ingress TLS configuration
- [ ] Add readiness/liveness probes over HTTPS

### Phase 4: Service Mesh Integration (Future)

- [ ] Document Istio PeerAuthentication setup
- [ ] Document Cilium network policies with L7 filtering
- [ ] Provide sidecar injection labels

---

## Configuration Example

```yaml
# values.yaml for production
security:
  networkPolicy:
    enabled: true
    strictMode: true  # Per-GameServer policies
  
  mTLS:
    enabled: true
    certManager:
      enabled: true
      issuerRef:
        name: minato-ca
        kind: ClusterIssuer
  
  controlPlane:
    tls:
      enabled: true
      certSecret: minato-controlplane-tls
    ingress:
      enabled: true
      tls:
        - secretName: minato-api-tls
          hosts:
            - minato.example.com
  
  rcon:
    passwordLength: 32
    rotateOnRecreate: true
```

---

## What minato Does vs. What Cluster Admin Does

| Concern | minato | Cluster Admin |
|---------|--------|---------------|
| **NetworkPolicy** | ❌ (game-specific, belongs in charts) | ✅ Game charts define policies; CNI enforces them |
| **Agent gRPC mTLS** | ✅ Implements TLS in code | ✅ Provides cert-manager or CA |
| **Control plane TLS** | ✅ Supports HTTPS mode | ✅ Provides certificates or ingress |
| **Service mesh** | ✅ Documents integration | ✅ Installs Istio/Linkerd/Cilium |
| **DDoS protection** | ❌ Out of scope | ✅ Cloudflare, AWS Shield, etc. |
| **Game traffic encryption** | ❌ Protocol limitation | ✅ VPN for players if needed |
| **Node security** | ❌ Out of scope | ✅ OS hardening, SELinux, etc. |
| **RBAC** | ✅ Creates minimal roles | ✅ Assigns roles, audits access |

---

## References

- [Kubernetes NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [cert-manager](https://cert-manager.io/)
- [Istio mTLS](https://istio.io/latest/docs/concepts/security/#mutual-tls-authentication)
- [Cilium Network Policy](https://docs.cilium.io/en/stable/security/policy/)
- [Minecraft RCON Protocol](https://wiki.vg/RCON)
- [Source RCON Protocol](https://developer.valvesoftware.com/wiki/Source_RCON_Protocol)
- [SPIFFE/SPIRE](https://spiffe.io/)

---

*Status: Architecture Draft*
*Date: 2026-05-28*
*Authors: minato Core Team*

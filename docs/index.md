# minato Documentation

**minato (南) is 7KGroup's Kubernetes-native platform for hosting persistent, multi-game dedicated game servers.** It is designed for enterprise use cases: hosting providers running many games for many tenants, and operators running large fleets of persistent worlds for a single game. A game-agnostic operator reconciles CRDs into Kubernetes resources, while per-game agents (sidecars) encapsulate all game-specific knowledge — RCON dialects, lifecycle quirks, and action execution — behind a public, versioned gRPC API.

[minato on GitHub](https://github.com/7K-Minato/minato)

---

## What lives here

| Section               | Contents                                                                                                                                                                                           |
| --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Architecture**      | How the platform fits together: the [Architecture Overview](architecture/overview.md) and the [Controller Flow](architecture/controller-flow.md).                                                   |
| **Operations**        | Running minato in production: [installation](operations/installation.md), [configuration](operations/configuration.md), the [CLI](operations/cli.md), [multi-tenancy](operations/multi-tenancy.md), [scalability](operations/scalability.md), [metrics](operations/metrics-schema.md), [security](operations/security.md), and [troubleshooting](operations/troubleshooting.md). |
| **Runbooks**          | Operational procedures for common failure modes: unreachable agents, crashlooping operators, pending PVCs, and pending StatefulSets.                                                                 |
| **Compliance**        | Compliance posture and controls mapping.                                                                                                                                                           |
| **Security**          | The security model: [control plane authentication & authorization](security/authentication.md) and the [communication security architecture](security/communication-security.md).                   |
| **Agent Developers**  | Building a per-game agent: the [quickstart](agent-developers/quickstart.md), [SDK reference](agent-developers/sdk-reference.md), [API stability guarantees](agent-developers/api-stability.md), and the [console streaming protocol](agent-developers/console-protocol.md). |
| **Design**            | Design explorations and assessments, including [modern Kubernetes features](design/k8s-features-assessment.md) and [server sharding](design/server-sharding.md).                                    |

## Why we publish this

Engineering buyers trust what they can read. Our architecture, our decisions, and our operational runbooks are public because they're the strongest evidence of how we work. If you're evaluating minato, your engineers should start here.

## Related

- [7KGroup](https://7kgroup.org) — company site
- [Hiroba docs](https://docs.7kgroup.io/hiroba/) — the platform engineering framework minato runs on
- [Inari docs](https://docs.7kgroup.io/inari/) — the PTaaS offering built on Hiroba
- [github.com/7K-Minato](https://github.com/7K-Minato) — minato repositories

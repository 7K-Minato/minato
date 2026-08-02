# Metrics Schema

This document defines the standard metric schema for Minato components.

## Operator Metrics

All operator metrics are prefixed with `minato_operator_`, except the
fleet-level metrics below which use the shorter `minato_` prefix for
dashboard compatibility.

| Metric Name | Type | Labels | Description | Status |
| ------------ | ------ | -------- | ------------- | -------- |
| `minato_operator_reconciliations_total` | Counter | `controller`, `result` | Total number of reconciliations | planned |
| `minato_gameservers` | Gauge | `namespace`, `profile`, `server`, `state` | One series per GameServer (value always 1), re-exported on every reconcile and removed on deletion | implemented |
| `minato_action_executions_total` | Counter | `action`, `result` | Total ActionExecutions reaching a terminal state (`Succeeded`, `Failed`, `TimedOut`, `Rejected`) | implemented |
| `minato_players_online` | Gauge | `namespace`, `server` | Current player count, exported by the operator from `GameServer.status.players` | implemented |
| `minato_player_capacity` | Gauge | `namespace`, `server` | Player capacity, exported by the operator from `GameServer.status.playerCapacity` | implemented |
| `minato_action_duration_seconds` | Histogram | `action`, `profile` | Action execution duration | planned |
| `minato_agent_unreachable_total` | Counter | `profile`, `namespace` | Agent unreachable events | planned |
| `minato_idle_shutdowns_total` | Counter | `profile` | Idle shutdown events | planned |

> **Note:** `minato_players_online` and `minato_player_capacity` are owned by
> the operator. Agents MUST NOT emit these metric names — the operator exports
> them from GameServer status so that the series survive agent restarts and
> carry consistent `namespace`/`server` labels.

## Agent Metrics

All agent metrics are prefixed with `minato_agent_`.

| Metric Name | Type | Labels | Description |
| ------------ | ------ | -------- | ------------- |
| `minato_agent_info` | Gauge | `game`, `version` | Agent info (always 1) |
| `minato_agent_uptime_seconds` | Gauge | `game`, `server` | Agent uptime |
| `minato_action_executed_total` | Counter | `game`, `server`, `action`, `result` | Total actions executed |
| `minato_rcon_errors_total` | Counter | `game`, `server` | RCON errors |
| `minato_game_responsive` | Gauge | `game`, `server` | Game responsiveness (0 or 1) |

## Game-Specific Metrics

Agents may emit additional metrics under `minato_<game>_*` namespace:

- `minato_minecraft_tps` - Minecraft server TPS
- `minato_cs2_tickrate` - CS2 server tickrate
- `minato_palworld_world_time` - Palworld world time

## Metric Naming Conventions

1. All metrics use snake_case
2. Units are suffixes: `_seconds`, `_bytes`, `_total`
3. Labels use camelCase
4. Game names in labels are lowercase (e.g., `minecraft`, `cs2`, `palworld`)

## Prometheus Queries

### GameServer Health Overview

```promql
# GameServers by state
minato_gameservers

# Servers with unreachable agents
minato_agent_unreachable_total[5m] > 0

# Action success rate
rate(minato_action_executions_total{result="Succeeded"}[5m])
/
rate(minato_action_executions_total[5m])
```

### Capacity Planning

```promql
# Player count vs capacity
minato_players_online / minato_player_capacity

# Gameservers nearing capacity
minato_players_online / minato_player_capacity > 0.8
```

## Integration

### Prometheus Operator

The operator exposes its own metrics (controller-runtime defaults plus the
implemented `minato_*` metrics above) on the manager metrics endpoint
(`:8080`, `/metrics`), scraped via the chart's ServiceMonitor
(`monitoring.serviceMonitor` values).

Agent metrics are scraped via per-GameServer ServiceMonitors **created by the
operator itself** (implemented): when a GameProfile sets
`spec.observability.serviceMonitor.enabled: true` and the Prometheus Operator
CRDs are detected in the cluster, the gameserver controller creates a
ServiceMonitor named after the GameServer that scrapes the agent Service
(`<server>-agent`) on the `metrics` port using
`spec.observability.agentMetrics.port`/`path` and the optional
`serviceMonitor.interval`. The ServiceMonitor is owned by the GameServer and
garbage-collected with it. If the Prometheus Operator is not installed, the
step is skipped with a log line and a `PrometheusOperatorNotDetected` warning
event on the GameServer.

### Grafana Alloy

Use the `prometheus.scrape` component with service discovery:

```alloy
prometheus.scrape "minato" {
  targets = discovery.kubernetes.services {
    selectors {
      role = "service"
      label = "minato.io/profile"
    }
  }
  forward_to = [prometheus.remote_write.default.receiver]
}
```

### OpenTelemetry Collector

Use the `prometheusreceiver` with service discovery:

```yaml
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: 'minato'
          kubernetes_sd_configs:
            - role: service
          relabel_configs:
            - source_labels: [__meta_kubernetes_service_label_minato_io_profile]
              action: keep
              regex: .+
```

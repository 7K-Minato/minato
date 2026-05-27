# Metrics Schema

This document defines the standard metric schema for Minato components.

## Operator Metrics

All operator metrics are prefixed with `minato_operator_`.

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `minato_operator_reconciliations_total` | Counter | `controller`, `result` | Total number of reconciliations |
| `minato_gameservers` | Gauge | `state`, `profile`, `namespace` | Number of GameServers by state |
| `minato_action_executions_total` | Counter | `action`, `profile`, `result` | Total ActionExecutions |
| `minato_action_duration_seconds` | Histogram | `action`, `profile` | Action execution duration |
| `minato_agent_unreachable_total` | Counter | `profile`, `namespace` | Agent unreachable events |
| `minato_idle_shutdowns_total` | Counter | `profile` | Idle shutdown events |

## Agent Metrics

All agent metrics are prefixed with `minato_agent_`.

| Metric Name | Type | Labels | Description |
|------------|------|--------|-------------|
| `minato_agent_info` | Gauge | `game`, `version` | Agent info (always 1) |
| `minato_agent_uptime_seconds` | Gauge | `game`, `server` | Agent uptime |
| `minato_players_online` | Gauge | `game`, `server` | Current player count |
| `minato_player_capacity` | Gauge | `game`, `server` | Server player capacity |
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

Agent metrics are scraped via ServiceMonitors created by a separate Helm chart (not the operator). The operator does not create ServiceMonitors directly.

To enable metrics scraping:
1. Install the Prometheus Operator in your cluster
2. Deploy the minato-monitoring Helm chart (or create ServiceMonitors manually)
3. Agents expose `/metrics` on port 9090 by default

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

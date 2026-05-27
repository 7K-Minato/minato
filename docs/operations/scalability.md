# Scalability

Minato is designed to scale from a single game server to thousands.

## Tested Limits

| Metric | Tested Value | Notes |
|--------|-------------|-------|
| GameServers per cluster | 1,000 | Limited by etcd and API server |
| GameServers per namespace | 100 | Recommended for ResourceQuota |
| Simultaneous ActionExecutions | 100 | Limited by operator concurrency |
| Tenant namespaces | 10+ | With 100 GameServers each |
| Snapshots per server | 50 | Configurable via retention |

## Performance Characteristics

### Operator

- **Reconciliation rate**: ~10 GameServers/second
- **Memory usage**: ~64MB base + ~1MB per 100 GameServers
- **CPU usage**: ~100m cores base + spikes during reconciliation

### Control Plane

- **Request latency**: P50 < 50ms, P99 < 200ms
- **Throughput**: 1,000 requests/second
- **Memory usage**: ~128MB base

### Agent

- **gRPC connections**: 1 per agent
- **Memory usage**: ~32MB base
- **CPU usage**: ~10m cores idle

## Tuning Knobs

### Operator

```yaml
# Increase workers for faster reconciliation
args:
  - --max-concurrent-reconciles=10

# Increase cache size
resources:
  limits:
    memory: 512Mi
```

### etcd

For large deployments (> 500 GameServers):
- Increase etcd memory: `--quota-backend-bytes=8589934592`
- Enable etcd defragmentation
- Use dedicated etcd nodes

### API Server

- Increase `max-requests-inflight`
- Increase `max-mutating-requests-inflight`
- Enable API priority and fairness

## Known Limits

1. **etcd object size**: 1.5MB per object
   - GameSnapshots with many entries may approach this
   - Use retention to limit entries

2. **Lease-based leader election**: ~15s failover
   - For faster failover, reduce lease duration

3. **Webhook latency**: Adds ~100ms to requests
   - For high-throughput, consider caching webhooks

4. **Prometheus metrics cardinality**: 
   - `minato_gameservers` has labels: state, profile, namespace
   - Limit unique profile + namespace combinations

## Scaling Checklist

- [ ] Monitor etcd memory and latency
- [ ] Monitor API server request rates
- [ ] Set ResourceQuotas per namespace
- [ ] Configure operator resource limits
- [ ] Enable horizontal pod autoscaling for control plane
- [ ] Use dedicated nodes for game servers
- [ ] Configure NetworkPolicies to limit cross-talk
- [ ] Set up log aggregation before scaling
- [ ] Test disaster recovery procedures
- [ ] Document capacity limits per cluster

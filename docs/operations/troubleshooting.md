# Troubleshooting Guide

This guide helps you diagnose and resolve common issues with Minato.

## Table of Contents

- [Operator Issues](#operator-issues)
- [GameServer Issues](#gameserver-issues)
- [Agent Issues](#agent-issues)
- [Snapshot Issues](#snapshot-issues)
- [Performance Issues](#performance-issues)
- [FAQ](#faq)

## Operator Issues

### Operator Pod CrashLoopBackOff

**Symptoms**: Operator pod keeps restarting

**Diagnosis**:
```bash
kubectl logs -n minato-system -l app.kubernetes.io/name=minato --previous
kubectl describe pod -n minato-system -l app.kubernetes.io/name=minato
```

**Common Causes**:
1. **Missing CRDs**: Ensure CRDs are installed
   ```bash
   kubectl get crds | grep minato
   ```
   Fix: `make install`

2. **RBAC Issues**: Check ClusterRole bindings
   ```bash
   kubectl auth can-i list gameservers --as=system:serviceaccount:minato-system:minato-operator
   ```

3. **Leader Election Failure**: Check for network issues between pods
   ```bash
   kubectl get leases -n minato-system
   ```

### Operator Not Reconciling

**Symptoms**: GameServer created but no resources appear

**Diagnosis**:
```bash
# Check operator logs
kubectl logs -n minato-system -l app.kubernetes.io/name=minato -f

# Check GameServer events
kubectl describe gameserver my-server -n default
```

**Common Causes**:
1. **Finalizer stuck**: Remove stuck finalizer
   ```bash
   kubectl patch gameserver my-server -n default --type=merge -p '{"metadata":{"finalizers":[]}}'
   ```

2. **Profile not found**: Ensure GameProfile exists
   ```bash
   kubectl get gameprofile my-profile
   ```

## GameServer Issues

### GameServer Stuck in Provisioning

**Symptoms**: GameServer status shows "Provisioning" indefinitely

**Diagnosis**:
```bash
# Check StatefulSet
kubectl describe statefulset my-server -n default

# Check Pod status
kubectl get pods -n default -l minato.io/gameserver=my-server

# Check PVC
kubectl describe pvc my-server -n default
```

**Common Causes**:

1. **PVC Pending**: No storage class or insufficient resources
   ```bash
   kubectl get storageclass
   kubectl describe pvc my-server -n default
   ```
   Fix: Ensure a default StorageClass exists or specify one in the GameServer spec.

2. **Image Pull Errors**:
   ```bash
   kubectl describe pod my-server-0 -n default
   ```
   Fix: Check image credentials, network access, or image tag.

3. **Resource Limits**: Insufficient CPU/memory
   ```bash
   kubectl describe node
   ```
   Fix: Adjust resource requests/limits or add nodes.

### GameServer Stuck in Error

**Symptoms**: GameServer status shows "Error"

**Diagnosis**:
```bash
kubectl describe gameserver my-server -n default
kubectl logs -n default -l minato.io/gameserver=my-server -c minato-game
```

**Common Causes**:

1. **Profile Missing**: The referenced GameProfile doesn't exist
2. **Invalid Configuration**: Environment variables or settings are invalid
3. **Game Container Crash**: Check game container logs

### Cannot Connect to Game Server

**Symptoms**: Players cannot connect to the game

**Diagnosis**:
```bash
# Check service endpoints
kubectl get endpoints my-server -n default

# Check service type
kubectl get service my-server -n default

# Test connectivity from within cluster
kubectl run -it --rm debug --image=busybox --restart=Never -- wget -O- my-server:25565
```

**Common Causes**:

1. **Service Type**: ClusterIP only works within the cluster
   - Use NodePort or LoadBalancer for external access
   - Or set up an ingress/controller

2. **Port Configuration**: Wrong port in GameProfile
   ```bash
   kubectl get gameprofile my-profile -o yaml
   ```

3. **Firewall Rules**: Cloud provider security groups blocking traffic

## Agent Issues

### Agent Unhealthy

**Symptoms**: GameServer shows AgentReachable=False

**Diagnosis**:
```bash
# Check agent container logs
kubectl logs -n default my-server-0 -c minato-agent

# Check agent health endpoint
kubectl exec -n default my-server-0 -c minato-agent -- wget -qO- localhost:8080/healthz

# Check agent metrics endpoint
kubectl exec -n default my-server-0 -c minato-agent -- wget -qO- localhost:9090/metrics
```

**Common Causes**:

1. **RCON Not Configured**: Agent can't connect to game RCON
   - Check RCON environment variables
   - Verify RCON port is exposed in GameProfile

2. **Game Not Ready**: Agent starts before game is ready
   - Add readiness probes to GameProfile
   - Agent will retry automatically

### Actions Failing

**Symptoms**: ActionExecution shows "Failed" status

**Diagnosis**:
```bash
kubectl describe actionexecution my-action -n default
kubectl logs -n default my-server-0 -c minato-agent
```

**Common Causes**:

1. **Unknown Action**: Action not defined in GameProfile
2. **Missing Parameters**: Required params not provided
3. **Agent Error**: RCON command failed

## Snapshot Issues

### Snapshots Not Creating

**Symptoms**: GameSnapshot shows error condition

**Diagnosis**:
```bash
kubectl describe gamesnapshot my-snapshot -n default
kubectl logs -n minato-system -l app.kubernetes.io/name=minato
```

**Common Causes**:

1. **VolumeSnapshot CRD Not Installed**:
   ```bash
   kubectl get crd volumesnapshots.snapshot.storage.k8s.io
   ```
   Fix: Install the CSI snapshotter CRDs.

2. **CSI Driver Not Installed**: Your cluster needs a CSI driver that supports snapshots
   - Check your cloud provider's CSI driver documentation

3. **PVC Not Bound**: Ensure the GameServer's PVC is bound before taking snapshots

## Performance Issues

### High CPU/Memory Usage

**Diagnosis**:
```bash
kubectl top pods -n minato-system
kubectl top nodes
```

**Solutions**:
1. Adjust operator resource limits
2. Reduce reconciliation frequency
3. Scale operator horizontally (if not using leader election)

### Slow Reconciliation

**Symptoms**: Changes take a long time to apply

**Solutions**:
1. Check etcd performance
2. Reduce number of watched resources
3. Check network latency to API server

## FAQ

### Q: How do I restart a game server?

A: Create an ActionExecution:
```bash
kubectl apply -f - <<EOF
apiVersion: operator.minato.io/v1
kind: ActionExecution
metadata:
  name: restart-my-server
  namespace: default
spec:
  targetRef:
    kind: GameServer
    name: my-server
  actionName: restart
EOF
```

### Q: How do I backup my world?

A: Create a GameSnapshot:
```bash
kubectl apply -f - <<EOF
apiVersion: operator.minato.io/v1
kind: GameSnapshot
metadata:
  name: manual-backup
  namespace: default
spec:
  gameServerRef: my-server
  retention:
    count: 5
EOF
```

### Q: Can I run multiple game servers on one node?

A: Yes, but consider:
- Resource limits and requests
- Anti-affinity rules for critical servers
- Node capacity

### Q: How do I update a GameProfile?

A: Edit the GameProfile and the operator will roll out changes to affected GameServers:
```bash
kubectl edit gameprofile my-profile
```

Note: Some changes (like image) require GameServer restart.

### Q: Where are the logs?

A:
- Operator: `kubectl logs -n minato-system -l app.kubernetes.io/name=minato`
- Game: `kubectl logs -n default my-server-0 -c minato-game`
- Agent: `kubectl logs -n default my-server-0 -c minato-agent`

### Q: How do I clean up old ActionExecutions?

A: The operator automatically cleans up old ActionExecutions based on TTL:
- Succeeded: 7 days
- Failed/Rejected/TimedOut: 30 days

### Q: Can I use my own game container image?

A: Yes! Create a GameProfile with your image and configure the agent appropriately.

## Getting More Help

If your issue isn't covered here:

1. Check the [runbooks](runbooks/) for specific scenarios
2. Search existing [GitHub issues](https://github.com/7k-group/minato/issues)
3. Join our community discussions
4. Open a new issue with:
   - Minato version
   - Kubernetes version
   - Relevant logs
   - Steps to reproduce

# Operator Pod CrashLooping

## Symptoms

- Operator pod restarts repeatedly
- GameServers not being reconciled
- `kubectl get pods -n minato` shows operator pod in CrashLoopBackOff

## Diagnosis

1. Check pod status:
```bash
kubectl describe pod -n minato -l app.kubernetes.io/component=operator
```

2. Check logs:
```bash
kubectl logs -n minato -l app.kubernetes.io/component=operator --previous
```

3. Check events:
```bash
kubectl get events -n minato --sort-by=.lastTimestamp
```

## Common Causes

### OOMKilled

Symptom: Pod killed with exit code 137

Solution: Increase memory limit:
```bash
kubectl patch deployment minato-operator -n minato -p '{"spec":{"template":{"spec":{"containers":[{"name":"operator","resources":{"limits":{"memory":"256Mi"}}}]}}}}'
```

### RBAC Issues

Symptom: Permission denied errors in logs

Solution: Verify ClusterRoleBinding exists:
```bash
kubectl get clusterrolebinding minato-manager-rolebinding
```

### API Server Unreachable

Symptom: Connection refused errors

Solution: Check network connectivity and API server status:
```bash
kubectl cluster-info
```

## Recovery

If the operator cannot recover automatically:

1. Scale to 0 and back:
```bash
kubectl scale deployment minato-operator -n minato --replicas=0
kubectl scale deployment minato-operator -n minato --replicas=1
```

2. If persistent, check for corrupted state:
```bash
kubectl delete lease minato-operator -n minato
```

## Prevention

- Set appropriate resource limits
- Monitor operator memory usage
- Configure alerting for CrashLoopBackOff

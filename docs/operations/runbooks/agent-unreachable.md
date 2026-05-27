# Agent Unreachable

## Symptoms

- GameServer status shows `AgentReachable: False`
- Actions fail with "agent unreachable" error
- Cannot connect to console

## Diagnosis

1. Check if agent pod is running:
```bash
kubectl get pods -n minato -l minato.io/gameserver=SERVER_NAME
```

2. Check agent logs:
```bash
kubectl logs -n minato POD_NAME -c minato-agent
```

3. Check service endpoints:
```bash
kubectl get endpoints -n minato SERVER_NAME
```

4. Test connectivity:
```bash
kubectl exec -it -n minato deploy/minato-operator -- nc -zv SERVER_NAME 9876
```

## Common Causes

### Agent Container CrashLoopBackOff

Symptom: Agent container restarting repeatedly

Solution:
```bash
kubectl logs -n minato POD_NAME -c minato-agent
# Fix configuration issue or restart GameServer
kubectl delete gameserver SERVER_NAME -n minato
```

### Network Policy Blocking Traffic

Symptom: Connection timeout

Solution: Verify NetworkPolicy allows operator to agent communication:
```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-operator-to-agent
spec:
  podSelector:
    matchLabels:
      minato.io/gameserver: SERVER_NAME
  ingress:
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/component: operator
      ports:
        - protocol: TCP
          port: 9876
```

### Wrong Agent Image

Symptom: Agent container starts but doesn't expose gRPC port

Solution: Check GameProfile agent configuration:
```bash
kubectl get gameprofile PROFILE_NAME -o jsonpath='{.spec.agent.image}'
```

## Recovery

1. Restart the GameServer to recreate the pod:
```bash
kubectl delete pod -n minato POD_NAME
```

2. If persistent, check GameProfile and GameServer configuration:
```bash
kubectl get gameserver SERVER_NAME -n minato -o yaml
kubectl get gameprofile PROFILE_NAME -o yaml
```

## Prevention

- Use health checks in agent images
- Monitor agent reachability metrics
- Set up alerts for agent unreachable > 5 minutes

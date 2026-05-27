# StatefulSet Stuck Pending

## Symptoms

- GameServer shows state "Provisioning" indefinitely
- StatefulSet has 0 ready replicas
- Pod stuck in Pending state

## Diagnosis

1. Check pod status:
```bash
kubectl get pods -n minato -l minato.io/gameserver=SERVER_NAME
```

2. Check pod events:
```bash
kubectl describe pod -n minato POD_NAME
```

3. Check StatefulSet status:
```bash
kubectl describe statefulset -n minato SERVER_NAME
```

## Common Causes

### PVC Not Bound

Symptom: `0/3 nodes are available: persistentvolumeclaim "..." not found`

Solution:
```bash
kubectl get pvc -n minato SERVER_NAME
# If not bound, check storage class
kubectl get storageclass
# If no default SC, specify one in GameProfile
```

### Insufficient Resources

Symptom: `0/3 nodes are available: Insufficient cpu/memory`

Solution: Reduce resource requests or add nodes:
```bash
kubectl patch gameserver SERVER_NAME -n minato --type=merge -p '{"spec":{"resources":{"requests":{"cpu":"100m","memory":"256Mi"}}}}'
```

### Image Pull Failure

Symptom: `ImagePullBackOff` or `ErrImagePull`

Solution:
```bash
kubectl describe pod -n minato POD_NAME
# Check image name and pull secrets
kubectl get secret -n minato
```

## Recovery

1. Delete and recreate:
```bash
kubectl delete gameserver SERVER_NAME -n minato
kubectl apply -f gameserver.yaml
```

2. Check node capacity:
```bash
kubectl describe nodes
```

## Prevention

- Monitor PVC binding time
- Set appropriate resource requests
- Verify image accessibility before deployment

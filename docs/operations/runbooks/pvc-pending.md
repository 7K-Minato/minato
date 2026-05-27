# PVC Stuck Pending

## Symptoms

- Pod cannot start because PVC is not bound
- `kubectl get pvc` shows status "Pending"

## Diagnosis

```bash
kubectl describe pvc -n minato PVC_NAME
kubectl get events -n minato --field-selector reason=FailedBinding
```

## Common Causes

### No StorageClass

Symptom: `no persistent volumes available for this claim`

Solution: Check and configure StorageClass:
```bash
kubectl get storageclass
# If none, install a provisioner (e.g., Longhorn, Rook-Ceph)
```

### StorageClass Not Set as Default

Solution: Mark StorageClass as default:
```bash
kubectl patch storageclass STORAGE_CLASS_NAME -p '{"metadata":{"annotations":{"storageclass.kubernetes.io/is-default-class":"true"}}}'
```

### Quota Exceeded

Symptom: `exceeded quota`

Solution: Check ResourceQuota:
```bash
kubectl get resourcequota -n minato
kubectl describe resourcequota -n minato
```

## Recovery

1. If PVC is stuck, delete and recreate:
```bash
kubectl delete pvc -n minato PVC_NAME
# The StatefulSet will recreate it
```

2. Pre-provision PV manually:
```yaml
apiVersion: v1
kind: PersistentVolume
metadata:
  name: manual-pv
spec:
  capacity:
    storage: 10Gi
  accessModes:
    - ReadWriteOnce
  persistentVolumeReclaimPolicy: Delete
  storageClassName: manual
  hostPath:
    path: /data/pv
```

## Prevention

- Ensure default StorageClass exists
- Monitor PVC binding metrics
- Set appropriate ResourceQuotas

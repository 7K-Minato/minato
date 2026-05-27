package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

const (
	gameSnapshotFinalizer = "minato.io/gamesnapshot-finalizer"
)

type GameSnapshotReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.minato.io,resources=gamesnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.minato.io,resources=gamesnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.minato.io,resources=gamesnapshots/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;update;patch;delete

func (r *GameSnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	snap := &operatorv1.GameSnapshot{}
	if err := r.Get(ctx, req.NamespacedName, snap); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if snap.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(snap, gameSnapshotFinalizer) {
			controllerutil.AddFinalizer(snap, gameSnapshotFinalizer)
			if err := r.Update(ctx, snap); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		if controllerutil.ContainsFinalizer(snap, gameSnapshotFinalizer) {
			controllerutil.RemoveFinalizer(snap, gameSnapshotFinalizer)
			if err := r.Update(ctx, snap); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	server := &operatorv1.GameServer{}
	serverKey := types.NamespacedName{
		Name:      snap.Spec.GameServerRef,
		Namespace: snap.Namespace,
	}
	if err := r.Get(ctx, serverKey, server); err != nil {
		if apierrors.IsNotFound(err) {
			r.setCondition(ctx, snap, "Error", metav1.ConditionFalse, "GameServerNotFound", "referenced GameServer not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if we need to take a snapshot
	shouldSnapshot := true
	if snap.Spec.Schedule != "" {
		// For scheduled snapshots, check if it's time
		// This is simplified - real implementation would use cron parser
		if snap.Status.LastSnapshotAt != nil && time.Since(snap.Status.LastSnapshotAt.Time) < time.Hour {
			shouldSnapshot = false
		}
	}

	if shouldSnapshot {
		if err := r.createSnapshot(ctx, snap, server); err != nil {
			logger.Error(err, "failed to create snapshot")
			r.setCondition(ctx, snap, "Error", metav1.ConditionFalse, "SnapshotFailed", err.Error())
			return ctrl.Result{}, nil
		}
	}

	// Enforce retention
	if err := r.enforceRetention(ctx, snap); err != nil {
		logger.Error(err, "failed to enforce retention")
	}

	r.setCondition(ctx, snap, "Ready", metav1.ConditionTrue, "SnapshotComplete", "snapshot created successfully")

	if snap.Spec.Schedule != "" {
		return ctrl.Result{RequeueAfter: time.Hour}, nil
	}

	return ctrl.Result{}, nil
}

func (r *GameSnapshotReconciler) createSnapshot(
	ctx context.Context,
	snap *operatorv1.GameSnapshot,
	server *operatorv1.GameServer,
) error {
	logger := log.FromContext(ctx)

	// Find the PVC for this GameServer
	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, pvc); err != nil {
		return fmt.Errorf("failed to get PVC: %w", err)
	}

	vsName := fmt.Sprintf("%s-%d", snap.Name, time.Now().Unix())

	// Create VolumeSnapshot using unstructured object to avoid direct CRD dependency
	volumeSnapshot := &unstructured.Unstructured{}
	volumeSnapshot.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "snapshot.storage.k8s.io",
		Version: "v1",
		Kind:    "VolumeSnapshot",
	})
	volumeSnapshot.SetName(vsName)
	volumeSnapshot.SetNamespace(snap.Namespace)
	volumeSnapshot.SetLabels(map[string]string{
		"minato.io/gameserver": server.Name,
		"minato.io/snapshot":   snap.Name,
	})

	// Set the source PVC
	if err := unstructured.SetNestedField(volumeSnapshot.Object, map[string]any{
		"persistentVolumeClaimName": pvc.Name,
	}, "spec", "source"); err != nil {
		return fmt.Errorf("failed to set VolumeSnapshot source: %w", err)
	}

	// Set owner reference so the VolumeSnapshot is cleaned up when GameSnapshot is deleted
	if err := controllerutil.SetControllerReference(snap, volumeSnapshot, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on VolumeSnapshot: %w", err)
	}

	if err := r.Create(ctx, volumeSnapshot); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("VolumeSnapshot CRD not installed: %w", err)
		}
		return fmt.Errorf("failed to create VolumeSnapshot: %w", err)
	}

	entry := operatorv1.SnapshotEntry{
		Name:              vsName,
		CreatedAt:         metav1.Now(),
		VolumeSnapshotRef: vsName,
		SizeBytes:         0, // Will be populated from VolumeSnapshot status in future reconcile
	}

	snap.Status.Snapshots = append(snap.Status.Snapshots, entry)
	now := metav1.Now()
	snap.Status.LastSnapshotAt = &now

	logger.Info("created VolumeSnapshot", "name", vsName, "server", server.Name, "pvc", pvc.Name)
	return r.Status().Update(ctx, snap)
}

func (r *GameSnapshotReconciler) enforceRetention(ctx context.Context, snap *operatorv1.GameSnapshot) error {
	if snap.Spec.Retention.Count <= 0 && snap.Spec.Retention.Duration == "" {
		return nil
	}

	var toKeep []operatorv1.SnapshotEntry
	var toDelete []operatorv1.SnapshotEntry
	cutoff := time.Time{}
	if snap.Spec.Retention.Duration != "" {
		if d, err := time.ParseDuration(snap.Spec.Retention.Duration); err == nil {
			cutoff = time.Now().Add(-d)
		}
	}

	for _, entry := range snap.Status.Snapshots {
		keep := true
		if !cutoff.IsZero() && entry.CreatedAt.Time.Before(cutoff) {
			keep = false
		}
		if keep {
			toKeep = append(toKeep, entry)
		} else {
			toDelete = append(toDelete, entry)
		}
	}

	// Keep only the most recent N snapshots
	if snap.Spec.Retention.Count > 0 && len(toKeep) > snap.Spec.Retention.Count {
		toDelete = append(toDelete, toKeep[:len(toKeep)-snap.Spec.Retention.Count]...)
		toKeep = toKeep[len(toKeep)-snap.Spec.Retention.Count:]
	}

	// Delete associated VolumeSnapshots
	for _, entry := range toDelete {
		vs := &unstructured.Unstructured{}
		vs.SetGroupVersionKind(schema.GroupVersionKind{
			Group:   "snapshot.storage.k8s.io",
			Version: "v1",
			Kind:    "VolumeSnapshot",
		})
		vs.SetName(entry.VolumeSnapshotRef)
		vs.SetNamespace(snap.Namespace)
		if err := r.Delete(ctx, vs); err != nil && !apierrors.IsNotFound(err) {
			log.FromContext(ctx).Error(err, "failed to delete old VolumeSnapshot", "name", entry.VolumeSnapshotRef)
		}
	}

	snap.Status.Snapshots = toKeep
	return r.Status().Update(ctx, snap)
}

func (r *GameSnapshotReconciler) setCondition(
	ctx context.Context,
	snap *operatorv1.GameSnapshot,
	condType string,
	status metav1.ConditionStatus,
	reason, message string,
) {
	condition := metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: snap.Generation,
		LastTransitionTime: metav1.Now(),
	}
	setCondition(&snap.Status.Conditions, condition)
	_ = r.Status().Update(ctx, snap)
}

func (r *GameSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.GameSnapshot{}).
		Complete(r)
}

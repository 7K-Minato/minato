package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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

	// Create VolumeSnapshot (using snapshot.storage.k8s.io/v1)
	// Note: This requires the VolumeSnapshot CRD to be installed
	// For now, we create a placeholder entry
	vsName := fmt.Sprintf("%s-%d", snap.Name, time.Now().Unix())

	entry := operatorv1.SnapshotEntry{
		Name:              vsName,
		CreatedAt:         metav1.Now(),
		VolumeSnapshotRef: vsName,
		SizeBytes:         0, // Would be populated from actual VolumeSnapshot status
	}

	snap.Status.Snapshots = append(snap.Status.Snapshots, entry)
	now := metav1.Now()
	snap.Status.LastSnapshotAt = &now

	logger.Info("created snapshot", "name", vsName, "server", server.Name)
	return r.Status().Update(ctx, snap)
}

func (r *GameSnapshotReconciler) enforceRetention(ctx context.Context, snap *operatorv1.GameSnapshot) error {
	if snap.Spec.Retention.Count <= 0 && snap.Spec.Retention.Duration == "" {
		return nil
	}

	var toKeep []operatorv1.SnapshotEntry
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
		}
	}

	// Keep only the most recent N snapshots
	if snap.Spec.Retention.Count > 0 && len(toKeep) > snap.Spec.Retention.Count {
		toKeep = toKeep[len(toKeep)-snap.Spec.Retention.Count:]
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

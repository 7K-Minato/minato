package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
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

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

const (
	gameSnapshotFinalizer = "minato.io/gamesnapshot-finalizer"

	// Snapshot phases
	phasePending   = "Pending"
	phaseSaving    = "Saving"
	phaseSnapshotting = "Snapshotting"
	phaseReady     = "Ready"
	phaseFailed    = "Failed"

	// Timeouts
	saveTimeout      = 2 * time.Minute
	snapshotTimeout  = 10 * time.Minute
)

type GameSnapshotReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.minato.io,resources=gamesnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.minato.io,resources=gamesnapshots/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.minato.io,resources=gamesnapshots/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch
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

	// Finalizer handling
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

	// Resolve GameServer
	server := &operatorv1.GameServer{}
	serverKey := types.NamespacedName{
		Name:      snap.Spec.GameServerRef,
		Namespace: snap.Namespace,
	}
	if err := r.Get(ctx, serverKey, server); err != nil {
		if apierrors.IsNotFound(err) {
			r.setCondition(ctx, snap, "Ready", metav1.ConditionFalse, "GameServerNotFound",
				fmt.Sprintf("referenced GameServer %s not found", snap.Spec.GameServerRef))
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Determine if we should take a snapshot now
	shouldSnapshot, requeueAfter, err := r.shouldTakeSnapshot(snap)
	if err != nil {
		r.setCondition(ctx, snap, "Ready", metav1.ConditionFalse, "InvalidSchedule", err.Error())
		return ctrl.Result{}, nil
	}

	if !shouldSnapshot {
		logger.Info("skipping snapshot, not due yet", "requeueAfter", requeueAfter)
		return ctrl.Result{RequeueAfter: requeueAfter}, nil
	}

	// Check if there's already a snapshot in progress
	if r.isSnapshotInProgress(snap) {
		logger.Info("snapshot already in progress, monitoring")
		return r.monitorSnapshot(ctx, snap)
	}

	// Execute snapshot workflow: save -> snapshot -> monitor
	if err := r.executeSnapshot(ctx, snap, server); err != nil {
		logger.Error(err, "snapshot execution failed")
		r.setCondition(ctx, snap, "Ready", metav1.ConditionFalse, "SnapshotFailed", err.Error())
		return ctrl.Result{}, nil
	}

	// Enforce retention after successful snapshot
	if err := r.enforceRetention(ctx, snap); err != nil {
		logger.Error(err, "failed to enforce retention")
	}

	// Calculate next scheduled time
	if snap.Spec.Schedule != "" {
		_, nextRequeue, _ := r.shouldTakeSnapshot(snap)
		return ctrl.Result{RequeueAfter: nextRequeue}, nil
	}

	return ctrl.Result{}, nil
}

// shouldTakeSnapshot determines if a snapshot should be taken now and when to requeue.
func (r *GameSnapshotReconciler) shouldTakeSnapshot(snap *operatorv1.GameSnapshot) (bool, time.Duration, error) {
	// If no schedule, take snapshot immediately (one-shot)
	if snap.Spec.Schedule == "" {
		// Only take if we haven't taken one yet
		if snap.Status.LastSnapshotAt == nil {
			return true, 0, nil
		}
		// One-shot already completed
		return false, 0, nil
	}

	// Parse cron schedule
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	schedule, err := parser.Parse(snap.Spec.Schedule)
	if err != nil {
		return false, 0, fmt.Errorf("invalid cron schedule %q: %w", snap.Spec.Schedule, err)
	}

	now := time.Now()

	// If never taken, check if we should take one now
	if snap.Status.LastSnapshotAt == nil {
		next := schedule.Next(now)
		// If the next scheduled time is very close (within 1 minute), take it now
		if next.Sub(now) < time.Minute {
			return true, 0, nil
		}
		return false, next.Sub(now), nil
	}

	// Check if it's time for the next snapshot
	lastSnapshot := snap.Status.LastSnapshotAt.Time
	next := schedule.Next(lastSnapshot)

	if now.After(next) || now.Equal(next) {
		return true, 0, nil
	}

	requeueAfter := next.Sub(now)
	return false, requeueAfter, nil
}

// isSnapshotInProgress checks if a VolumeSnapshot is currently being created.
func (r *GameSnapshotReconciler) isSnapshotInProgress(snap *operatorv1.GameSnapshot) bool {
	if len(snap.Status.Snapshots) == 0 {
		return false
	}
	// Check the most recent snapshot
	latest := snap.Status.Snapshots[len(snap.Status.Snapshots)-1]
	// If it was created very recently (within timeout), consider it in progress
	if time.Since(latest.CreatedAt.Time) < snapshotTimeout {
		// Check if we have a condition indicating it's done
		for _, cond := range snap.Status.Conditions {
			if cond.Type == "SnapshotReady" && cond.Status == metav1.ConditionTrue {
				return false
			}
		}
		return true
	}
	return false
}

// executeSnapshot orchestrates the full snapshot workflow.
func (r *GameSnapshotReconciler) executeSnapshot(ctx context.Context, snap *operatorv1.GameSnapshot, server *operatorv1.GameServer) error {
	logger := log.FromContext(ctx)

	// Step 1: Ask agent to save/flush game data
	r.setCondition(ctx, snap, "SnapshotReady", metav1.ConditionFalse, "Saving", "requesting game save via agent")
	if err := r.callAgentSave(ctx, server); err != nil {
		logger.Error(err, "agent save failed")
		// Continue anyway - the snapshot will still capture the filesystem state
		// Some games handle this gracefully
	}

	// Step 2: Create VolumeSnapshot
	r.setCondition(ctx, snap, "SnapshotReady", metav1.ConditionFalse, "Snapshotting", "creating VolumeSnapshot")
	if err := r.createVolumeSnapshot(ctx, snap, server); err != nil {
		return fmt.Errorf("failed to create VolumeSnapshot: %w", err)
	}

	// Step 3: Monitor VolumeSnapshot status
	return r.waitForSnapshotReady(ctx, snap)
}

// callAgentSave asks the game agent to save/flush world data.
func (r *GameSnapshotReconciler) callAgentSave(ctx context.Context, server *operatorv1.GameServer) error {
	ctx, cancel := context.WithTimeout(ctx, saveTimeout)
	defer cancel()

	addr := agentAddress(server)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to agent: %w", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentv1.NewAgentClient(conn)

	// Use ExecuteAction with a "save" action if available
	_, err = client.ExecuteAction(ctx, &agentv1.ExecuteActionRequest{
		ActionName: "save",
		Params:     map[string]string{},
	})
	if err != nil {
		// If "save" action doesn't exist, try "prepare-shutdown" as fallback
		// which typically triggers a save
		_, err = client.PrepareShutdown(ctx, &agentv1.ShutdownRequest{
			TimeoutSeconds: 60,
			DrainReason:    "snapshot",
		})
		if err != nil {
			return fmt.Errorf("agent save/shutdown call failed: %w", err)
		}
	}

	return nil
}

// createVolumeSnapshot creates a Kubernetes VolumeSnapshot of the game server's PVC.
func (r *GameSnapshotReconciler) createVolumeSnapshot(ctx context.Context, snap *operatorv1.GameSnapshot, server *operatorv1.GameServer) error {
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
		"minato.io/managed-by": "gamesnapshot-controller",
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

	// Record the snapshot in status
	entry := operatorv1.SnapshotEntry{
		Name:              vsName,
		CreatedAt:         metav1.Now(),
		VolumeSnapshotRef: vsName,
		SizeBytes:         0, // Will be populated when VolumeSnapshot is ready
	}

	snap.Status.Snapshots = append(snap.Status.Snapshots, entry)
	now := metav1.Now()
	snap.Status.LastSnapshotAt = &now

	logger.Info("created VolumeSnapshot", "name", vsName, "server", server.Name, "pvc", pvc.Name)
	return r.Status().Update(ctx, snap)
}

// waitForSnapshotReady polls the VolumeSnapshot status until it's ready or fails.
func (r *GameSnapshotReconciler) waitForSnapshotReady(ctx context.Context, snap *operatorv1.GameSnapshot) error {
	if len(snap.Status.Snapshots) == 0 {
		return nil
	}

	latest := snap.Status.Snapshots[len(snap.Status.Snapshots)-1]

	// Check VolumeSnapshot status
	vs := &unstructured.Unstructured{}
	vs.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "snapshot.storage.k8s.io",
		Version: "v1",
		Kind:    "VolumeSnapshot",
	})

	vsKey := types.NamespacedName{Name: latest.VolumeSnapshotRef, Namespace: snap.Namespace}
	if err := r.Get(ctx, vsKey, vs); err != nil {
		return fmt.Errorf("failed to get VolumeSnapshot status: %w", err)
	}

	// Check if readyToUse is true
	ready, found, err := unstructured.NestedBool(vs.Object, "status", "readyToUse")
	if err != nil {
		return fmt.Errorf("failed to read VolumeSnapshot status: %w", err)
	}

	if found && ready {
		// Update entry with size
		if size, found, _ := unstructured.NestedInt64(vs.Object, "status", "restoreSize"); found {
			latest.SizeBytes = size
			snap.Status.Snapshots[len(snap.Status.Snapshots)-1] = latest
		}

		r.setCondition(ctx, snap, "SnapshotReady", metav1.ConditionTrue, "SnapshotComplete",
			fmt.Sprintf("VolumeSnapshot %s is ready", latest.VolumeSnapshotRef))
		return nil
	}

	// Check for error conditions
	conditions, found, err := unstructured.NestedSlice(vs.Object, "status", "error")
	if err == nil && found && len(conditions) > 0 {
		r.setCondition(ctx, snap, "SnapshotReady", metav1.ConditionFalse, "SnapshotFailed",
			fmt.Sprintf("VolumeSnapshot %s failed", latest.VolumeSnapshotRef))
		return fmt.Errorf("VolumeSnapshot failed")
	}

	// Still pending, requeue to check again
	return fmt.Errorf("snapshot still pending")
}

// monitorSnapshot continues monitoring an in-progress snapshot.
func (r *GameSnapshotReconciler) monitorSnapshot(ctx context.Context, snap *operatorv1.GameSnapshot) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if len(snap.Status.Snapshots) == 0 {
		return ctrl.Result{}, nil
	}

	latest := snap.Status.Snapshots[len(snap.Status.Snapshots)-1]

	// Check if we've exceeded the timeout
	if time.Since(latest.CreatedAt.Time) > snapshotTimeout {
		r.setCondition(ctx, snap, "SnapshotReady", metav1.ConditionFalse, "SnapshotTimeout",
			fmt.Sprintf("VolumeSnapshot %s timed out after %v", latest.VolumeSnapshotRef, snapshotTimeout))
		logger.Error(nil, "snapshot timed out", "name", latest.VolumeSnapshotRef)
		return ctrl.Result{}, nil
	}

	// Try to get the latest status
	vs := &unstructured.Unstructured{}
	vs.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "snapshot.storage.k8s.io",
		Version: "v1",
		Kind:    "VolumeSnapshot",
	})

	vsKey := types.NamespacedName{Name: latest.VolumeSnapshotRef, Namespace: snap.Namespace}
	if err := r.Get(ctx, vsKey, vs); err != nil {
		if apierrors.IsNotFound(err) {
			// VolumeSnapshot was deleted or not created yet
			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}

	// Check readyToUse
	ready, found, err := unstructured.NestedBool(vs.Object, "status", "readyToUse")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to read VolumeSnapshot status: %w", err)
	}

	if found && ready {
		// Update size
		if size, found, _ := unstructured.NestedInt64(vs.Object, "status", "restoreSize"); found {
			latest.SizeBytes = size
			snap.Status.Snapshots[len(snap.Status.Snapshots)-1] = latest
		}

		r.setCondition(ctx, snap, "SnapshotReady", metav1.ConditionTrue, "SnapshotComplete",
			fmt.Sprintf("VolumeSnapshot %s is ready", latest.VolumeSnapshotRef))
		return ctrl.Result{}, nil
	}

	// Check for errors
	errMsg, found, _ := unstructured.NestedString(vs.Object, "status", "error", "message")
	if found && errMsg != "" {
		r.setCondition(ctx, snap, "SnapshotReady", metav1.ConditionFalse, "SnapshotFailed", errMsg)
		return ctrl.Result{}, fmt.Errorf("VolumeSnapshot failed: %s", errMsg)
	}

	// Still in progress, requeue
	logger.Info("snapshot in progress", "name", latest.VolumeSnapshotRef)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
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

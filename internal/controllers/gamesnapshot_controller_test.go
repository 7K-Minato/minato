package controllers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func TestGameSnapshotReconciler_CreateSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	server := newTestGameServer()
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns}}
	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: server.Name,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, pvc, snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	err := r.createVolumeSnapshot(ctx, snap, server)
	require.NoError(t, err)
	assert.Len(t, snap.Status.Snapshots, 1)
	assert.NotEmpty(t, snap.Status.Snapshots[0].Name)
	assert.NotNil(t, snap.Status.LastSnapshotAt)
}

func TestGameSnapshotReconciler_CreateSnapshot_PVCNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	server := newTestGameServer()
	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: server.Name,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, snap).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	err := r.createVolumeSnapshot(ctx, snap, server)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get PVC")
}

func TestGameSnapshotReconciler_EnforceRetention_CountOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			Retention: operatorv1.SnapshotRetention{Count: 2},
		},
		Status: operatorv1.GameSnapshotStatus{
			Snapshots: []operatorv1.SnapshotEntry{
				{Name: "snap-1", CreatedAt: metav1.NewTime(time.Now().Add(-3 * time.Hour))},
				{Name: "snap-2", CreatedAt: metav1.NewTime(time.Now().Add(-2 * time.Hour))},
				{Name: "snap-3", CreatedAt: metav1.NewTime(time.Now().Add(-1 * time.Hour))},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	err := r.enforceRetention(ctx, snap)
	require.NoError(t, err)
	require.Len(t, snap.Status.Snapshots, 2)
	assert.Equal(t, "snap-2", snap.Status.Snapshots[0].Name)
	assert.Equal(t, "snap-3", snap.Status.Snapshots[1].Name)
}

func TestGameSnapshotReconciler_EnforceRetention_DurationOnly(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			Retention: operatorv1.SnapshotRetention{Duration: "1h"},
		},
		Status: operatorv1.GameSnapshotStatus{
			Snapshots: []operatorv1.SnapshotEntry{
				{Name: "snap-1", CreatedAt: metav1.NewTime(time.Now().Add(-3 * time.Hour))},
				{Name: "snap-2", CreatedAt: metav1.NewTime(time.Now().Add(-30 * time.Minute))},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	err := r.enforceRetention(ctx, snap)
	require.NoError(t, err)
	require.Len(t, snap.Status.Snapshots, 1)
	assert.Equal(t, "snap-2", snap.Status.Snapshots[0].Name)
}

func TestGameSnapshotReconciler_EnforceRetention_NoPolicy(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Status: operatorv1.GameSnapshotStatus{
			Snapshots: []operatorv1.SnapshotEntry{
				{Name: "snap-1"},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	err := r.enforceRetention(ctx, snap)
	require.NoError(t, err)
	assert.Len(t, snap.Status.Snapshots, 1)
}

func TestGameSnapshotReconciler_SetCondition(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	r.setCondition(ctx, snap, "Ready", metav1.ConditionTrue, "Test", "all good")
	require.Len(t, snap.Status.Conditions, 1)
	assert.Equal(t, "Ready", snap.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, snap.Status.Conditions[0].Status)
	assert.Equal(t, "Test", snap.Status.Conditions[0].Reason)

	r.setCondition(ctx, snap, "Ready", metav1.ConditionFalse, "Test2", "not good")
	require.Len(t, snap.Status.Conditions, 1)
	assert.Equal(t, metav1.ConditionFalse, snap.Status.Conditions[0].Status)
	assert.Equal(t, "Test2", snap.Status.Conditions[0].Reason)
}

func TestGameSnapshotReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	r := &GameSnapshotReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"}})
	require.NoError(t, err)
}

func TestGameSnapshotReconciler_Reconcile_AddFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: "srv",
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: snap.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.GameSnapshot{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Contains(t, updated.Finalizers, gameSnapshotFinalizer)
}

func TestGameSnapshotReconciler_Reconcile_GameServerNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: "missing-srv",
		},
	}
	snap.Finalizers = []string{gameSnapshotFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: snap.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.GameSnapshot{}
	require.NoError(t, cl.Get(ctx, req, updated))
	require.GreaterOrEqual(t, len(updated.Status.Conditions), 1)
	assert.Equal(t, "Ready", updated.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionFalse, updated.Status.Conditions[0].Status)
	assert.Equal(t, "GameServerNotFound", updated.Status.Conditions[0].Reason)
}

func TestGameSnapshotReconciler_Reconcile_ScheduledSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	server := newTestGameServer()
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns}}
	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: server.Name,
			Schedule:      "* * * * *", // Every minute - will trigger immediately
		},
	}
	snap.Finalizers = []string{gameSnapshotFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, pvc, snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: snap.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.GameSnapshot{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Len(t, updated.Status.Snapshots, 1)
}

func TestGameSnapshotReconciler_Reconcile_Deletion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: "srv",
		},
	}
	snap.Finalizers = []string{gameSnapshotFinalizer}
	now := metav1.Now()
	snap.DeletionTimestamp = &now

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: snap.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.GameSnapshot{}
	assert.Error(t, cl.Get(ctx, req, updated))
}

func TestGameSnapshotReconciler_Reconcile_GetServerError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: "srv",
		},
	}
	snap.Finalizers = []string{gameSnapshotFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if _, ok := obj.(*operatorv1.GameServer); ok {
					return errors.New("get server failed")
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: snap.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get server failed")
}

func TestGameSnapshotReconciler_Reconcile_SnapshotError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	server := newTestGameServer()
	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: server.Name,
		},
	}
	snap.Finalizers = []string{gameSnapshotFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: snap.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.GameSnapshot{}
	require.NoError(t, cl.Get(ctx, req, updated))
	require.GreaterOrEqual(t, len(updated.Status.Conditions), 1)
	// Find the SnapshotFailed condition
	var found bool
	for _, cond := range updated.Status.Conditions {
		if cond.Reason == "SnapshotFailed" {
			assert.Equal(t, metav1.ConditionFalse, cond.Status)
			found = true
			break
		}
	}
	assert.True(t, found, "expected SnapshotFailed condition")
}

func TestGameSnapshotReconciler_Reconcile_ScheduledSkip(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	server := newTestGameServer()
	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns}}
	now := metav1.Now()
	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: server.Name,
			Schedule:      "0 * * * *",
		},
		Status: operatorv1.GameSnapshotStatus{
			LastSnapshotAt: &now,
		},
	}
	snap.Finalizers = []string{gameSnapshotFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, pvc, snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: snap.Name, Namespace: ns}
	res, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)
	// Should requeue sometime in the next hour
	assert.Greater(t, res.RequeueAfter, time.Duration(0))
	assert.LessOrEqual(t, res.RequeueAfter, time.Hour)

	updated := &operatorv1.GameSnapshot{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Len(t, updated.Status.Snapshots, 0)
}

func TestGameSnapshotReconciler_SetupWithManager(t *testing.T) {
	r := &GameSnapshotReconciler{}
	assert.NotNil(t, r)
}

func TestGameSnapshotReconciler_ShouldTakeSnapshot_OneShot(t *testing.T) {
	r := &GameSnapshotReconciler{}

	// Never taken - should take now
	snap := &operatorv1.GameSnapshot{}
	should, requeue, err := r.shouldTakeSnapshot(snap)
	require.NoError(t, err)
	assert.True(t, should)
	assert.Equal(t, time.Duration(0), requeue)

	// Already taken - should not take again
	now := metav1.Now()
	snap.Status.LastSnapshotAt = &now
	should, requeue, err = r.shouldTakeSnapshot(snap)
	require.NoError(t, err)
	assert.False(t, should)
	assert.Equal(t, time.Duration(0), requeue)
}

func TestGameSnapshotReconciler_ShouldTakeSnapshot_Cron(t *testing.T) {
	r := &GameSnapshotReconciler{}

	// First time with schedule - should take if next is within 1 minute
	snap := &operatorv1.GameSnapshot{
		Spec: operatorv1.GameSnapshotSpec{
			Schedule: "* * * * *", // Every minute
		},
	}
	should, requeue, err := r.shouldTakeSnapshot(snap)
	require.NoError(t, err)
	assert.True(t, should)
	assert.Equal(t, time.Duration(0), requeue)

	// Taken recently - should not take yet
	snap.Status.LastSnapshotAt = &metav1.Time{Time: time.Now()}
	should, requeue, err = r.shouldTakeSnapshot(snap)
	require.NoError(t, err)
	assert.False(t, should)
	assert.Greater(t, requeue, time.Duration(0))
}

func TestGameSnapshotReconciler_ShouldTakeSnapshot_InvalidCron(t *testing.T) {
	r := &GameSnapshotReconciler{}

	snap := &operatorv1.GameSnapshot{
		Spec: operatorv1.GameSnapshotSpec{
			Schedule: "invalid cron",
		},
	}
	should, requeue, err := r.shouldTakeSnapshot(snap)
	require.Error(t, err)
	assert.False(t, should)
	assert.Equal(t, time.Duration(0), requeue)
}

func TestGameSnapshotReconciler_IsSnapshotInProgress(t *testing.T) {
	r := &GameSnapshotReconciler{}

	// No snapshots
	snap := &operatorv1.GameSnapshot{}
	assert.False(t, r.isSnapshotInProgress(snap))

	// Recent snapshot without ready condition
	snap.Status.Snapshots = []operatorv1.SnapshotEntry{
		{Name: "snap-1", CreatedAt: metav1.Now()},
	}
	assert.True(t, r.isSnapshotInProgress(snap))

	// Recent snapshot with ready condition
	snap.Status.Conditions = []metav1.Condition{
		{Type: "SnapshotReady", Status: metav1.ConditionTrue},
	}
	assert.False(t, r.isSnapshotInProgress(snap))

	// Old snapshot
	snap.Status.Snapshots = []operatorv1.SnapshotEntry{
		{Name: "snap-1", CreatedAt: metav1.NewTime(time.Now().Add(-20 * time.Minute))},
	}
	snap.Status.Conditions = nil
	assert.False(t, r.isSnapshotInProgress(snap))
}

func TestGameSnapshotReconciler_MonitorSnapshot_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Status: operatorv1.GameSnapshotStatus{
			Snapshots: []operatorv1.SnapshotEntry{
				{Name: "vs-1", CreatedAt: metav1.Now()},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	res, err := r.monitorSnapshot(ctx, snap)
	require.NoError(t, err)
	assert.Equal(t, 10*time.Second, res.RequeueAfter)
}

func TestGameSnapshotReconciler_MonitorSnapshot_Ready(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Status: operatorv1.GameSnapshotStatus{
			Snapshots: []operatorv1.SnapshotEntry{
				{Name: "vs-1", CreatedAt: metav1.Now()},
			},
		},
	}

	// Mock the Get call to return a ready VolumeSnapshot
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					if u.GetKind() == "VolumeSnapshot" {
						u.Object = map[string]any{
							"apiVersion": "snapshot.storage.k8s.io/v1",
							"kind":       "VolumeSnapshot",
							"metadata": map[string]any{
								"name":      key.Name,
								"namespace": key.Namespace,
							},
							"status": map[string]any{
								"readyToUse":  true,
								"restoreSize": int64(1073741824),
							},
						}
						return nil
					}
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	res, err := r.monitorSnapshot(ctx, snap)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)

	// Check that size was updated
	assert.Equal(t, int64(1073741824), snap.Status.Snapshots[0].SizeBytes)
}

func TestGameSnapshotReconciler_MonitorSnapshot_Failed(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Status: operatorv1.GameSnapshotStatus{
			Snapshots: []operatorv1.SnapshotEntry{
				{Name: "vs-1", CreatedAt: metav1.Now()},
			},
		},
	}

	// Mock the Get call to return a failed VolumeSnapshot
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, cl client.WithWatch, key client.ObjectKey, obj client.Object, opts ...client.GetOption) error {
				if u, ok := obj.(*unstructured.Unstructured); ok {
					if u.GetKind() == "VolumeSnapshot" {
						u.Object = map[string]any{
							"apiVersion": "snapshot.storage.k8s.io/v1",
							"kind":       "VolumeSnapshot",
							"metadata": map[string]any{
								"name":      key.Name,
								"namespace": key.Namespace,
							},
							"status": map[string]any{
								"error": map[string]any{
									"message": "snapshot failed",
								},
							},
						}
						return nil
					}
				}
				return cl.Get(ctx, key, obj, opts...)
			},
		}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	res, err := r.monitorSnapshot(ctx, snap)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "snapshot failed")
	assert.Equal(t, time.Duration(0), res.RequeueAfter)
}

func TestGameSnapshotReconciler_MonitorSnapshot_Timeout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap", Namespace: ns},
		Status: operatorv1.GameSnapshotStatus{
			Snapshots: []operatorv1.SnapshotEntry{
				{Name: "vs-1", CreatedAt: metav1.NewTime(time.Now().Add(-15 * time.Minute))},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(snap).WithStatusSubresource(&operatorv1.GameSnapshot{}).Build()
	r := &GameSnapshotReconciler{Client: cl, Scheme: scheme}

	res, err := r.monitorSnapshot(ctx, snap)
	require.NoError(t, err)
	assert.Equal(t, time.Duration(0), res.RequeueAfter)

	// Check condition was set
	assert.Len(t, snap.Status.Conditions, 1)
	assert.Equal(t, "SnapshotReady", snap.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionFalse, snap.Status.Conditions[0].Status)
	assert.Equal(t, "SnapshotTimeout", snap.Status.Conditions[0].Reason)
}

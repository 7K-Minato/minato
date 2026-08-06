package controllers

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func TestRecordGameServerMetrics(t *testing.T) {
	server := newTestGameServer()
	server.Name = "metrics-basic"
	server.Status.State = stateRunning
	server.Status.Players = 3
	server.Status.PlayerCapacity = 20
	t.Cleanup(func() { deleteGameServerMetrics(server.Namespace, server.Name) })

	recordGameServerMetrics(server)

	assert.Equal(t, float64(1), testutil.ToFloat64(
		gameServersGauge.WithLabelValues(server.Namespace, "mc", server.Name, stateRunning)))
	assert.Equal(t, float64(3), testutil.ToFloat64(
		playersOnlineGauge.WithLabelValues(server.Namespace, server.Name)))
	assert.Equal(t, float64(20), testutil.ToFloat64(
		playerCapacityGauge.WithLabelValues(server.Namespace, server.Name)))
}

func TestRecordGameServerMetrics_StateChangeRemovesStaleSeries(t *testing.T) {
	server := newTestGameServer()
	server.Name = "metrics-state-change"
	t.Cleanup(func() { deleteGameServerMetrics(server.Namespace, server.Name) })

	server.Status.State = stateProvisioning
	recordGameServerMetrics(server)

	server.Status.State = stateRunning
	recordGameServerMetrics(server)

	assert.Equal(t, float64(1), testutil.ToFloat64(
		gameServersGauge.WithLabelValues(server.Namespace, "mc", server.Name, stateRunning)))
	assert.Equal(t, float64(0), testutil.ToFloat64(
		gameServersGauge.WithLabelValues(server.Namespace, "mc", server.Name, stateProvisioning)),
		"stale state series must be removed on transition")
}

func TestDeleteGameServerMetrics(t *testing.T) {
	server := newTestGameServer()
	server.Name = "metrics-delete"
	server.Status.State = stateRunning

	recordGameServerMetrics(server)
	deleteGameServerMetrics(server.Namespace, server.Name)

	assert.Equal(t, float64(0), testutil.ToFloat64(
		gameServersGauge.WithLabelValues(server.Namespace, "mc", server.Name, stateRunning)))
	assert.Equal(t, float64(0), testutil.ToFloat64(
		playersOnlineGauge.WithLabelValues(server.Namespace, server.Name)))
	assert.Equal(t, float64(0), testutil.ToFloat64(
		playerCapacityGauge.WithLabelValues(server.Namespace, server.Name)))
}

func TestObserveActionExecution(t *testing.T) {
	before := testutil.ToFloat64(actionExecutionsTotal.WithLabelValues("backup", "Succeeded"))
	observeActionExecution("backup", "Succeeded")
	after := testutil.ToFloat64(actionExecutionsTotal.WithLabelValues("backup", "Succeeded"))
	assert.Equal(t, before+1, after)
}

func TestGameServerReconciler_Reconcile_ExportsMetrics(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	profile := newTestProfile()
	server := newTestGameServer()
	server.Name = "metrics-reconcile"
	server.Finalizers = []string{gameServerFinalizer}
	t.Cleanup(func() { deleteGameServerMetrics(server.Namespace, server.Name) })

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server).
		WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: server.Name, Namespace: server.Namespace}}
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, float64(1), testutil.ToFloat64(
		gameServersGauge.WithLabelValues(server.Namespace, "mc", server.Name, stateProvisioning)))
	assert.Equal(t, float64(0), testutil.ToFloat64(
		playersOnlineGauge.WithLabelValues(server.Namespace, server.Name)))
	assert.Equal(t, float64(0), testutil.ToFloat64(
		playerCapacityGauge.WithLabelValues(server.Namespace, server.Name)))
}

func TestGameServerReconciler_Reconcile_DeletionCleansUpMetrics(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	profile := newTestProfile()
	server := newTestGameServer()
	server.Name = "metrics-deletion"
	server.Finalizers = []string{gameServerFinalizer}
	now := metav1.Now()
	server.DeletionTimestamp = &now

	// Simulate a previously exported series for the server being deleted.
	server.Status.State = stateRunning
	recordGameServerMetrics(server)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server).
		WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: server.Name, Namespace: server.Namespace}}
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	assert.Equal(t, float64(0), testutil.ToFloat64(
		gameServersGauge.WithLabelValues(server.Namespace, "mc", server.Name, stateRunning)))
	assert.Equal(t, float64(0), testutil.ToFloat64(
		playersOnlineGauge.WithLabelValues(server.Namespace, server.Name)))
}

func TestActionExecutionReconciler_Reconcile_TerminalStateIncrementsCounter(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()

	// Target GameServer does not exist -> terminal Rejected state.
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec-metrics", Namespace: "default"},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: "missing-server", Namespace: "default"},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(exec).
		WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	reconciler := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	before := testutil.ToFloat64(actionExecutionsTotal.WithLabelValues("backup", string(operatorv1.ActionExecutionRejected)))

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: exec.Name, Namespace: exec.Namespace}}
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	after := testutil.ToFloat64(actionExecutionsTotal.WithLabelValues("backup", string(operatorv1.ActionExecutionRejected)))
	assert.Equal(t, before+1, after)

	// A second reconcile of the terminal object must not double-count.
	_, err = reconciler.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, after, testutil.ToFloat64(
		actionExecutionsTotal.WithLabelValues("backup", string(operatorv1.ActionExecutionRejected))))
}

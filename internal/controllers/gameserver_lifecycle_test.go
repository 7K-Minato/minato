package controllers

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	agentv1 "github.com/7k-minato/minato/api/agent/v1/minato/agent/v1"
	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

// fakeAgent is an in-memory agent gRPC server used to verify that controllers
// call PrepareShutdown before stopping/deleting game servers.
type fakeAgent struct {
	agentv1.UnimplementedAgentServer

	shutdownCalls atomic.Int32
	lastShutdown  atomic.Pointer[agentv1.ShutdownRequest]
	shutdownDelay time.Duration
}

func (f *fakeAgent) PrepareShutdown(ctx context.Context, req *agentv1.ShutdownRequest) (*agentv1.ShutdownResponse, error) {
	f.shutdownCalls.Add(1)
	f.lastShutdown.Store(req)
	if f.shutdownDelay > 0 {
		select {
		case <-time.After(f.shutdownDelay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &agentv1.ShutdownResponse{}, nil
}

// stubAgentDial registers the fake agent on a bufconn listener and points the
// package-level dialAgent at it for the duration of the test.
func stubAgentDial(t *testing.T, agent *fakeAgent) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	agentv1.RegisterAgentServer(srv, agent)
	go func() { _ = srv.Serve(lis) }()

	old := dialAgent
	dialAgent = func(_ *operatorv1.GameServer) (*grpc.ClientConn, error) {
		return grpc.NewClient("passthrough:///bufconn",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}
	t.Cleanup(func() {
		dialAgent = old
		srv.Stop()
	})
}

func TestGameServerReconciler_Reconcile_AutoStartFalse_StopsServer(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	server := newTestGameServer()
	server.Finalizers = []string{gameServerFinalizer}
	server.Spec.Lifecycle.AutoStart = ptr.To(false)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns},
		Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
		Status:     appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}

	agent := &fakeAgent{}
	stubAgentDial(t, agent)

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, sts).WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: server.Name, Namespace: ns}
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	// Agent was asked to shut down gracefully before scale-down.
	assert.Equal(t, int32(1), agent.shutdownCalls.Load())

	// StatefulSet scaled to 0.
	updatedSts := &appsv1.StatefulSet{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: ns}, updatedSts))
	require.NotNil(t, updatedSts.Spec.Replicas)
	assert.Equal(t, int32(0), *updatedSts.Spec.Replicas)

	// Status reports Stopped.
	updated := &operatorv1.GameServer{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, stateStopped, updated.Status.State)
	require.NotEmpty(t, updated.Status.Conditions)
	assert.Equal(t, "AutoStartDisabled", updated.Status.Conditions[0].Reason)
}

func TestGameServerReconciler_Reconcile_AutoStartFalse_ShutdownFailureStillStops(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	server := newTestGameServer()
	server.Finalizers = []string{gameServerFinalizer}
	server.Spec.Lifecycle.AutoStart = ptr.To(false)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns},
		Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
		Status:     appsv1.StatefulSetStatus{ReadyReplicas: 1},
	}

	// Agent unreachable: dial fails immediately.
	old := dialAgent
	dialAgent = func(_ *operatorv1.GameServer) (*grpc.ClientConn, error) {
		return nil, context.DeadlineExceeded
	}
	t.Cleanup(func() { dialAgent = old })

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, sts).WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: server.Name, Namespace: ns}
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.GameServer{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, stateStopped, updated.Status.State)
}

func TestGameServerReconciler_Reconcile_AutoStartTrue_StartsStoppedServer(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	server := newTestGameServer()
	server.Finalizers = []string{gameServerFinalizer}
	server.Spec.Lifecycle.AutoStart = ptr.To(true)

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns},
		Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](0)},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, sts).WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: server.Name, Namespace: ns}
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	// StatefulSet scaled back to 1.
	updatedSts := &appsv1.StatefulSet{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: ns}, updatedSts))
	require.NotNil(t, updatedSts.Spec.Replicas)
	assert.Equal(t, int32(1), *updatedSts.Spec.Replicas)

	// State moves to Provisioning (not Stopped) while the pod comes up.
	updated := &operatorv1.GameServer{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, stateProvisioning, updated.Status.State)
}

func TestGameServerReconciler_Reconcile_AutoStartNil_DefaultsToRunning(t *testing.T) {
	scheme := newTestScheme()
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	server := newTestGameServer()
	server.Finalizers = []string{gameServerFinalizer}
	server.Spec.Lifecycle.AutoStart = nil // unset: pre-existing servers must keep running

	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns},
		Spec:       appsv1.StatefulSetSpec{Replicas: ptr.To[int32](1)},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, sts).WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: server.Name, Namespace: ns}
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updatedSts := &appsv1.StatefulSet{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: ns}, updatedSts))
	require.NotNil(t, updatedSts.Spec.Replicas)
	assert.Equal(t, int32(1), *updatedSts.Spec.Replicas)

	updated := &operatorv1.GameServer{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.NotEqual(t, stateStopped, updated.Status.State)
}

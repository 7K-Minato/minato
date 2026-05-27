package controllers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

func TestGameServerFleetReconciler_BuildGameServer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
		Spec: operatorv1.GameServerFleetSpec{
			Profile:  "mc",
			Replicas: 3,
			Template: operatorv1.GameServerTemplateSpec{
				Metadata: operatorv1.GameServerTemplateMetadata{
					Labels:      map[string]string{"env": "prod"},
					Annotations: map[string]string{"note": "test"},
				},
				Spec: operatorv1.FleetGameServerSpec{
					Env: map[string]string{"KEY": "VALUE"},
				},
			},
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	server := r.buildGameServer(fleet, 0)
	assert.Equal(t, "fleet-0", server.Name)
	assert.Equal(t, ns, server.Namespace)
	assert.Equal(t, "mc", server.Spec.Profile)
	assert.Equal(t, map[string]string{"KEY": "VALUE"}, server.Spec.Env)
	assert.Equal(t, "minato", server.Labels["app.kubernetes.io/name"])
	assert.Equal(t, "fleet", server.Labels["minato.io/fleet"])
	assert.Equal(t, "mc", server.Labels["minato.io/profile"])
	assert.Equal(t, "gameserverfleet", server.Labels["minato.io/managed-by"])
	assert.Equal(t, "prod", server.Labels["env"])
	assert.Equal(t, "test", server.Annotations["note"])

	server2 := r.buildGameServer(fleet, 5)
	assert.Equal(t, "fleet-5", server2.Name)
}

func TestGameServerFleetReconciler_UpdateStatus(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns, Generation: 2},
		Spec: operatorv1.GameServerFleetSpec{
			Profile:  "mc",
			Replicas: 3,
		},
	}

	servers := []operatorv1.GameServer{
		{Spec: operatorv1.GameServerSpec{Profile: "mc"}, Status: operatorv1.GameServerStatus{State: stateRunning}},
		{Spec: operatorv1.GameServerSpec{Profile: "mc"}, Status: operatorv1.GameServerStatus{State: stateProvisioning}},
		{Spec: operatorv1.GameServerSpec{Profile: "mc"}, Status: operatorv1.GameServerStatus{State: stateRunning}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet).WithStatusSubresource(&operatorv1.GameServerFleet{}).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	err := r.updateStatus(ctx, fleet, servers)
	require.NoError(t, err)
	assert.Equal(t, int32(3), fleet.Status.Replicas)
	assert.Equal(t, int32(2), fleet.Status.ReadyReplicas)
	assert.Equal(t, int32(3), fleet.Status.UpdatedReplicas)
	require.Len(t, fleet.Status.Conditions, 1)
	assert.Equal(t, "Ready", fleet.Status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, fleet.Status.Conditions[0].Status)
}

func TestGameServerFleetReconciler_CleanupFleet(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
	}
	servers := []operatorv1.GameServer{
		{ObjectMeta: metav1.ObjectMeta{Name: "fleet-0", Namespace: ns, Labels: map[string]string{"minato.io/fleet": "fleet"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "fleet-1", Namespace: ns, Labels: map[string]string{"minato.io/fleet": "fleet"}}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet, &servers[0], &servers[1]).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	err := r.cleanupFleet(ctx, fleet)
	require.NoError(t, err)

	remaining := &operatorv1.GameServerList{}
	require.NoError(t, cl.List(ctx, remaining))
	assert.Len(t, remaining.Items, 0)
}

func TestGameServerFleetReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	r := &GameServerFleetReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"}})
	require.NoError(t, err)
}

func TestGameServerFleetReconciler_Reconcile_AddFinalizer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
		Spec: operatorv1.GameServerFleetSpec{
			Profile:  "mc",
			Replicas: 0,
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet).WithStatusSubresource(&operatorv1.GameServerFleet{}).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: fleet.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.GameServerFleet{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Contains(t, updated.Finalizers, gameServerFleetFinalizer)
}

func TestGameServerFleetReconciler_Reconcile_CreateServers(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
		Spec: operatorv1.GameServerFleetSpec{
			Profile:  "mc",
			Replicas: 2,
		},
	}
	fleet.Finalizers = []string{gameServerFleetFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet).WithStatusSubresource(&operatorv1.GameServerFleet{}).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: fleet.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	servers := &operatorv1.GameServerList{}
	require.NoError(t, cl.List(ctx, servers))
	assert.Len(t, servers.Items, 2)
	assert.Equal(t, "fleet-0", servers.Items[0].Name)
	assert.Equal(t, "fleet-1", servers.Items[1].Name)
}

func TestGameServerFleetReconciler_Reconcile_DeleteExcessServers(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
		Spec: operatorv1.GameServerFleetSpec{
			Profile:  "mc",
			Replicas: 1,
		},
	}
	fleet.Finalizers = []string{gameServerFleetFinalizer}

	existing := []operatorv1.GameServer{
		{ObjectMeta: metav1.ObjectMeta{Name: "fleet-0", Namespace: ns, Labels: map[string]string{"minato.io/fleet": "fleet"}}},
		{ObjectMeta: metav1.ObjectMeta{Name: "fleet-1", Namespace: ns, Labels: map[string]string{"minato.io/fleet": "fleet"}}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet, &existing[0], &existing[1]).WithStatusSubresource(&operatorv1.GameServerFleet{}).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: fleet.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	servers := &operatorv1.GameServerList{}
	require.NoError(t, cl.List(ctx, servers))
	assert.Len(t, servers.Items, 1)
	// With the default RollingUpdate strategy, the oldest servers are deleted first.
	// Since both servers have zero CreationTimestamp, the tie-breaker is name;
	// fleet-0 < fleet-1, so fleet-0 is considered "older" and deleted, leaving fleet-1.
	assert.Equal(t, "fleet-1", servers.Items[0].Name)
}

func TestGameServerFleetReconciler_Reconcile_Deletion(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
		Spec: operatorv1.GameServerFleetSpec{
			Profile:  "mc",
			Replicas: 0,
		},
	}
	fleet.Finalizers = []string{gameServerFleetFinalizer}
	now := metav1.Now()
	fleet.DeletionTimestamp = &now

	existing := []operatorv1.GameServer{
		{ObjectMeta: metav1.ObjectMeta{Name: "fleet-0", Namespace: ns, Labels: map[string]string{"minato.io/fleet": "fleet"}}},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet, &existing[0]).WithStatusSubresource(&operatorv1.GameServerFleet{}).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: fleet.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	// Servers should be deleted and fleet should be gone (finalizer removed + deletionTimestamp set)
	servers := &operatorv1.GameServerList{}
	require.NoError(t, cl.List(ctx, servers))
	assert.Len(t, servers.Items, 0)

	updated := &operatorv1.GameServerFleet{}
	assert.Error(t, cl.Get(ctx, req, updated))
}

func TestGameServerFleetReconciler_Reconcile_ListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
		Spec: operatorv1.GameServerFleetSpec{
			Profile:  "mc",
			Replicas: 1,
		},
	}
	fleet.Finalizers = []string{gameServerFleetFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet).WithStatusSubresource(&operatorv1.GameServerFleet{}).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return errors.New("list failed")
			},
		}).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: fleet.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

func TestGameServerFleetReconciler_CleanupFleet_ListError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet", Namespace: ns},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(fleet).
		WithInterceptorFuncs(interceptor.Funcs{
			List: func(ctx context.Context, cl client.WithWatch, list client.ObjectList, opts ...client.ListOption) error {
				return errors.New("list failed")
			},
		}).Build()
	r := &GameServerFleetReconciler{Client: cl, Scheme: scheme}

	err := r.cleanupFleet(ctx, fleet)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "list failed")
}

func TestGameServerFleetReconciler_SetupWithManager(t *testing.T) {
	r := &GameServerFleetReconciler{}
	assert.NotNil(t, r)
}

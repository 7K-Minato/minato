package controllers

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func TestValidateParams_Expanded(t *testing.T) {
	r := &ActionExecutionReconciler{}

	decl := &operatorv1.ActionDecl{
		Name: "test-action",
		Params: map[string]operatorv1.ActionParamSchema{
			"required-param": {Type: "string", Required: true},
			"optional-param": {Type: "string", Required: false, Default: "default"},
		},
	}

	tests := []struct {
		name    string
		params  map[string]string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "all params present",
			params:  map[string]string{"required-param": "value", "optional-param": "opt"},
			wantErr: false,
		},
		{
			name:    "only required param",
			params:  map[string]string{"required-param": "value"},
			wantErr: false,
		},
		{
			name:    "missing required param",
			params:  map[string]string{"optional-param": "opt"},
			wantErr: true,
			errMsg:  `required param "required-param" missing`,
		},
		{
			name:    "no params",
			params:  map[string]string{},
			wantErr: true,
			errMsg:  `required param "required-param" missing`,
		},
		{
			name:    "empty params with no required fields",
			params:  map[string]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			exec := &operatorv1.ActionExecution{
				Spec: operatorv1.ActionExecutionSpec{
					Params: tt.params,
				},
			}
			declCopy := decl
			if tt.name == "empty params with no required fields" {
				declCopy = &operatorv1.ActionDecl{
					Name:   "no-required",
					Params: map[string]operatorv1.ActionParamSchema{},
				}
			}
			err := r.validateParams(exec, declCopy)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCheckConcurrency(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	server := newTestGameServer()
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec-1", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	// Another running execution on the same server
	running := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec-2", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionRunning},
	}

	declExclusive := &operatorv1.ActionDecl{
		Name:        "backup",
		Concurrency: operatorv1.ActionConcurrencyExclusive,
	}
	declSerialize := &operatorv1.ActionDecl{
		Name:        "backup",
		Concurrency: operatorv1.ActionConcurrencySerialize,
	}
	declAllow := &operatorv1.ActionDecl{
		Name:        "backup",
		Concurrency: operatorv1.ActionConcurrencyAllow,
	}

	t.Run("exclusive blocked by running", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, exec, running).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
		r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}
		canRun, reason := r.checkConcurrency(ctx, exec, server, declExclusive)
		assert.False(t, canRun)
		assert.Contains(t, reason, "exclusive action blocked by running execution")
	})

	t.Run("serialize blocked by same action running", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, exec, running).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
		r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}
		canRun, reason := r.checkConcurrency(ctx, exec, server, declSerialize)
		assert.False(t, canRun)
		assert.Contains(t, reason, "serialized action blocked by running execution")
	})

	t.Run("serialize allowed different action", func(t *testing.T) {
		runningOther := running.DeepCopy()
		runningOther.Spec.ActionName = "restore"
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, exec, runningOther).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
		r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}
		canRun, _ := r.checkConcurrency(ctx, exec, server, declSerialize)
		assert.True(t, canRun)
	})

	t.Run("allow always allowed", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, exec, running).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
		r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}
		canRun, _ := r.checkConcurrency(ctx, exec, server, declAllow)
		assert.True(t, canRun)
	})

	t.Run("no running executions", func(t *testing.T) {
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
		r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}
		canRun, _ := r.checkConcurrency(ctx, exec, server, declExclusive)
		assert.True(t, canRun)
	})

	t.Run("list error", func(t *testing.T) {
		// Use empty scheme so List fails
		badScheme := runtime.NewScheme()
		cl := fake.NewClientBuilder().WithScheme(badScheme).Build()
		r := &ActionExecutionReconciler{Client: cl, Scheme: badScheme}
		canRun, reason := r.checkConcurrency(ctx, exec, server, declExclusive)
		assert.False(t, canRun)
		assert.Contains(t, reason, "failed to list action executions")
	})
}

func TestSetState(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	r.setState(ctx, exec, operatorv1.ActionExecutionRunning, "", "")
	assert.Equal(t, operatorv1.ActionExecutionRunning, exec.Status.State)
	assert.NotNil(t, exec.Status.StartedAt)
	assert.Nil(t, exec.Status.EndedAt)
	assert.Len(t, exec.Status.Conditions, 1)
	assert.Equal(t, string(operatorv1.ActionExecutionRunning), exec.Status.Conditions[0].Type)

	r.setState(ctx, exec, operatorv1.ActionExecutionSucceeded, "done", "")
	assert.Equal(t, operatorv1.ActionExecutionSucceeded, exec.Status.State)
	assert.NotNil(t, exec.Status.EndedAt)
	assert.Equal(t, "done", exec.Status.AgentResponse)

	r.setState(ctx, exec, operatorv1.ActionExecutionFailed, "", "something broke")
	assert.Equal(t, operatorv1.ActionExecutionFailed, exec.Status.State)
	assert.Equal(t, "something broke", exec.Status.Error)

	r.setState(ctx, exec, operatorv1.ActionExecutionTimedOut, "", "timed out")
	assert.Equal(t, operatorv1.ActionExecutionTimedOut, exec.Status.State)

	r.setState(ctx, exec, operatorv1.ActionExecutionRejected, "", "rejected")
	assert.Equal(t, operatorv1.ActionExecutionRejected, exec.Status.State)
}

func TestActionExecutionReconciler_Reconcile_NotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	r := &ActionExecutionReconciler{Client: fake.NewClientBuilder().WithScheme(scheme).Build(), Scheme: scheme}

	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: types.NamespacedName{Name: "missing", Namespace: "default"}})
	require.NoError(t, err)
}

func TestActionExecutionReconciler_Reconcile_InitialState(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: "srv", Namespace: ns},
			ActionName: "backup",
		},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionPending, updated.Status.State)
}

func TestActionExecutionReconciler_Reconcile_TargetNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: "missing-srv", Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionRejected, updated.Status.State)
	assert.Contains(t, updated.Status.Error, "target GameServer not found")
}

func TestActionExecutionReconciler_Reconcile_ProfileNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	server := newTestGameServer()
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server, exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionRejected, updated.Status.State)
	assert.Contains(t, updated.Status.Error, "referenced GameProfile not found")
}

func TestActionExecutionReconciler_Reconcile_ActionNotFound(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	server := newTestGameServer()
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "nonexistent",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionRejected, updated.Status.State)
	assert.Contains(t, updated.Status.Error, "not found in profile")
}

func TestActionExecutionReconciler_Reconcile_InvalidParams(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	profile.Spec.Actions = []operatorv1.ActionDecl{
		{
			Name: "backup",
			Params: map[string]operatorv1.ActionParamSchema{
				"path": {Type: "string", Required: true},
			},
		},
	}
	server := newTestGameServer()
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
			Params:     map[string]string{},
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionRejected, updated.Status.State)
	assert.Contains(t, updated.Status.Error, "required param")
}

func TestActionExecutionReconciler_Reconcile_ConcurrencyBlocked(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	profile.Spec.Actions = []operatorv1.ActionDecl{
		{Name: "backup", Concurrency: operatorv1.ActionConcurrencyExclusive},
	}
	server := newTestGameServer()
	exec1 := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec-1", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionRunning},
	}
	exec2 := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec-2", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, exec1, exec2).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec2.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionRejected, updated.Status.State)
	assert.Contains(t, updated.Status.Error, "exclusive action blocked")
}

func TestActionExecutionReconciler_Reconcile_NonPendingState(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Status:     operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionSucceeded},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)
}

func TestActionExecutionReconciler_Reconcile_CustomTimeout(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	profile.Spec.Actions = []operatorv1.ActionDecl{
		{Name: "backup", Timeout: "10s"},
	}
	server := newTestGameServer()
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	// This will attempt to dispatch to agent and fail (no agent running), ending in Failed state.
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionFailed, updated.Status.State)
}

func TestActionExecutionReconciler_Reconcile_StatusUpdateError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(exec).WithStatusSubresource(&operatorv1.ActionExecution{}).
		WithInterceptorFuncs(interceptor.Funcs{
			SubResourceUpdate: func(ctx context.Context, cl client.Client, subResourceName string, obj client.Object, opts ...client.SubResourceUpdateOption) error {
				return errors.New("status update failed")
			},
		}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status update failed")
}

func TestActionExecutionReconciler_Reconcile_DispatchToAgent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	ctx := context.Background()
	ns := "default"

	profile := newTestProfile()
	profile.Spec.Actions = []operatorv1.ActionDecl{
		{Name: "backup", Timeout: "1s"},
	}
	server := newTestGameServer()
	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: ns}}
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "exec", Namespace: ns},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef:  operatorv1.TargetRef{Name: server.Name, Namespace: ns},
			ActionName: "backup",
		},
		Status: operatorv1.ActionExecutionStatus{State: operatorv1.ActionExecutionPending},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(profile, server, svc, exec).WithStatusSubresource(&operatorv1.ActionExecution{}).Build()
	r := &ActionExecutionReconciler{Client: cl, Scheme: scheme}

	req := types.NamespacedName{Name: exec.Name, Namespace: ns}
	_, err := r.Reconcile(ctx, ctrl.Request{NamespacedName: req})
	require.NoError(t, err)

	updated := &operatorv1.ActionExecution{}
	require.NoError(t, cl.Get(ctx, req, updated))
	assert.Equal(t, operatorv1.ActionExecutionFailed, updated.Status.State)
}

func TestActionExecutionReconciler_SetupWithManager(t *testing.T) {
	r := &ActionExecutionReconciler{}
	assert.NotNil(t, r)
}

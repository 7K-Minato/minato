// Package controllers contains the Kubernetes controllers for minato CRDs.
package controllers

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1 "github.com/7k-minato/minato/api/agent/v1/minato/agent/v1"
	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controllers/builder"
)

const (
	actionExecutionFinalizer = "minato.io/actionexecution-finalizer"
	defaultActionTimeout     = 5 * time.Minute
)

type ActionExecutionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.minato.io,resources=actionexecutions,verbs=get;list;watch
// +kubebuilder:rbac:groups=operator.minato.io,resources=actionexecutions,verbs=create;update;patch;delete
// +kubebuilder:rbac:groups=operator.minato.io,resources=actionexecutions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.minato.io,resources=actionexecutions/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers,verbs=get;list;watch
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch

func (r *ActionExecutionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	exec := &operatorv1.ActionExecution{}
	if err := r.Get(ctx, req.NamespacedName, exec); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if exec.DeletionTimestamp.IsZero() {
		if exec.Status.State == "" {
			exec.Status.State = operatorv1.ActionExecutionPending
			if err := r.Status().Update(ctx, exec); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	}

	if exec.Status.State != operatorv1.ActionExecutionPending {
		return ctrl.Result{}, nil
	}

	server := &operatorv1.GameServer{}
	serverKey := types.NamespacedName{
		Name:      exec.Spec.TargetRef.Name,
		Namespace: exec.Spec.TargetRef.Namespace,
	}
	if serverKey.Namespace == "" {
		serverKey.Namespace = req.Namespace
	}

	if err := r.Get(ctx, serverKey, server); err != nil {
		if apierrors.IsNotFound(err) {
			r.setState(ctx, exec, operatorv1.ActionExecutionRejected, "", "target GameServer not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	profile := &operatorv1.GameProfile{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Spec.Profile}, profile); err != nil {
		if apierrors.IsNotFound(err) {
			r.setState(ctx, exec, operatorv1.ActionExecutionRejected, "", "referenced GameProfile not found")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	var actionDecl *operatorv1.ActionDecl
	for i := range profile.Spec.Actions {
		if profile.Spec.Actions[i].Name == exec.Spec.ActionName {
			actionDecl = &profile.Spec.Actions[i]
			break
		}
	}
	if actionDecl == nil {
		msg := fmt.Sprintf("action %q not found in profile %q", exec.Spec.ActionName, profile.Name)
		r.setState(ctx, exec, operatorv1.ActionExecutionRejected, "", msg)
		return ctrl.Result{}, nil
	}

	if err := r.validateParams(exec, actionDecl); err != nil {
		r.setState(ctx, exec, operatorv1.ActionExecutionRejected, "", err.Error())
		return ctrl.Result{}, nil
	}

	if canRun, reason := r.checkConcurrency(ctx, exec, server, actionDecl); !canRun {
		r.setState(ctx, exec, operatorv1.ActionExecutionRejected, "", reason)
		return ctrl.Result{}, nil
	}

	timeout := defaultActionTimeout
	if actionDecl.Timeout != "" {
		if d, err := time.ParseDuration(actionDecl.Timeout); err == nil {
			timeout = d
		}
	}

	r.setState(ctx, exec, operatorv1.ActionExecutionRunning, "", "")

	result, err := r.dispatchToAgent(ctx, server, exec, timeout)
	if err != nil {
		r.setState(ctx, exec, operatorv1.ActionExecutionFailed, "", err.Error())
		return ctrl.Result{}, nil
	}

	r.setState(ctx, exec, operatorv1.ActionExecutionSucceeded, result, "")
	logger.Info("action execution completed",
		"action", exec.Spec.ActionName, "server", server.Name, "state", operatorv1.ActionExecutionSucceeded)
	return ctrl.Result{}, nil
}

func (r *ActionExecutionReconciler) validateParams(exec *operatorv1.ActionExecution, decl *operatorv1.ActionDecl) error {
	for name, param := range decl.Params {
		if param.Required {
			if _, ok := exec.Spec.Params[name]; !ok {
				return fmt.Errorf("required param %q missing", name)
			}
		}
	}
	return nil
}

func (r *ActionExecutionReconciler) checkConcurrency(
	ctx context.Context,
	exec *operatorv1.ActionExecution,
	server *operatorv1.GameServer,
	decl *operatorv1.ActionDecl,
) (bool, string) {
	var actionExecList operatorv1.ActionExecutionList
	if err := r.List(ctx, &actionExecList, client.InNamespace(server.Namespace)); err != nil {
		return false, fmt.Sprintf("failed to list action executions: %v", err)
	}

	for _, other := range actionExecList.Items {
		if other.Name == exec.Name {
			continue
		}
		if other.Status.State != operatorv1.ActionExecutionRunning {
			continue
		}
		if other.Spec.TargetRef.Name != server.Name {
			continue
		}

		switch decl.Concurrency {
		case operatorv1.ActionConcurrencyExclusive:
			return false, fmt.Sprintf("exclusive action blocked by running execution %q", other.Name)
		case operatorv1.ActionConcurrencySerialize:
			if other.Spec.ActionName == exec.Spec.ActionName {
				return false, fmt.Sprintf("serialized action blocked by running execution %q", other.Name)
			}
		}
	}

	return true, ""
}

func (r *ActionExecutionReconciler) dispatchToAgent(
	ctx context.Context,
	server *operatorv1.GameServer,
	exec *operatorv1.ActionExecution,
	timeout time.Duration,
) (string, error) {
	svc := &corev1.Service{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, svc); err != nil {
		return "", fmt.Errorf("failed to get service: %w", err)
	}

	addr := fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, builder.AgentGRPCPort)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return "", fmt.Errorf("failed to connect to agent: %w", err)
	}
	defer func() { _ = conn.Close() }()

	agentClient := agentv1.NewAgentClient(conn)
	resp, err := agentClient.ExecuteAction(ctx, &agentv1.ExecuteActionRequest{
		ActionName:  exec.Spec.ActionName,
		Params:      exec.Spec.Params,
		ExecutionId: exec.Name,
	})
	if err != nil {
		return "", fmt.Errorf("agent execution failed: %w", err)
	}

	if resp.State == agentv1.ActionState_ACTION_STATE_FAILED {
		return "", fmt.Errorf("agent reported failure: %s", resp.Error)
	}
	if resp.State == agentv1.ActionState_ACTION_STATE_REJECTED {
		return "", fmt.Errorf("agent rejected action: %s", resp.Error)
	}

	return resp.Result.String(), nil
}

func (r *ActionExecutionReconciler) setState(ctx context.Context, exec *operatorv1.ActionExecution, state operatorv1.ActionExecutionState, result, errMsg string) {
	now := metav1.Now()
	exec.Status.State = state
	if state == operatorv1.ActionExecutionRunning {
		exec.Status.StartedAt = &now
	}
	if state == operatorv1.ActionExecutionSucceeded ||
		state == operatorv1.ActionExecutionFailed ||
		state == operatorv1.ActionExecutionTimedOut ||
		state == operatorv1.ActionExecutionRejected {
		exec.Status.EndedAt = &now
	}
	exec.Status.AgentResponse = result
	exec.Status.Error = errMsg

	condition := metav1.Condition{
		Type:               string(state),
		Status:             metav1.ConditionTrue,
		Reason:             string(state),
		Message:            errMsg,
		ObservedGeneration: exec.Generation,
		LastTransitionTime: now,
	}
	setCondition(&exec.Status.Conditions, condition)

	_ = r.Status().Update(ctx, exec)
}

func (r *ActionExecutionReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.ActionExecution{}).
		Complete(r)
}

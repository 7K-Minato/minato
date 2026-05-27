package controllers

import (
	"context"
	"fmt"
	"maps"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

const (
	gameServerFleetFinalizer = "minato.io/gameserverfleet-finalizer"
)

type GameServerFleetReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.minato.io,resources=gameserverfleets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameserverfleets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameserverfleets/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers,verbs=get;list;watch;create;update;patch;delete

func (r *GameServerFleetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	fleet := &operatorv1.GameServerFleet{}
	if err := r.Get(ctx, req.NamespacedName, fleet); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if fleet.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(fleet, gameServerFleetFinalizer) {
			controllerutil.AddFinalizer(fleet, gameServerFleetFinalizer)
			if err := r.Update(ctx, fleet); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		if controllerutil.ContainsFinalizer(fleet, gameServerFleetFinalizer) {
			if err := r.cleanupFleet(ctx, fleet); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(fleet, gameServerFleetFinalizer)
			if err := r.Update(ctx, fleet); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// List existing GameServers for this fleet
	var existingServers operatorv1.GameServerList
	if err := r.List(ctx, &existingServers,
		client.InNamespace(fleet.Namespace),
		client.MatchingLabels{
			"minato.io/fleet": fleet.Name,
		},
	); err != nil {
		return ctrl.Result{}, err
	}

	desiredReplicas := fleet.Spec.Replicas
	currentReplicas := int32(len(existingServers.Items))

	// Create missing GameServers
	if currentReplicas < desiredReplicas {
		for i := currentReplicas; i < desiredReplicas; i++ {
			server := r.buildGameServer(fleet, i)
			if err := controllerutil.SetControllerReference(fleet, server, r.Scheme); err != nil {
				return ctrl.Result{}, err
			}
			if err := r.Create(ctx, server); err != nil {
				logger.Error(err, "failed to create GameServer", "index", i)
				return ctrl.Result{}, err
			}
		}
	}

	// Delete excess GameServers
	if currentReplicas > desiredReplicas {
		for i := desiredReplicas; i < currentReplicas; i++ {
			server := &existingServers.Items[i]
			if err := r.Delete(ctx, server); err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to delete GameServer", "name", server.Name)
				return ctrl.Result{}, err
			}
		}
	}

	// Update status
	if err := r.updateStatus(ctx, fleet, existingServers.Items); err != nil {
		logger.Error(err, "failed to update fleet status")
	}

	return ctrl.Result{}, nil
}

func (r *GameServerFleetReconciler) buildGameServer(
	fleet *operatorv1.GameServerFleet,
	index int32,
) *operatorv1.GameServer {
	name := fmt.Sprintf("%s-%d", fleet.Name, index)

	labels := map[string]string{
		"app.kubernetes.io/name": "minato",
		"minato.io/fleet":        fleet.Name,
		"minato.io/profile":      fleet.Spec.Profile,
		"minato.io/managed-by":   "gameserverfleet",
	}

	// Merge template labels
	maps.Copy(labels, fleet.Spec.Template.Metadata.Labels)

	annotations := map[string]string{}
	maps.Copy(annotations, fleet.Spec.Template.Metadata.Annotations)

	return &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   fleet.Namespace,
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: operatorv1.GameServerSpec{
			Profile: fleet.Spec.Profile,
			Env:     fleet.Spec.Template.Spec.Env,
		},
	}
}

func (r *GameServerFleetReconciler) updateStatus(
	ctx context.Context,
	fleet *operatorv1.GameServerFleet,
	servers []operatorv1.GameServer,
) error {
	var readyReplicas, updatedReplicas int32

	for _, server := range servers {
		if server.Status.State == "Running" {
			readyReplicas++
		}
		// Simple update detection: check if server matches fleet spec
		if server.Spec.Profile == fleet.Spec.Profile {
			updatedReplicas++
		}
	}

	fleet.Status.Replicas = int32(len(servers))
	fleet.Status.ReadyReplicas = readyReplicas
	fleet.Status.UpdatedReplicas = updatedReplicas

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "Reconciled",
		Message:            fmt.Sprintf("%d/%d replicas ready", readyReplicas, fleet.Spec.Replicas),
		ObservedGeneration: fleet.Generation,
	}
	setCondition(&fleet.Status.Conditions, condition)

	return r.Status().Update(ctx, fleet)
}

func (r *GameServerFleetReconciler) cleanupFleet(ctx context.Context, fleet *operatorv1.GameServerFleet) error {
	logger := log.FromContext(ctx)

	var servers operatorv1.GameServerList
	if err := r.List(ctx, &servers,
		client.InNamespace(fleet.Namespace),
		client.MatchingLabels{
			"minato.io/fleet": fleet.Name,
		},
	); err != nil {
		return err
	}

	for _, server := range servers.Items {
		if err := r.Delete(ctx, &server); err != nil && !apierrors.IsNotFound(err) {
			logger.Error(err, "failed to delete GameServer during fleet cleanup", "name", server.Name)
			return err
		}
	}

	return nil
}

func (r *GameServerFleetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.GameServerFleet{}).
		Owns(&operatorv1.GameServer{}).
		Complete(r)
}

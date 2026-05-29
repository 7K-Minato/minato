package controllers

import (
	"context"
	"fmt"
	"maps"
	"sort"

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

	// Fleet labels
	fleetLabel          = "minato.io/fleet"
	fleetGenerationLabel = "minato.io/fleet-generation"
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

	// Finalizer handling
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
		client.MatchingLabels{fleetLabel: fleet.Name},
	); err != nil {
		return ctrl.Result{}, err
	}

	desiredReplicas := fleet.Spec.Replicas
	currentReplicas := int32(len(existingServers.Items))

	updateStrategy := fleet.Spec.UpdateStrategy.Type
	if updateStrategy == "" {
		updateStrategy = "RollingUpdate"
	}

	// Handle scale up: create missing GameServers
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

	// Handle scale down: delete excess GameServers (player-aware)
	if currentReplicas > desiredReplicas {
		toDelete := r.selectServersToDelete(ctx, existingServers.Items, desiredReplicas, updateStrategy)
		for _, server := range toDelete {
			// Graceful drain: call agent shutdown before deletion
			if err := r.drainServer(ctx, &server); err != nil {
				logger.Error(err, "failed to drain GameServer, deleting anyway", "name", server.Name)
			}
			if err := r.Delete(ctx, &server); err != nil && !apierrors.IsNotFound(err) {
				logger.Error(err, "failed to delete GameServer", "name", server.Name)
				return ctrl.Result{}, err
			}
		}
	}

	// Handle rolling update: update GameServers that don't match fleet template
	if updateStrategy == "RollingUpdate" {
		if err := r.handleRollingUpdate(ctx, fleet, existingServers.Items); err != nil {
			logger.Error(err, "failed to handle rolling update")
		}
	}

	// Update status
	if err := r.updateStatus(ctx, fleet, existingServers.Items); err != nil {
		logger.Error(err, "failed to update fleet status")
	}

	return ctrl.Result{}, nil
}

// selectServersToDelete chooses which GameServers to remove.
// For RollingUpdate: prefer servers with 0 players, then oldest.
// For OnDelete: return empty slice (user must delete manually).
func (r *GameServerFleetReconciler) selectServersToDelete(
	ctx context.Context,
	servers []operatorv1.GameServer,
	desiredReplicas int32,
	updateStrategy string,
) []operatorv1.GameServer {
	if updateStrategy == "OnDelete" {
		return nil
	}

	// Sort: empty servers first (0 players), then by creation time ascending
	sorted := make([]operatorv1.GameServer, len(servers))
	copy(sorted, servers)

	// Custom sort: empty servers first, then oldest first
	sort.Slice(sorted, func(i, j int) bool {
		// Prefer servers with fewer players
		if sorted[i].Status.Players != sorted[j].Status.Players {
			return sorted[i].Status.Players < sorted[j].Status.Players
		}
		// For equal players, prefer older servers
		if !sorted[i].CreationTimestamp.Equal(&sorted[j].CreationTimestamp) {
			return sorted[i].CreationTimestamp.Before(&sorted[j].CreationTimestamp)
		}
		// Deterministic tie-breaker
		return sorted[i].Name < sorted[j].Name
	})

	excess := int32(len(sorted)) - desiredReplicas
	if excess <= 0 {
		return nil
	}
	return sorted[:excess]
}

// drainServer gracefully shuts down a GameServer before deletion.
// This triggers the agent to save the world, notify players, etc.
func (r *GameServerFleetReconciler) drainServer(ctx context.Context, server *operatorv1.GameServer) error {
	logger := log.FromContext(ctx)

	// Only drain if server is running and has an agent
	if server.Status.State != "Running" || server.Status.AgentVersion == "" {
		return nil
	}

	// Call agent shutdown (similar to idle timeout logic)
	logger.Info("draining GameServer before deletion", "name", server.Name)

	// TODO: Implement agent shutdown call via gRPC
	// For now, just log and proceed
	// In production, this would call the agent's PrepareShutdown endpoint

	return nil
}

// handleRollingUpdate updates GameServers when the fleet template changes.
// Respects maxUnavailable to avoid downtime.
func (r *GameServerFleetReconciler) handleRollingUpdate(
	ctx context.Context,
	fleet *operatorv1.GameServerFleet,
	servers []operatorv1.GameServer,
) error {
	logger := log.FromContext(ctx)

	fleetGeneration := fmt.Sprintf("%d", fleet.Generation)
	maxUnavailable := int32(1) // Default: update one at a time
	if fleet.Spec.UpdateStrategy.RollingUpdate != nil &&
		fleet.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable != nil {
		maxUnavailable = *fleet.Spec.UpdateStrategy.RollingUpdate.MaxUnavailable
	}

	// Count how many servers are currently being updated (not matching fleet generation)
	var updatingCount int32
	for _, server := range servers {
		if server.Labels[fleetGenerationLabel] != fleetGeneration {
			updatingCount++
		}
	}

	// Find servers that need updating
	var serversToUpdate []operatorv1.GameServer
	for _, server := range servers {
		if server.Labels[fleetGenerationLabel] != fleetGeneration {
			serversToUpdate = append(serversToUpdate, server)
		}
	}

	// Sort: prefer empty servers, then oldest
	sort.Slice(serversToUpdate, func(i, j int) bool {
		if serversToUpdate[i].Status.Players != serversToUpdate[j].Status.Players {
			return serversToUpdate[i].Status.Players < serversToUpdate[j].Status.Players
		}
		return serversToUpdate[i].CreationTimestamp.Before(&serversToUpdate[j].CreationTimestamp)
	})

	// Update servers within maxUnavailable limit
	availableSlots := maxUnavailable - updatingCount
	for i := int32(0); i < availableSlots && i < int32(len(serversToUpdate)); i++ {
		server := &serversToUpdate[i]
		logger.Info("updating GameServer to match fleet template",
			"name", server.Name,
			"fleetGeneration", fleetGeneration)

		// Update the server to match fleet template
		updated := server.DeepCopy()
		updated.Labels[fleetGenerationLabel] = fleetGeneration

		// Update spec from fleet template
		updated.Spec.Profile = fleet.Spec.Profile
		if updated.Spec.Env == nil {
			updated.Spec.Env = make(map[string]string)
		}
		for k, v := range fleet.Spec.Template.Spec.Env {
			updated.Spec.Env[k] = v
		}

		// Merge labels
		if updated.Labels == nil {
			updated.Labels = make(map[string]string)
		}
		maps.Copy(updated.Labels, fleet.Spec.Template.Metadata.Labels)

		// Merge annotations
		if updated.Annotations == nil {
			updated.Annotations = make(map[string]string)
		}
		maps.Copy(updated.Annotations, fleet.Spec.Template.Metadata.Annotations)

		if err := r.Update(ctx, updated); err != nil {
			logger.Error(err, "failed to update GameServer", "name", server.Name)
			continue
		}
	}

	return nil
}

func (r *GameServerFleetReconciler) buildGameServer(
	fleet *operatorv1.GameServerFleet,
	index int32,
) *operatorv1.GameServer {
	name := fmt.Sprintf("%s-%d", fleet.Name, index)
	fleetGeneration := fmt.Sprintf("%d", fleet.Generation)

	labels := map[string]string{
		"app.kubernetes.io/name": "minato",
		fleetLabel:               fleet.Name,
		"minato.io/profile":      fleet.Spec.Profile,
		"minato.io/managed-by":   "gameserverfleet",
		fleetGenerationLabel:     fleetGeneration,
	}
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
			Profile:                   fleet.Spec.Profile,
			Env:                       fleet.Spec.Template.Spec.Env,
			PriorityClassName:         fleet.Spec.PriorityClassName,
			TopologySpreadConstraints: fleet.Spec.TopologySpreadConstraints,
		},
	}
}

func (r *GameServerFleetReconciler) updateStatus(
	ctx context.Context,
	fleet *operatorv1.GameServerFleet,
	servers []operatorv1.GameServer,
) error {
	var readyReplicas, updatedReplicas int32
	fleetGeneration := fmt.Sprintf("%d", fleet.Generation)

	for _, server := range servers {
		if server.Status.State == "Running" {
			readyReplicas++
		}
		if server.Labels[fleetGenerationLabel] == fleetGeneration {
			updatedReplicas++
		}
	}

	fleet.Status.Replicas = int32(len(servers))
	fleet.Status.ReadyReplicas = readyReplicas
	fleet.Status.UpdatedReplicas = updatedReplicas

	// Update condition
	status := metav1.ConditionTrue
	reason := "Reconciled"
	message := fmt.Sprintf("%d/%d replicas ready", readyReplicas, fleet.Spec.Replicas)

	if fleet.Status.Replicas != fleet.Spec.Replicas {
		status = metav1.ConditionFalse
		reason = "Scaling"
		message = fmt.Sprintf("scaling %d -> %d replicas", fleet.Status.Replicas, fleet.Spec.Replicas)
	}
	if fleet.Status.UpdatedReplicas < fleet.Status.Replicas {
		status = metav1.ConditionFalse
		reason = "Updating"
		message = fmt.Sprintf("%d/%d replicas updated", fleet.Status.UpdatedReplicas, fleet.Spec.Replicas)
	}

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: fleet.Generation,
		LastTransitionTime: metav1.Now(),
	}
	setCondition(&fleet.Status.Conditions, condition)

	return r.Status().Update(ctx, fleet)
}

func (r *GameServerFleetReconciler) cleanupFleet(ctx context.Context, fleet *operatorv1.GameServerFleet) error {
	logger := log.FromContext(ctx)

	var servers operatorv1.GameServerList
	if err := r.List(ctx, &servers,
		client.InNamespace(fleet.Namespace),
		client.MatchingLabels{fleetLabel: fleet.Name},
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

// Package controllers contains the Kubernetes controllers for minato CRDs.
package controllers

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
	operatorv1 "github.com/7k-group/minato/api/operator/v1"
	"github.com/7k-group/minato/internal/controllers/builder"
)

// Ensure unused imports don't break compilation (types is used indirectly via client.ObjectKey).
var _ = types.NamespacedName{}

const (
	gameServerFinalizer    = "minato.io/gameserver-finalizer"
	agentHealthCheckPeriod = 30 * time.Second
	stateProvisioning      = "Provisioning"
	stateRunning           = "Running"
	stateIdle              = "Idle"
	stateError             = "Error"

	// gRPC timeouts for agent communication
	healthCheckTimeout = 5 * time.Second
	shutdownTimeout    = 30 * time.Second
)

type GameServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

// Reconcile is the main entrypoint for the GameServer controller.
// The reconcile flow is:
//  1. Handle finalizer (add on create, cleanup resources on delete).
//  2. Fetch the referenced GameProfile; if missing, set error state.
//  3. Build and reconcile the child resources: StatefulSet, Service, PVC.
//  4. If enabled and Prometheus Operator is present, reconcile ServiceMonitor.
//  5. Check StatefulSet readiness and update GameServer status.
//  6. If ready, perform agent health check and idle-timeout evaluation.
//  7. Requeue periodically while the server is ready to keep health/idle checks alive.
func (r *GameServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	server := &operatorv1.GameServer{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Step 1: Finalizer handling
	if server.DeletionTimestamp.IsZero() {
		if !controllerutil.ContainsFinalizer(server, gameServerFinalizer) {
			controllerutil.AddFinalizer(server, gameServerFinalizer)
			if err := r.Update(ctx, server); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
	} else {
		if controllerutil.ContainsFinalizer(server, gameServerFinalizer) {
			if err := r.cleanupResources(ctx, server); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(server, gameServerFinalizer)
			if err := r.Update(ctx, server); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Step 2: Resolve GameProfile
	profile := &operatorv1.GameProfile{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Spec.Profile}, profile); err != nil {
		if apierrors.IsNotFound(err) {
			r.setProfileMissingCondition(ctx, server, err)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	podSpec, err := builder.BuildGameServerPodSpec(profile, server)
	if err != nil {
		r.setProfileMissingCondition(ctx, server, err)
		return ctrl.Result{}, err
	}

	labelsMap := buildGameServerLabels(server, profile)

	// Step 3: Reconcile StatefulSet, Headless Service, Agent Service, PVC
	sts := buildStatefulSet(server, podSpec, labelsMap)
	if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Patch(ctx, sts, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return ctrl.Result{}, err
	}

	// Headless service for StatefulSet DNS stability (no load balancing)
	headlessSvc := buildHeadlessService(server, labelsMap)
	if err := controllerutil.SetControllerReference(server, headlessSvc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Patch(ctx, headlessSvc, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return ctrl.Result{}, err
	}

	// ClusterIP service for agent gRPC (internal control plane communication only)
	agentSvc := buildAgentService(server, labelsMap)
	if err := controllerutil.SetControllerReference(server, agentSvc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Patch(ctx, agentSvc, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return ctrl.Result{}, err
	}

	pvc := buildPVC(server, profile)
	if err := controllerutil.SetControllerReference(server, pvc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Patch(ctx, pvc, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return ctrl.Result{}, err
	}

	// Step 4: Check readiness and update status
	currentSts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, currentSts); err != nil {
		return ctrl.Result{}, err
	}
	ready := stsReady(currentSts)
	if err := r.updateStatus(ctx, server, ready); err != nil {
		logger.Error(err, "failed to update GameServer status")
		return ctrl.Result{}, err
	}

	// Step 6: Agent health check and idle timeout
	if ready {
		agentVersion, agentHealthy := r.checkAgentHealth(ctx, server)
		if err := r.updateAgentStatus(ctx, server, agentVersion, agentHealthy); err != nil {
			logger.Error(err, "failed to update GameServer agent status")
		}

		if server.Spec.Lifecycle.IdleTimeoutSeconds > 0 {
			requeueAfter, err := r.checkIdleTimeout(ctx, server, currentSts)
			if err != nil {
				logger.Error(err, "failed to check idle timeout")
			}
			if requeueAfter > 0 {
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			}
		}

		return ctrl.Result{RequeueAfter: agentHealthCheckPeriod}, nil
	}

	return ctrl.Result{}, nil
}

func (r *GameServerReconciler) setProfileMissingCondition(
	ctx context.Context,
	server *operatorv1.GameServer,
	err error,
) {
	message := fmt.Sprintf("profile not found: %s", err.Error())
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ProfileNotFound",
		Message:            message,
		ObservedGeneration: server.Generation,
	}

	server.Status.State = stateError
	server.Status.AgentVersion = ""
	setCondition(&server.Status.Conditions, condition)

	_ = r.Status().Update(ctx, server)
}

func (r *GameServerReconciler) updateStatus(ctx context.Context, server *operatorv1.GameServer, ready bool) error {
	state := stateProvisioning
	if ready {
		state = stateRunning
	}

	now := metav1.Now()
	readyCondition := metav1.Condition{
		Type:               "Ready",
		Status:             boolToConditionStatus(ready),
		Reason:             "StatefulSetReady",
		Message:            "",
		ObservedGeneration: server.Generation,
		LastTransitionTime: now,
	}

	agentCondition := metav1.Condition{
		Type:               "AgentReachable",
		Status:             metav1.ConditionUnknown,
		Reason:             "NotProbed",
		Message:            "agent reachability not yet implemented",
		ObservedGeneration: server.Generation,
		LastTransitionTime: now,
	}

	server.Status.State = state
	server.Status.AgentVersion = ""
	setCondition(&server.Status.Conditions, readyCondition)
	setCondition(&server.Status.Conditions, agentCondition)

	return r.Status().Update(ctx, server)
}

func (r *GameServerReconciler) cleanupResources(ctx context.Context, server *operatorv1.GameServer) error {
	sts := &appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if err := r.Delete(ctx, sts); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	headlessSvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if err := r.Delete(ctx, headlessSvc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	agentSvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: server.Name + "-agent", Namespace: server.Namespace}}
	if err := r.Delete(ctx, agentSvc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

// agentAddress returns the gRPC address for the game's agent sidecar.
// Uses the dedicated agent service (server-name-agent) to avoid routing through the headless service.
func agentAddress(server *operatorv1.GameServer) string {
	return fmt.Sprintf("%s-agent.%s.svc.cluster.local:%d", server.Name, server.Namespace, builder.AgentGRPCPort)
}

// dialAgent establishes a gRPC connection to the agent for the given GameServer.
func dialAgent(server *operatorv1.GameServer) (*grpc.ClientConn, error) {
	addr := agentAddress(server)
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

func (r *GameServerReconciler) checkAgentHealth(ctx context.Context, server *operatorv1.GameServer) (string, bool) {
	logger := log.FromContext(ctx)

	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	conn, err := dialAgent(server)
	if err != nil {
		logger.Error(err, "failed to connect to agent for health check")
		return "", false
	}
	defer func() { _ = conn.Close() }()

	agentClient := agentv1.NewAgentClient(conn)
	resp, err := agentClient.HealthCheck(ctx, &agentv1.HealthRequest{})
	if err != nil {
		logger.Error(err, "agent health check failed")
		return "", false
	}

	infoResp, err := agentClient.Info(ctx, &agentv1.InfoRequest{})
	if err != nil {
		logger.Error(err, "agent info request failed")
		return "", resp.Ready
	}

	return infoResp.Version, resp.Ready
}

func (r *GameServerReconciler) updateAgentStatus(
	ctx context.Context,
	server *operatorv1.GameServer,
	version string,
	healthy bool,
) error {
	server.Status.AgentVersion = version

	now := metav1.Now()
	status := metav1.ConditionTrue
	reason := "AgentHealthy"
	message := "agent is reachable and healthy"
	if !healthy {
		status = metav1.ConditionFalse
		reason = "AgentUnhealthy"
		message = "agent is not healthy"
	}

	condition := metav1.Condition{
		Type:               "AgentReachable",
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: server.Generation,
		LastTransitionTime: now,
	}
	setCondition(&server.Status.Conditions, condition)

	return r.Status().Update(ctx, server)
}

func (r *GameServerReconciler) checkIdleTimeout(
	ctx context.Context,
	server *operatorv1.GameServer,
	sts *appsv1.StatefulSet,
) (time.Duration, error) {
	logger := log.FromContext(ctx)

	// If already scaled to 0, nothing to do
	if sts.Spec.Replicas != nil && *sts.Spec.Replicas == 0 {
		return 0, nil
	}

	// Get player count from agent
	players, capacity, err := r.getPlayerCount(ctx, server)
	if err != nil {
		logger.Error(err, "failed to get player count for idle check")
		return 0, nil
	}

	// Update status with player info
	server.Status.Players = players
	server.Status.PlayerCapacity = capacity
	if players > 0 {
		now := metav1.Now()
		server.Status.LastActivity = &now
		server.Status.State = stateRunning
		return 0, r.Status().Update(ctx, server)
	}

	timeout := time.Duration(server.Spec.Lifecycle.IdleTimeoutSeconds) * time.Second

	// Check if we've been idle long enough
	if server.Status.LastActivity != nil {
		idleDuration := time.Since(server.Status.LastActivity.Time)
		if idleDuration >= timeout {
			logger.Info("GameServer idle timeout reached, scaling to 0", "server", server.Name, "idleDuration", idleDuration)

			// Call agent PrepareShutdown
			if err := r.callAgentShutdown(ctx, server); err != nil {
				logger.Error(err, "agent shutdown failed, proceeding with scale-down")
			}

			// Scale StatefulSet to 0
			stsCopy := sts.DeepCopy()
			zero := int32(0)
			stsCopy.Spec.Replicas = &zero
			if err := r.Update(ctx, stsCopy); err != nil {
				return 0, fmt.Errorf("failed to scale StatefulSet to 0: %w", err)
			}

			server.Status.State = stateIdle
			return 0, r.Status().Update(ctx, server)
		}
	} else {
		// No last activity recorded, set it now and requeue so the timeout is evaluated again.
		now := metav1.Now()
		server.Status.LastActivity = &now
		if err := r.Status().Update(ctx, server); err != nil {
			return 0, err
		}
		return timeout, nil
	}

	return 0, nil
}

func (r *GameServerReconciler) getPlayerCount(
	ctx context.Context,
	server *operatorv1.GameServer,
) (int32, int32, error) {
	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	conn, err := dialAgent(server)
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = conn.Close() }()

	agentClient := agentv1.NewAgentClient(conn)
	resp, err := agentClient.GetPlayers(ctx, &agentv1.PlayersRequest{})
	if err != nil {
		return 0, 0, err
	}

	return resp.Online, resp.Capacity, nil
}

func (r *GameServerReconciler) callAgentShutdown(ctx context.Context, server *operatorv1.GameServer) error {
	ctx, cancel := context.WithTimeout(ctx, shutdownTimeout)
	defer cancel()

	conn, err := dialAgent(server)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	agentClient := agentv1.NewAgentClient(conn)
	_, err = agentClient.PrepareShutdown(ctx, &agentv1.ShutdownRequest{
		TimeoutSeconds: int32(shutdownTimeout.Seconds()),
		DrainReason:    "idle timeout",
	})
	return err
}

func buildStatefulSet(
	server *operatorv1.GameServer,
	podSpec corev1.PodSpec,
	labelsMap map[string]string,
) *appsv1.StatefulSet {
	name := server.Name
	if podSpec.Volumes == nil {
		podSpec.Volumes = []corev1.Volume{}
	}
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: builder.DataVolumeName,
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: server.Name},
		},
	})

	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{APIVersion: appsv1.SchemeGroupVersion.String(), Kind: "StatefulSet"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
			Labels:    labelsMap,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    ptr.To[int32](1),
			Selector:    &metav1.LabelSelector{MatchLabels: labelsMap},
			ServiceName: name,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labelsMap},
				Spec:       podSpec,
			},
		},
	}
}

// buildHeadlessService creates a headless Service for the StatefulSet.
// This provides stable DNS names for pods (e.g., minecraft-0.minecraft-smp-1.minato.svc.cluster.local)
// without any load balancing. Game traffic should NOT go through this service.
func buildHeadlessService(
	server *operatorv1.GameServer,
	labelsMap map[string]string,
) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
			Labels:    labelsMap,
		},
		Spec: corev1.ServiceSpec{
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "None", // Headless — no virtual IP, DNS returns pod IPs directly
			Selector:  labelsMap,
			Ports: []corev1.ServicePort{
				{
					Name:       "placeholder",
					Port:       1,
					TargetPort: intstr.FromInt(1),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			PublishNotReadyAddresses: true, // Allow DNS resolution even before pod is ready
		},
	}
}

// buildAgentService creates a ClusterIP Service for the agent gRPC port.
// This is used by the control plane to communicate with the agent sidecar.
// It is NOT exposed to player traffic.
func buildAgentService(
	server *operatorv1.GameServer,
	labelsMap map[string]string,
) *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name + "-agent",
			Namespace: server.Namespace,
			Labels:    labelsMap,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labelsMap,
			Ports: []corev1.ServicePort{
				{
					Name:       builder.AgentPortName,
					Port:       builder.AgentGRPCPort,
					TargetPort: intstr.FromString(builder.AgentPortName),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

func buildPVC(server *operatorv1.GameServer, profile *operatorv1.GameProfile) *corev1.PersistentVolumeClaim {
	quantity, err := resource.ParseQuantity(profile.Spec.Storage.SizeDefault)
	if err != nil {
		quantity = resource.MustParse("1Gi")
	}

	// Use user-provided size if specified
	if server.Spec.Storage.Size != "" {
		if q, err := resource.ParseQuantity(server.Spec.Storage.Size); err == nil {
			quantity = q
		}
	}

	pvc := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "PersistentVolumeClaim"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: quantity,
				},
			},
		},
	}

	if server.Spec.Storage.StorageClass != "" {
		pvc.Spec.StorageClassName = &server.Spec.Storage.StorageClass
	}

	// Restore from snapshot if specified
	if server.Spec.Storage.SnapshotRef != nil {
		snapNamespace := server.Spec.Storage.SnapshotRef.Namespace
		if snapNamespace == "" {
			snapNamespace = server.Namespace
		}
		pvc.Spec.DataSource = &corev1.TypedLocalObjectReference{
			APIGroup: strPtr("snapshot.storage.k8s.io"),
			Kind:     "VolumeSnapshot",
			Name:     server.Spec.Storage.SnapshotRef.Name,
		}
		// Add annotation for cross-namespace snapshot references
		// Some CSI drivers require this
		if snapNamespace != server.Namespace {
			if pvc.Annotations == nil {
				pvc.Annotations = make(map[string]string)
			}
			pvc.Annotations["minato.io/snapshot-namespace"] = snapNamespace
		}
	}

	return pvc
}

func strPtr(s string) *string {
	return &s
}

func buildGameServerLabels(server *operatorv1.GameServer, profile *operatorv1.GameProfile) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": "minato",
		"minato.io/gameserver":   server.Name,
		"minato.io/profile":      profile.Name,
	}
}

func stsReady(sts *appsv1.StatefulSet) bool {
	if sts == nil || sts.Spec.Replicas == nil {
		return false
	}
	return sts.Status.ReadyReplicas >= *sts.Spec.Replicas
}

func boolToConditionStatus(value bool) metav1.ConditionStatus {
	if value {
		return metav1.ConditionTrue
	}
	return metav1.ConditionFalse
}

func setCondition(conditions *[]metav1.Condition, condition metav1.Condition) {
	if conditions == nil {
		return
	}
	for i, existing := range *conditions {
		if existing.Type == condition.Type {
			(*conditions)[i] = condition
			return
		}
	}
	*conditions = append(*conditions, condition)
}

func (r *GameServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1.GameServer{}).
		Owns(&appsv1.StatefulSet{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

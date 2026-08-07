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
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	agentv1 "github.com/7k-minato/minato/api/agent/v1/minato/agent/v1"
	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controllers/builder"
)

// Ensure unused imports don't break compilation (types is used indirectly via client.ObjectKey).
var _ = types.NamespacedName{}

const (
	gameServerFinalizer    = "minato.io/gameserver-finalizer"
	agentHealthCheckPeriod = 30 * time.Second
	stateProvisioning      = "Provisioning"
	stateRunning           = "Running"
	stateIdle              = "Idle"
	stateStopped           = "Stopped"
	stateError             = "Error"

	// gRPC timeouts for agent communication
	healthCheckTimeout = 5 * time.Second
	shutdownTimeout    = 30 * time.Second
)

type GameServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// OperatorNamespace is where the operator runs; image pull secrets are
	// copied from here into GameServer namespaces.
	OperatorNamespace string
	// ImagePullSecrets are secret names to attach to game server pods. Each
	// must exist in OperatorNamespace; the reconciler replicates them into
	// the GameServer's namespace.
	ImagePullSecrets []string
	// ExternalDNSZone, when set, annotates game LoadBalancer services with an
	// external-dns hostname (<server>.<zone>) and publishes that hostname in
	// status.endpoints instead of the raw IP.
	ExternalDNSZone string
	// Recorder emits Kubernetes events (e.g. when ServiceMonitor creation is
	// skipped because the Prometheus Operator is not installed). May be nil
	// in tests.
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create
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
func (r *GameServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (res ctrl.Result, err error) {
	start := time.Now()
	defer func() { observeReconcile("gameserver", start, err) }()
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

	if err := r.ensureImagePullSecrets(ctx, server.Namespace); err != nil {
		return ctrl.Result{}, fmt.Errorf("ensure image pull secrets: %w", err)
	}

	podSpec, err := builder.BuildGameServerPodSpecWithPullSecrets(profile, server, r.ImagePullSecrets)
	if err != nil {
		r.setProfileMissingCondition(ctx, server, err)
		return ctrl.Result{}, err
	}

	labelsMap := buildGameServerLabels(server, profile)

	// SFTP credentials must exist before the pod starts: the sidecar mounts
	// the secret. Creation is idempotent; the password is never rotated.
	if builder.SFTPEnabled(profile) {
		if err := r.ensureSFTPSecret(ctx, server, profile); err != nil {
			return ctrl.Result{}, fmt.Errorf("ensure sftp secret: %w", err)
		}
	}

	// Lifecycle gate: spec.lifecycle.autoStart=false means the server should
	// be stopped. Drain gracefully via the agent before scaling to 0.
	stopping := !server.Spec.Lifecycle.AutoStartEnabled()
	if stopping {
		if err := r.drainBeforeStop(ctx, server); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Step 3: Reconcile StatefulSet, Headless Service, Agent Service, PVC
	if err := r.reconcileChildResources(ctx, server, profile, podSpec, labelsMap, stopping); err != nil {
		return ctrl.Result{}, err
	}

	// Step 4: Check readiness and update status
	currentSts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, currentSts); err != nil {
		return ctrl.Result{}, err
	}
	if stopping {
		// Server is intentionally stopped; report Stopped and skip
		// health/idle checks until autoStart is set back to true.
		if err := r.updateStoppedStatus(ctx, server); err != nil {
			logger.Error(err, "failed to update GameServer status")
			return ctrl.Result{}, err
		}
		recordGameServerMetrics(server)
		return ctrl.Result{}, nil
	}
	ready := stsReady(currentSts)
	// Provisioning duration is observed exactly once per server: when the
	// reconciler first transitions the state to Running from a non-Running
	// prior state. (Idle -> Running re-activation after an idle shutdown is
	// not provisioning; it re-uses an existing PVC and typically starts fast,
	// so it is excluded to keep the histogram about cold starts.)
	priorState := server.Status.State
	if err := r.updateStatus(ctx, server, profile, ready); err != nil {
		logger.Error(err, "failed to update GameServer status")
		return ctrl.Result{}, err
	}
	if ready && priorState == stateProvisioning {
		observeProvisioningDuration(server, time.Since(server.CreationTimestamp.Time))
	}
	recordGameServerMetrics(server)

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
			// checkIdleTimeout refreshes status.players/playerCapacity and may
			// change state to Idle — re-export the updated status.
			recordGameServerMetrics(server)
			if requeueAfter > 0 {
				return ctrl.Result{RequeueAfter: requeueAfter}, nil
			}
		}

		return ctrl.Result{RequeueAfter: agentHealthCheckPeriod}, nil
	}

	return ctrl.Result{}, nil
}

// drainBeforeStop gracefully drains a running server via the agent when the
// lifecycle gate asks for a stop. Drain failures are logged and evented but
// never block the scale-down.
func (r *GameServerReconciler) drainBeforeStop(ctx context.Context, server *operatorv1.GameServer) error {
	logger := log.FromContext(ctx)

	currentSts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, currentSts); err != nil && !apierrors.IsNotFound(err) {
		return err
	} else if err == nil && currentSts.Spec.Replicas != nil && *currentSts.Spec.Replicas > 0 && stsReady(currentSts) {
		if err := r.callAgentShutdown(ctx, server); err != nil {
			logger.Error(err, "agent shutdown failed, proceeding with scale-down")
		}
		if r.Recorder != nil {
			r.Recorder.Event(server, corev1.EventTypeNormal, "AutoStartDisabled",
				"stopping server: spec.lifecycle.autoStart is false")
		}
	}
	return nil
}

// reconcileChildResources applies the StatefulSet, services, PVC and
// ServiceMonitor owned by the GameServer via server-side apply.
func (r *GameServerReconciler) reconcileChildResources(
	ctx context.Context,
	server *operatorv1.GameServer,
	profile *operatorv1.GameProfile,
	podSpec corev1.PodSpec,
	labelsMap map[string]string,
	stopping bool,
) error {
	sts := buildStatefulSet(server, podSpec, labelsMap)
	if stopping {
		zero := int32(0)
		sts.Spec.Replicas = &zero
	}
	if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
		return err
	}
	if err := r.Patch(ctx, sts, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return err
	}

	// Headless service for StatefulSet DNS stability (no load balancing)
	headlessSvc := buildHeadlessService(server, labelsMap)
	if err := controllerutil.SetControllerReference(server, headlessSvc, r.Scheme); err != nil {
		return err
	}
	if err := r.Patch(ctx, headlessSvc, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return err
	}

	// ClusterIP service for agent gRPC (internal control plane communication only)
	agentSvc := buildAgentService(server, labelsMap)
	addAgentMetricsPort(agentSvc, profile)
	if err := controllerutil.SetControllerReference(server, agentSvc, r.Scheme); err != nil {
		return err
	}
	if err := r.Patch(ctx, agentSvc, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return err
	}

	// LoadBalancer service for player traffic (external IP via MetalLB etc.)
	gameSvc := buildGameService(server, profile, labelsMap, r.ExternalDNSZone)
	if err := controllerutil.SetControllerReference(server, gameSvc, r.Scheme); err != nil {
		return err
	}
	if err := r.Patch(ctx, gameSvc, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return err
	}

	pvc := buildPVC(server, profile)
	if err := controllerutil.SetControllerReference(server, pvc, r.Scheme); err != nil {
		return err
	}
	if err := r.Patch(ctx, pvc, client.Apply, client.ForceOwnership, client.FieldOwner("minato-operator")); err != nil {
		return err
	}

	// ServiceMonitor for the agent metrics endpoint, when the profile opts in
	// and the Prometheus Operator is installed (skipped with an event otherwise).
	return r.reconcileServiceMonitor(ctx, server, profile, labelsMap)
}

// ensureImagePullSecrets replicates the configured image pull secrets from the
// operator's namespace into the target namespace so game server pods can pull
// private images.
func (r *GameServerReconciler) ensureImagePullSecrets(ctx context.Context, namespace string) error {
	if len(r.ImagePullSecrets) == 0 || r.OperatorNamespace == "" || namespace == r.OperatorNamespace {
		return nil
	}
	for _, name := range r.ImagePullSecrets {
		src := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: r.OperatorNamespace}, src); err != nil {
			return fmt.Errorf("get pull secret %s/%s: %w", r.OperatorNamespace, name, err)
		}

		dst := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, dst)
		switch {
		case apierrors.IsNotFound(err):
			dst = &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels:    map[string]string{"minato.io/replicated-from": r.OperatorNamespace},
				},
				Type: src.Type,
				Data: src.Data,
			}
			if err := r.Create(ctx, dst); err != nil && !apierrors.IsAlreadyExists(err) {
				return fmt.Errorf("replicate pull secret to %s: %w", namespace, err)
			}
		case err != nil:
			return err
		default:
			if dst.Labels["minato.io/replicated-from"] == "" {
				continue // not managed by us; leave user-provided secret alone
			}
			dst.Type = src.Type
			dst.Data = src.Data
			if err := r.Update(ctx, dst); err != nil {
				return fmt.Errorf("refresh pull secret in %s: %w", namespace, err)
			}
		}
	}
	return nil
}

func (r *GameServerReconciler) setProfileMissingCondition(ctx context.Context,
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

// updateStoppedStatus reports the GameServer as intentionally stopped
// (spec.lifecycle.autoStart=false) and clears player-facing endpoints.
func (r *GameServerReconciler) updateStoppedStatus(ctx context.Context, server *operatorv1.GameServer) error {
	server.Status.State = stateStopped
	server.Status.AgentVersion = ""
	server.Status.Endpoints = nil
	server.Status.Players = 0

	setCondition(&server.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "AutoStartDisabled",
		Message:            "server is stopped: spec.lifecycle.autoStart is false",
		ObservedGeneration: server.Generation,
		LastTransitionTime: metav1.Now(),
	})

	return r.Status().Update(ctx, server)
}

func (r *GameServerReconciler) updateStatus(ctx context.Context, server *operatorv1.GameServer, profile *operatorv1.GameProfile, ready bool) error {
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
	server.Status.Endpoints = r.resolveEndpoints(ctx, server, profile)
	setCondition(&server.Status.Conditions, readyCondition)
	setCondition(&server.Status.Conditions, agentCondition)

	return r.Status().Update(ctx, server)
}

// resolveEndpoints maps exposed profile ports to the player-facing address of
// the game server: the external-dns hostname when a zone is configured,
// otherwise the LoadBalancer ingress IP. Empty until the address exists —
// pod IPs are never published.
func (r *GameServerReconciler) resolveEndpoints(ctx context.Context, server *operatorv1.GameServer, profile *operatorv1.GameProfile) []operatorv1.Endpoint {
	address := ""
	if r.ExternalDNSZone != "" {
		address = server.Name + "." + r.ExternalDNSZone
	} else {
		svc := &corev1.Service{}
		if err := r.Get(ctx, types.NamespacedName{Name: server.Name + "-game", Namespace: server.Namespace}, svc); err != nil {
			return nil
		}
		if len(svc.Status.LoadBalancer.Ingress) == 0 {
			return nil
		}
		ingress := svc.Status.LoadBalancer.Ingress[0]
		address = ingress.IP
		if address == "" {
			address = ingress.Hostname
		}
	}
	if address == "" {
		return nil
	}

	endpoints := make([]operatorv1.Endpoint, 0, len(profile.Spec.Ports))
	for _, p := range profile.Spec.Ports {
		if !p.ExposedPort() {
			continue
		}
		endpoints = append(endpoints, operatorv1.Endpoint{
			Name:    p.Name,
			Address: address,
			Port:    p.ContainerPort,
		})
	}
	if builder.SFTPEnabled(profile) {
		endpoints = append(endpoints, operatorv1.Endpoint{
			Name:    builder.SFTPPortName,
			Address: address,
			Port:    builder.SFTPPort,
		})
	}
	return endpoints
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

	gameSvc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: server.Name + "-game", Namespace: server.Namespace}}
	if err := r.Delete(ctx, gameSvc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	sftpSecret := &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: builder.SFTPSecretName(server.Name), Namespace: server.Namespace}}
	if err := r.Delete(ctx, sftpSecret); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	deleteGameServerMetrics(server.Namespace, server.Name)

	return nil
}

// agentAddress returns the gRPC address for the game's agent sidecar.
// Uses the dedicated agent service (server-name-agent) to avoid routing through the headless service.
func agentAddress(server *operatorv1.GameServer) string {
	return fmt.Sprintf("%s-agent.%s.svc.cluster.local:%d", server.Name, server.Namespace, builder.AgentGRPCPort)
}

// dialAgent establishes a gRPC connection to the agent for the given GameServer.
// Declared as a package-level variable so controller tests can stub agent
// connectivity (e.g. with a bufconn fake agent).
var dialAgent = func(server *operatorv1.GameServer) (*grpc.ClientConn, error) {
	addr := agentAddress(server)
	return grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithConnectParams(grpc.ConnectParams{
			MinConnectTimeout: healthCheckTimeout,
		}),
	)
}

func (r *GameServerReconciler) checkAgentHealth(ctx context.Context, server *operatorv1.GameServer) (string, bool) {
	logger := log.FromContext(ctx)

	ctx, cancel := context.WithTimeout(ctx, healthCheckTimeout)
	defer cancel()

	conn, err := dialAgent(server)
	if err != nil {
		logger.Error(err, "failed to connect to agent for health check")
		recordAgentUnreachable(server.Namespace, server.Name)
		return "", false
	}
	defer func() { _ = conn.Close() }()

	agentClient := agentv1.NewAgentClient(conn)
	resp, err := agentClient.HealthCheck(ctx, &agentv1.HealthRequest{})
	if err != nil {
		logger.Error(err, "agent health check failed")
		recordAgentUnreachable(server.Namespace, server.Name)
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

// buildGameService creates the player-facing LoadBalancer Service for a game
// server. One service port per exposed profile port; the external IP is
// assigned by the cluster's load balancer (e.g. MetalLB) and surfaced via
// status.endpoints. spec.loadBalancerIP pins the address; when dnsZone is
// set, an external-dns hostname annotation is added.
func buildGameService(
	server *operatorv1.GameServer,
	profile *operatorv1.GameProfile,
	labelsMap map[string]string,
	dnsZone string,
) *corev1.Service {
	type protoPort struct {
		port     int32
		protocol corev1.Protocol
	}
	seen := map[protoPort]bool{}
	ports := []corev1.ServicePort{}
	for _, p := range profile.Spec.Ports {
		if !p.ExposedPort() {
			continue
		}
		protocol := p.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		key := protoPort{p.ContainerPort, protocol}
		if seen[key] {
			continue
		}
		seen[key] = true
		ports = append(ports, corev1.ServicePort{
			Name:       p.Name,
			Port:       p.ContainerPort,
			TargetPort: intstr.FromInt32(p.ContainerPort),
			Protocol:   protocol,
		})
	}

	// The SFTP sidecar (profile capability) is exposed on the same
	// player-facing LoadBalancer service; no second LB is created.
	if builder.SFTPEnabled(profile) {
		key := protoPort{builder.SFTPPort, corev1.ProtocolTCP}
		if !seen[key] {
			ports = append(ports, corev1.ServicePort{
				Name:       builder.SFTPPortName,
				Port:       builder.SFTPPort,
				TargetPort: intstr.FromInt32(builder.SFTPPort),
				Protocol:   corev1.ProtocolTCP,
			})
		}
	}

	svc := &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name + "-game",
			Namespace: server.Namespace,
			Labels:    labelsMap,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeLoadBalancer,
			Selector: labelsMap,
			Ports:    ports,
		},
	}
	if server.Spec.LoadBalancerIP != "" {
		svc.Spec.LoadBalancerIP = server.Spec.LoadBalancerIP
	}
	if dnsZone != "" {
		svc.Annotations = map[string]string{
			"external-dns.alpha.kubernetes.io/hostname": server.Name + "." + dnsZone,
		}
	}
	return svc
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
			APIGroup: new("snapshot.storage.k8s.io"),
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

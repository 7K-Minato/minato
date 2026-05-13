package controllers

import (
	"context"
	"fmt"

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

	operatorv1 "github.com/7k-group/minami/api/operator/v1"
	"github.com/7k-group/minami/internal/controllers/builder"
)

const (
	gameServerFinalizer = "minami.io/gameserver-finalizer"
)

type GameServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=operator.minami.io,resources=gameservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.minami.io,resources=gameservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.minami.io,resources=gameservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=operator.minami.io,resources=gameprofiles,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete

func (r *GameServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	server := &operatorv1.GameServer{}
	if err := r.Get(ctx, req.NamespacedName, server); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

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

	sts := buildStatefulSet(server, profile, podSpec, labelsMap)
	if err := controllerutil.SetControllerReference(server, sts, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Patch(ctx, sts, client.Apply, client.ForceOwnership, client.FieldOwner("minami-operator")); err != nil {
		return ctrl.Result{}, err
	}

	svc := buildService(server, profile, labelsMap)
	if err := controllerutil.SetControllerReference(server, svc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Patch(ctx, svc, client.Apply, client.ForceOwnership, client.FieldOwner("minami-operator")); err != nil {
		return ctrl.Result{}, err
	}

	pvc := buildPVC(server, profile)
	if err := controllerutil.SetControllerReference(server, pvc, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.Patch(ctx, pvc, client.Apply, client.ForceOwnership, client.FieldOwner("minami-operator")); err != nil {
		return ctrl.Result{}, err
	}

	currentSts := &appsv1.StatefulSet{}
	if err := r.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, currentSts); err != nil {
		return ctrl.Result{}, err
	}
	ready := stsReady(currentSts)
	if err := r.updateStatus(ctx, server, ready); err != nil {
		logger.Error(err, "failed to update GameServer status")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *GameServerReconciler) setProfileMissingCondition(ctx context.Context, server *operatorv1.GameServer, err error) {
	message := fmt.Sprintf("profile not found: %s", err.Error())
	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ProfileNotFound",
		Message:            message,
		ObservedGeneration: server.Generation,
	}

	server.Status.State = "Error"
	server.Status.AgentVersion = ""
	setCondition(&server.Status.Conditions, condition)

	_ = r.Status().Update(ctx, server)
}

func (r *GameServerReconciler) updateStatus(ctx context.Context, server *operatorv1.GameServer, ready bool) error {
	state := "Provisioning"
	if ready {
		state = "Running"
	}

	readyCondition := metav1.Condition{
		Type:               "Ready",
		Status:             boolToConditionStatus(ready),
		Reason:             "StatefulSetReady",
		Message:            "",
		ObservedGeneration: server.Generation,
	}

	agentCondition := metav1.Condition{
		Type:               "AgentReachable",
		Status:             metav1.ConditionUnknown,
		Reason:             "NotProbed",
		Message:            "agent reachability not yet implemented",
		ObservedGeneration: server.Generation,
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

	svc := &corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if err := r.Delete(ctx, svc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	pvc := &corev1.PersistentVolumeClaim{ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace}}
	if err := r.Delete(ctx, pvc); err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	return nil
}

func buildStatefulSet(server *operatorv1.GameServer, profile *operatorv1.GameProfile, podSpec corev1.PodSpec, labelsMap map[string]string) *appsv1.StatefulSet {
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

func buildService(server *operatorv1.GameServer, profile *operatorv1.GameProfile, labelsMap map[string]string) *corev1.Service {
	ports := make([]corev1.ServicePort, 0, len(profile.Spec.Ports)+1)
	for _, port := range profile.Spec.Ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		ports = append(ports, corev1.ServicePort{
			Name:       port.Name,
			Port:       port.ContainerPort,
			TargetPort: intstr.FromInt32(port.ContainerPort),
			Protocol:   protocol,
		})
	}
	ports = append(ports, corev1.ServicePort{
		Name:       builder.AgentPortName,
		Port:       builder.AgentGRPCPort,
		TargetPort: intstr.FromString(builder.AgentPortName),
		Protocol:   corev1.ProtocolTCP,
	})

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: corev1.SchemeGroupVersion.String(), Kind: "Service"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name,
			Namespace: server.Namespace,
			Labels:    labelsMap,
		},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: labelsMap,
			Ports:    ports,
		},
	}
}

func buildPVC(server *operatorv1.GameServer, profile *operatorv1.GameProfile) *corev1.PersistentVolumeClaim {
	quantity, err := resource.ParseQuantity(profile.Spec.Storage.SizeDefault)
	if err != nil {
		quantity = resource.MustParse("1Gi")
	}

	return &corev1.PersistentVolumeClaim{
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
}

func buildGameServerLabels(server *operatorv1.GameServer, profile *operatorv1.GameProfile) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name": "minami",
		"minami.io/gameserver":   server.Name,
		"minami.io/profile":      profile.Name,
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

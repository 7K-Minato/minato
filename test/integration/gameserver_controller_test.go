package integration

import (
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func TestGameServerReconcileCreatesResources(t *testing.T) {
	k8sClient := k8sManager.GetClient()

	profile := &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "profile"},
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Profile",
			Image:       "example.com/game:latest",
			Ports: []operatorv1.PortSpec{
				{Name: "game", ContainerPort: 7777, Protocol: corev1.ProtocolUDP},
			},
			Storage: operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "1Gi"},
			Agent:   operatorv1.AgentSpec{Image: "busybox", Version: "0.1.0"},
		},
	}
	if err := k8sClient.Create(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	server := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: profile.Name},
	}
	if err := k8sClient.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	sts := &appsv1.StatefulSet{}
	if err := wait.PollUntilContextTimeout(
		ctx, time.Millisecond*200, time.Second*5, true,
		func(ctx context.Context) (bool, error) {
			return k8sClient.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, sts) == nil, nil
		}); err != nil {
		t.Fatalf("statefulset not created: %v", err)
	}
	if len(sts.Spec.Template.Spec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(sts.Spec.Template.Spec.Containers))
	}

	// Check headless service exists (for StatefulSet DNS)
	headlessSvc := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, headlessSvc); err != nil {
		t.Fatalf("headless service not created: %v", err)
	}
	if headlessSvc.Spec.ClusterIP != "None" {
		t.Fatalf("expected headless service (ClusterIP=None), got %s", headlessSvc.Spec.ClusterIP)
	}

	// Check agent service exists (for control plane → agent communication)
	agentSvc := &corev1.Service{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: server.Name + "-agent", Namespace: server.Namespace}, agentSvc); err != nil {
		t.Fatalf("agent service not created: %v", err)
	}

	agentPortFound := false
	for _, port := range agentSvc.Spec.Ports {
		if port.Name == "agent" {
			agentPortFound = true
			if port.TargetPort.String() != "agent" {
				t.Fatalf("expected agent target port name, got %s", port.TargetPort.String())
			}
		}
	}
	if !agentPortFound {
		t.Fatalf("expected agent port on agent service")
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := k8sClient.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, pvc); err != nil {
		t.Fatalf("pvc not created: %v", err)
	}

	if err := bindPVC(ctx, k8sClient, pvc, profile.Spec.Storage.SizeDefault); err != nil {
		t.Fatalf("bind pvc: %v", err)
	}

	if err := k8sClient.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, pvc); err != nil {
		t.Fatalf("get pvc: %v", err)
	}
	if pvc.Status.Phase != corev1.ClaimBound {
		t.Fatalf("expected pvc bound, got %s", pvc.Status.Phase)
	}
}

func TestGameServerFinalizerCleanup(t *testing.T) {
	k8sClient := k8sManager.GetClient()

	profile := &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "profile-cleanup"},
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Profile",
			Image:       "example.com/game:latest",
			Storage:     operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "1Gi"},
			Agent:       operatorv1.AgentSpec{Image: "busybox", Version: "0.1.0"},
		},
	}
	if err := k8sClient.Create(ctx, profile); err != nil {
		t.Fatalf("create profile: %v", err)
	}

	server := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "server-cleanup", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: profile.Name},
	}
	if err := k8sClient.Create(ctx, server); err != nil {
		t.Fatalf("create server: %v", err)
	}

	if err := k8sClient.Delete(ctx, server); err != nil {
		t.Fatalf("delete server: %v", err)
	}

	if err := wait.PollUntilContextTimeout(
		ctx, time.Millisecond*200, time.Second*10, true,
		func(ctx context.Context) (bool, error) {
			gs := &operatorv1.GameServer{}
			err := k8sClient.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, gs)
			return err != nil, nil
		}); err != nil {
		t.Fatalf("gameserver not deleted: %v", err)
	}
}

func bindPVC(ctx context.Context, c client.Client, pvc *corev1.PersistentVolumeClaim, size string) error {
	pv := &corev1.PersistentVolume{}
	pv.Name = "pv-" + pvc.Name
	pv.Spec = corev1.PersistentVolumeSpec{
		Capacity: corev1.ResourceList{
			corev1.ResourceStorage: resource.MustParse(size),
		},
		AccessModes:                   []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
		ClaimRef: &corev1.ObjectReference{
			Namespace: pvc.Namespace,
			Name:      pvc.Name,
		},
		PersistentVolumeSource: corev1.PersistentVolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: "/tmp/" + pvc.Name},
		},
	}
	if err := c.Create(ctx, pv); err != nil {
		return err
	}

	// Retry update to handle conflicts with controller's server-side apply
	return wait.PollUntilContextTimeout(
		ctx, time.Millisecond*100, time.Second*2, true,
		func(ctx context.Context) (bool, error) {
			current := &corev1.PersistentVolumeClaim{}
			if err := c.Get(ctx, types.NamespacedName{Name: pvc.Name, Namespace: pvc.Namespace}, current); err != nil {
				return false, err
			}
			current.Spec.VolumeName = pv.Name
			if err := c.Update(ctx, current); err != nil {
				return false, nil // retry on conflict
			}

			current.Status.Phase = corev1.ClaimBound
			if err := c.Status().Update(ctx, current); err != nil {
				return false, nil // retry on conflict
			}

			pv.Status.Phase = corev1.VolumeBound
			if err := c.Status().Update(ctx, pv); err != nil {
				return false, nil
			}
			return true, nil
		})
}

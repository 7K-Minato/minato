package webhook

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

var gameserverlog = logf.Log.WithName("gameserver-webhook")

// +kubebuilder:webhook:path=/validate-operator-minato-io-v1-gameserver,mutating=false,failurePolicy=fail,sideEffects=None,groups=operator.minato.io,resources=gameservers,verbs=create;update,versions=v1,name=vgameserver.kb.io,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=operator.minato.io,resources=gameprofiles,verbs=get;list;watch

type GameServerValidator struct {
	Client client.Client
}

func (v *GameServerValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &operatorv1.GameServer{}).
		WithValidator(v).
		Complete()
}

func (v *GameServerValidator) ValidateCreate(ctx context.Context, server *operatorv1.GameServer) (admission.Warnings, error) {
	gameserverlog.Info("validate create", "name", server.Name, "namespace", server.Namespace)
	return v.validateGameServer(ctx, server)
}

func (v *GameServerValidator) ValidateUpdate(ctx context.Context, oldServer, newServer *operatorv1.GameServer) (admission.Warnings, error) {
	gameserverlog.Info("validate update", "name", newServer.Name, "namespace", newServer.Namespace)
	return v.validateGameServer(ctx, newServer)
}

func (v *GameServerValidator) ValidateDelete(ctx context.Context, server *operatorv1.GameServer) (admission.Warnings, error) {
	return nil, nil
}

func (v *GameServerValidator) validateGameServer(ctx context.Context, server *operatorv1.GameServer) (admission.Warnings, error) {
	// Validate profile reference exists
	if server.Spec.Profile == "" {
		return nil, fmt.Errorf("spec.profile is required")
	}

	profile := &operatorv1.GameProfile{}
	if err := v.Client.Get(ctx, types.NamespacedName{Name: server.Spec.Profile}, profile); err != nil {
		return nil, fmt.Errorf("spec.profile %q not found: %w", server.Spec.Profile, err)
	}

	// Validate storage snapshot reference if provided
	if server.Spec.Storage.SnapshotRef != nil {
		if server.Spec.Storage.SnapshotRef.Name == "" {
			return nil, fmt.Errorf("spec.storage.snapshotRef.name is required when snapshotRef is set")
		}
	}

	return nil, nil
}

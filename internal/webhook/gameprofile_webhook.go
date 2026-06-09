package webhook

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

var gameprofilelog = logf.Log.WithName("gameprofile-webhook")

// +kubebuilder:webhook:path=/validate-operator-minato-io-v1-gameprofile,mutating=false,failurePolicy=fail,sideEffects=None,groups=operator.minato.io,resources=gameprofiles,verbs=create;update,versions=v1,name=vgameprofile.kb.io,admissionReviewVersions=v1

type GameProfileValidator struct{}

func (v *GameProfileValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &operatorv1.GameProfile{}).
		WithValidator(v).
		Complete()
}

func (v *GameProfileValidator) ValidateCreate(ctx context.Context, profile *operatorv1.GameProfile) (admission.Warnings, error) {
	gameprofilelog.Info("validate create", "name", profile.Name)
	return v.validateGameProfile(profile)
}

func (v *GameProfileValidator) ValidateUpdate(ctx context.Context, oldProfile, newProfile *operatorv1.GameProfile) (admission.Warnings, error) {
	gameprofilelog.Info("validate update", "name", newProfile.Name)
	return v.validateGameProfile(newProfile)
}

func (v *GameProfileValidator) ValidateDelete(ctx context.Context, profile *operatorv1.GameProfile) (admission.Warnings, error) {
	return nil, nil
}

func (v *GameProfileValidator) validateGameProfile(profile *operatorv1.GameProfile) (admission.Warnings, error) {
	// Validate required fields
	if profile.Spec.Image == "" {
		return nil, fmt.Errorf("spec.image is required")
	}

	if profile.Spec.DisplayName == "" {
		return nil, fmt.Errorf("spec.displayName is required")
	}

	if profile.Spec.Storage.MountPath == "" {
		return nil, fmt.Errorf("spec.storage.mountPath is required")
	}

	if profile.Spec.Storage.SizeDefault == "" {
		return nil, fmt.Errorf("spec.storage.sizeDefault is required")
	}

	if profile.Spec.Agent.Image == "" {
		return nil, fmt.Errorf("spec.agent.image is required")
	}

	// Validate port configurations
	for i, port := range profile.Spec.Ports {
		if port.Name == "" {
			return nil, fmt.Errorf("spec.ports[%d].name is required", i)
		}
		if port.ContainerPort <= 0 || port.ContainerPort > 65535 {
			return nil, fmt.Errorf("spec.ports[%d].containerPort must be between 1 and 65535", i)
		}
	}

	return nil, nil
}

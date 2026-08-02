package webhook

import (
	"context"
	"fmt"
	"regexp"

	"k8s.io/apimachinery/pkg/api/resource"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

var gameprofilelog = logf.Log.WithName("gameprofile-webhook")

var tierNamePattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

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

	// Validate resource tiers
	if err := validateResourceTiers(profile); err != nil {
		return nil, err
	}

	// Validate storage size bounds
	if err := validateStorageSizes(profile); err != nil {
		return nil, err
	}

	return nil, nil
}

func validateStorageSizes(profile *operatorv1.GameProfile) error {
	s := profile.Spec.Storage
	def, err := resource.ParseQuantity(s.SizeDefault)
	if err != nil {
		return fmt.Errorf("spec.storage.sizeDefault %q is not a valid quantity: %w", s.SizeDefault, err)
	}
	if s.SizeMin != "" {
		min, err := resource.ParseQuantity(s.SizeMin)
		if err != nil {
			return fmt.Errorf("spec.storage.sizeMin %q is not a valid quantity: %w", s.SizeMin, err)
		}
		if def.Cmp(min) < 0 {
			return fmt.Errorf("spec.storage.sizeDefault %q is below sizeMin %q", s.SizeDefault, s.SizeMin)
		}
	}
	if s.SizeMax != "" {
		max, err := resource.ParseQuantity(s.SizeMax)
		if err != nil {
			return fmt.Errorf("spec.storage.sizeMax %q is not a valid quantity: %w", s.SizeMax, err)
		}
		if def.Cmp(max) > 0 {
			return fmt.Errorf("spec.storage.sizeDefault %q exceeds sizeMax %q", s.SizeDefault, s.SizeMax)
		}
		if s.SizeMin != "" {
			min, _ := resource.ParseQuantity(s.SizeMin)
			if min.Cmp(max) > 0 {
				return fmt.Errorf("spec.storage.sizeMin %q exceeds sizeMax %q", s.SizeMin, s.SizeMax)
			}
		}
	}
	return nil
}

func validateResourceTiers(profile *operatorv1.GameProfile) error {
	resources := profile.Spec.Resources

	if len(resources.Tiers) == 0 {
		if resources.Default != "" {
			return fmt.Errorf("spec.resources.default %q has no matching tier: spec.resources.tiers is empty", resources.Default)
		}
		return nil
	}

	for name := range resources.Tiers {
		if len(name) > 32 || !tierNamePattern.MatchString(name) {
			return fmt.Errorf("spec.resources.tiers[%q]: tier names must be lowercase alphanumeric or dashes, max 32 characters", name)
		}
	}

	if resources.Default == "" {
		return fmt.Errorf("spec.resources.default is required when spec.resources.tiers is set")
	}
	if _, ok := resources.Tiers[resources.Default]; !ok {
		return fmt.Errorf("spec.resources.default %q must name an existing tier", resources.Default)
	}

	return nil
}

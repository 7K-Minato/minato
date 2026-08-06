package webhook

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func validGameProfile() *operatorv1.GameProfile {
	return &operatorv1.GameProfile{
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Test Game",
			Image:       "example.com/game:latest",
			Storage: operatorv1.StorageSpec{
				MountPath:   "/data",
				SizeDefault: "1Gi",
			},
			Agent: operatorv1.AgentSpec{
				Image:   "example.com/agent:latest",
				Version: "0.1.0",
			},
		},
	}
}

func tier(cpu string) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse(cpu)},
	}
}

func TestValidateGameProfileResourceTiers(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*operatorv1.GameProfile)
		wantError string
	}{
		{
			name:   "no tiers and no default is valid",
			mutate: func(p *operatorv1.GameProfile) {},
		},
		{
			name: "flat resources without tiers is valid",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Resources.ResourceRequirements = tier("100m")
			},
		},
		{
			name: "tiers with valid default is valid",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Resources.Tiers = map[string]corev1.ResourceRequirements{
					"small": tier("250m"),
					"large": tier("2"),
				}
				p.Spec.Resources.Default = "small"
			},
		},
		{
			name: "tiers without default is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Resources.Tiers = map[string]corev1.ResourceRequirements{
					"small": tier("250m"),
				}
			},
			wantError: "spec.resources.default is required",
		},
		{
			name: "default referencing unknown tier is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Resources.Tiers = map[string]corev1.ResourceRequirements{
					"small": tier("250m"),
				}
				p.Spec.Resources.Default = "medium"
			},
			wantError: "must name an existing tier",
		},
		{
			name: "default without tiers is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Resources.Default = "small"
			},
			wantError: "tiers is empty",
		},
		{
			name: "tier name with uppercase is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Resources.Tiers = map[string]corev1.ResourceRequirements{
					"Small": tier("250m"),
				}
				p.Spec.Resources.Default = "Small"
			},
			wantError: "lowercase alphanumeric or dashes",
		},
		{
			name: "tier name over 32 characters is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				long := "a123456789012345678901234567890123"
				p.Spec.Resources.Tiers = map[string]corev1.ResourceRequirements{
					long: tier("250m"),
				}
				p.Spec.Resources.Default = long
			},
			wantError: "max 32 characters",
		},
		{
			name: "tier name with dashes is valid",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Resources.Tiers = map[string]corev1.ResourceRequirements{
					"extra-large": tier("4"),
				}
				p.Spec.Resources.Default = "extra-large"
			},
		},
	}

	validator := &GameProfileValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := validGameProfile()
			tt.mutate(profile)
			_, err := validator.validateGameProfile(profile)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantError)
			}
			if got := err.Error(); !strings.Contains(got, tt.wantError) {
				t.Fatalf("expected error containing %q, got %q", tt.wantError, got)
			}
		})
	}
}

func TestValidateGameProfileStorageSizes(t *testing.T) {
	tests := []struct {
		name      string
		mutate    func(*operatorv1.GameProfile)
		wantError string
	}{
		{
			name:   "no bounds is valid",
			mutate: func(p *operatorv1.GameProfile) {},
		},
		{
			name: "default within bounds is valid",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Storage.SizeMin = "512Mi"
				p.Spec.Storage.SizeMax = "10Gi"
			},
		},
		{
			name: "invalid sizeDefault is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Storage.SizeDefault = "lots"
			},
			wantError: "spec.storage.sizeDefault",
		},
		{
			name: "invalid sizeMin is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Storage.SizeMin = "abc"
			},
			wantError: "spec.storage.sizeMin",
		},
		{
			name: "invalid sizeMax is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Storage.SizeMax = "abc"
			},
			wantError: "spec.storage.sizeMax",
		},
		{
			name: "default below min is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Storage.SizeMin = "5Gi"
			},
			wantError: "below sizeMin",
		},
		{
			name: "default above max is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Storage.SizeMax = "512Mi"
			},
			wantError: "exceeds sizeMax",
		},
		{
			name: "min above max is rejected",
			mutate: func(p *operatorv1.GameProfile) {
				p.Spec.Storage.SizeDefault = "5Gi"
				p.Spec.Storage.SizeMin = "10Gi"
				p.Spec.Storage.SizeMax = "1Gi"
			},
			wantError: "sizeMin",
		},
	}

	validator := &GameProfileValidator{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := validGameProfile()
			tt.mutate(profile)
			_, err := validator.validateGameProfile(profile)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantError)
			}
			if got := err.Error(); !strings.Contains(got, tt.wantError) {
				t.Fatalf("expected error containing %q, got %q", tt.wantError, got)
			}
		})
	}
}

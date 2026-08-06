package builder

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func TestBuildGameServerPodSpec(t *testing.T) {
	profile := &operatorv1.GameProfile{
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Test",
			Image:       "example.com/game:latest",
			Ports: []operatorv1.PortSpec{
				{Name: "game", ContainerPort: 7777, Protocol: corev1.ProtocolUDP},
			},
			Environment: []operatorv1.EnvironmentSpec{
				{Key: "EULA", Default: "true", Required: true},
				{Key: "OPTIONAL", Default: "value", Required: false},
			},
			Resources: operatorv1.ResourcesSpec{
				ResourceRequirements: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			},
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

	server := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "server-1"},
		Spec: operatorv1.GameServerSpec{
			Profile: "test",
			Env: map[string]string{
				"OPTIONAL": "override",
				"CUSTOM":   "custom",
			},
		},
	}

	podSpec, err := BuildGameServerPodSpec(profile, server)
	if err != nil {
		t.Fatalf("BuildGameServerPodSpec returned error: %v", err)
	}

	if len(podSpec.Containers) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(podSpec.Containers))
	}

	var game corev1.Container
	var agent corev1.Container
	for _, container := range podSpec.Containers {
		switch container.Name {
		case GameContainerName:
			game = container
		case AgentContainerName:
			agent = container
		}
	}
	if game.Name != GameContainerName {
		t.Fatalf("expected game container name %q", GameContainerName)
	}
	if game.Image != profile.Spec.Image {
		t.Fatalf("expected game image %q, got %q", profile.Spec.Image, game.Image)
	}
	if len(game.Ports) != 1 || game.Ports[0].ContainerPort != 7777 {
		t.Fatalf("expected game port 7777, got %#v", game.Ports)
	}
	if game.Resources.Requests.Cpu().String() != "100m" {
		t.Fatalf("expected cpu request 100m, got %s", game.Resources.Requests.Cpu().String())
	}

	if agent.Name != AgentContainerName {
		t.Fatalf("expected agent container name %q", AgentContainerName)
	}
	if agent.Image != profile.Spec.Agent.Image {
		t.Fatalf("expected agent image %q, got %q", profile.Spec.Agent.Image, agent.Image)
	}
	if len(agent.Ports) != 1 || agent.Ports[0].ContainerPort != AgentGRPCPort || agent.Ports[0].Name != AgentPortName {
		t.Fatalf("expected agent port %d named %s, got %#v", AgentGRPCPort, AgentPortName, agent.Ports)
	}
	agentEnv := map[string]string{}
	for _, item := range agent.Env {
		agentEnv[item.Name] = item.Value
	}
	if agentEnv["minato_GAMESERVER_NAME"] != "server-1" {
		t.Fatalf("expected minato_GAMESERVER_NAME, got %q", agentEnv["minato_GAMESERVER_NAME"])
	}
	if agentEnv["minato_GAMESERVER_NAMESPACE"] != "default" {
		t.Fatalf("expected minato_GAMESERVER_NAMESPACE, got %q", agentEnv["minato_GAMESERVER_NAMESPACE"])
	}
	if agentEnv["minato_GAME_CONTAINER"] != GameContainerName {
		t.Fatalf("expected minato_GAME_CONTAINER, got %q", agentEnv["minato_GAME_CONTAINER"])
	}
	if agentEnv["EULA"] != "true" {
		t.Fatalf("expected game env forwarded to agent, got %q", agentEnv["EULA"])
	}
	if agentEnv["CUSTOM"] != "custom" {
		t.Fatalf("expected server env forwarded to agent, got %q", agentEnv["CUSTOM"])
	}

	env := map[string]string{}
	for _, item := range game.Env {
		env[item.Name] = item.Value
	}
	if env["EULA"] != "true" {
		t.Fatalf("expected EULA env, got %q", env["EULA"])
	}
	if env["OPTIONAL"] != "override" {
		t.Fatalf("expected OPTIONAL override, got %q", env["OPTIONAL"])
	}
	if env["CUSTOM"] != "custom" {
		t.Fatalf("expected CUSTOM env, got %q", env["CUSTOM"])
	}
}

func TestResolveGameResources(t *testing.T) {
	flat := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("100m")},
	}
	small := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("250m")},
	}
	large := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("2")},
	}

	profileWithTiers := func() *operatorv1.GameProfile {
		return &operatorv1.GameProfile{
			Spec: operatorv1.GameProfileSpec{
				Resources: operatorv1.ResourcesSpec{
					ResourceRequirements: flat,
					Default:              "small",
					Tiers: map[string]corev1.ResourceRequirements{
						"small": small,
						"large": large,
					},
				},
			},
		}
	}

	tests := []struct {
		name    string
		profile *operatorv1.GameProfile
		tier    string
		wantCPU string
	}{
		{name: "no tiers uses flat resources", profile: &operatorv1.GameProfile{Spec: operatorv1.GameProfileSpec{Resources: operatorv1.ResourcesSpec{ResourceRequirements: flat}}}, tier: "", wantCPU: "100m"},
		{name: "explicit tier", profile: profileWithTiers(), tier: "large", wantCPU: "2"},
		{name: "no tier falls back to default tier", profile: profileWithTiers(), tier: "", wantCPU: "250m"},
		{name: "unknown tier falls back to flat resources", profile: profileWithTiers(), tier: "huge", wantCPU: "100m"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &operatorv1.GameServer{Spec: operatorv1.GameServerSpec{Tier: tt.tier}}
			got := ResolveGameResources(tt.profile, server)
			if got.Requests.Cpu().String() != tt.wantCPU {
				t.Fatalf("expected cpu request %s, got %s", tt.wantCPU, got.Requests.Cpu().String())
			}
		})
	}
}

func TestBuildGameServerPodSpecWithPullSecrets(t *testing.T) {
	profile := &operatorv1.GameProfile{
		Spec: operatorv1.GameProfileSpec{
			Image:   "game:latest",
			Storage: operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "1Gi"},
			Agent:   operatorv1.AgentSpec{Image: "agent:latest"},
		},
	}
	server := &operatorv1.GameServer{}

	podSpec, err := BuildGameServerPodSpecWithPullSecrets(profile, server, []string{"harbor-docker-pull", "other"})
	if err != nil {
		t.Fatalf("BuildGameServerPodSpecWithPullSecrets returned error: %v", err)
	}
	if len(podSpec.ImagePullSecrets) != 2 ||
		podSpec.ImagePullSecrets[0].Name != "harbor-docker-pull" ||
		podSpec.ImagePullSecrets[1].Name != "other" {
		t.Fatalf("unexpected imagePullSecrets: %#v", podSpec.ImagePullSecrets)
	}

	podSpec, err = BuildGameServerPodSpec(profile, server)
	if err != nil {
		t.Fatalf("BuildGameServerPodSpec returned error: %v", err)
	}
	if len(podSpec.ImagePullSecrets) != 0 {
		t.Fatalf("expected no imagePullSecrets, got %#v", podSpec.ImagePullSecrets)
	}
}

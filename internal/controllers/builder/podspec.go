package builder

import (
	"fmt"
	"sort"

	corev1 "k8s.io/api/core/v1"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

const (
	GameContainerName  = "minato-game"
	AgentContainerName = "minato-agent"
	DataVolumeName     = "data"
	AgentGRPCPort      = 9876
	AgentPortName      = "agent"
)

func BuildGameServerPodSpec(profile *operatorv1.GameProfile, server *operatorv1.GameServer) (corev1.PodSpec, error) {
	return BuildGameServerPodSpecWithPullSecrets(profile, server, nil)
}

// BuildGameServerPodSpecWithPullSecrets builds the pod spec for a GameServer,
// attaching the given image pull secrets (by name, must exist in the
// GameServer's namespace) to the pod.
func BuildGameServerPodSpecWithPullSecrets(profile *operatorv1.GameProfile, server *operatorv1.GameServer, pullSecrets []string) (corev1.PodSpec, error) {
	if profile.Spec.Storage.MountPath == "" {
		return corev1.PodSpec{}, fmt.Errorf("storage.mountPath is required")
	}
	if profile.Spec.Image == "" {
		return corev1.PodSpec{}, fmt.Errorf("image is required")
	}
	if profile.Spec.Agent.Image == "" {
		return corev1.PodSpec{}, fmt.Errorf("agent.image is required")
	}
	gameEnv := buildGameEnv(profile, server)
	gamePorts := buildGamePorts(profile)

	gameContainer := corev1.Container{
		Name:         GameContainerName,
		Image:        profile.Spec.Image,
		Ports:        gamePorts,
		Env:          gameEnv,
		Resources:    ResolveGameResources(profile, server),
		VolumeMounts: buildDataVolumeMounts(profile),
	}

	agentContainer := corev1.Container{
		Name:  AgentContainerName,
		Image: profile.Spec.Agent.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          AgentPortName,
				ContainerPort: AgentGRPCPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		// The agent sees the same game environment (plus its own minato_* vars) —
		// agents typically need game config such as RCON credentials.
		Env: append(gameEnv,
			corev1.EnvVar{Name: "minato_GAMESERVER_NAME", Value: server.Name},
			corev1.EnvVar{Name: "minato_GAMESERVER_NAMESPACE", Value: server.Namespace},
			corev1.EnvVar{Name: "minato_GAME_CONTAINER", Value: GameContainerName},
		),
		VolumeMounts: buildDataVolumeMounts(profile),
	}

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{gameContainer, agentContainer},
	}

	if SFTPEnabled(profile) {
		applySFTPSidecar(&podSpec, profile, server)
	}

	for _, name := range pullSecrets {
		podSpec.ImagePullSecrets = append(podSpec.ImagePullSecrets, corev1.LocalObjectReference{Name: name})
	}

	if server.Spec.PriorityClassName != "" {
		podSpec.PriorityClassName = server.Spec.PriorityClassName
	}

	if len(server.Spec.TopologySpreadConstraints) > 0 {
		podSpec.TopologySpreadConstraints = server.Spec.TopologySpreadConstraints
	}

	return podSpec, nil
}

// ResolveGameResources returns the ResourceRequirements for the game
// container, honoring the tier selected by the GameServer. Falls back to the
// profile's default tier, then to the flat inline resources, when the server
// requests no tier or an unknown tier.
func ResolveGameResources(profile *operatorv1.GameProfile, server *operatorv1.GameServer) corev1.ResourceRequirements {
	return profile.Spec.Resources.ForTier(server.Spec.Tier)
}

func buildGameEnv(profile *operatorv1.GameProfile, server *operatorv1.GameServer) []corev1.EnvVar {
	values := map[string]string{}
	keys := map[string]struct{}{}

	for _, item := range profile.Spec.Environment {
		if item.Required || item.Default != "" {
			values[item.Key] = item.Default
			keys[item.Key] = struct{}{}
		}
	}

	for key, value := range server.Spec.Env {
		values[key] = value
		keys[key] = struct{}{}
	}

	ordered := make([]string, 0, len(keys))
	for key := range keys {
		ordered = append(ordered, key)
	}
	sort.Strings(ordered)

	env := make([]corev1.EnvVar, 0, len(ordered))
	for _, key := range ordered {
		env = append(env, corev1.EnvVar{Name: key, Value: values[key]})
	}

	return env
}

func buildGamePorts(profile *operatorv1.GameProfile) []corev1.ContainerPort {
	ports := make([]corev1.ContainerPort, 0, len(profile.Spec.Ports))
	for _, port := range profile.Spec.Ports {
		protocol := port.Protocol
		if protocol == "" {
			protocol = corev1.ProtocolTCP
		}
		ports = append(ports, corev1.ContainerPort{
			Name:          port.Name,
			ContainerPort: port.ContainerPort,
			Protocol:      protocol,
		})
	}
	return ports
}

func buildDataVolumeMounts(profile *operatorv1.GameProfile) []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{
			Name:      DataVolumeName,
			MountPath: profile.Spec.Storage.MountPath,
		},
	}
}

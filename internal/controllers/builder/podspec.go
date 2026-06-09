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
		Resources:    profile.Spec.Resources,
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
		Env: []corev1.EnvVar{
			{Name: "minato_GAMESERVER_NAME", Value: server.Name},
			{Name: "minato_GAMESERVER_NAMESPACE", Value: server.Namespace},
			{Name: "minato_GAME_CONTAINER", Value: GameContainerName},
		},
		VolumeMounts: buildDataVolumeMounts(profile),
	}

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{gameContainer, agentContainer},
	}

	if server.Spec.PriorityClassName != "" {
		podSpec.PriorityClassName = server.Spec.PriorityClassName
	}

	if len(server.Spec.TopologySpreadConstraints) > 0 {
		podSpec.TopologySpreadConstraints = server.Spec.TopologySpreadConstraints
	}

	return podSpec, nil
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

package builder

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

const (
	SFTPContainerName = "minato-sftp"
	SFTPPortName      = "sftp"
	SFTPPort          = 2022
	SFTPUsername      = "minato"
	// SFTPImage is the pinned SFTPGo image. The alpine variant runs as
	// USER 1000:1000 by default, needs no root or extra capabilities, and is
	// configured entirely via environment variables, which makes it
	// PSS-restricted compliant.
	SFTPImage = "drakkan/sftpgo:v2.6.6-alpine"

	sftpCredentialsVolumeName = "sftp-credentials"
	sftpConfigVolumeName      = "sftpgo-config"
	sftpCredentialsMountPath  = "/etc/sftpgo-secrets"
	sftpConfigMountPath       = "/etc/sftpgo"

	// SFTPUsersKey is the Secret key holding the SFTPGo users dump.
	SFTPUsersKey = "users.json"
	// SFTPPasswordKey is the Secret key holding the SFTP password.
	SFTPPasswordKey = "password"
	// SFTPUsernameKey is the Secret key holding the SFTP username.
	SFTPUsernameKey = "username"
)

// SFTPEnabled reports whether the profile opts into the SFTP sidecar.
func SFTPEnabled(profile *operatorv1.GameProfile) bool {
	return profile.Spec.Capabilities != nil && profile.Spec.Capabilities.SFTP
}

// SFTPSecretName returns the name of the per-server SFTP credentials secret.
func SFTPSecretName(serverName string) string {
	return "minato-" + serverName + "-sftp"
}

// SFTPUsersJSON renders the SFTPGo users dump for the given world mount path.
// passwordHash must be a bcrypt hash ($2a$...): SFTPGo does NOT hash plaintext
// passwords when restoring a dump, so the hash is computed at Secret creation
// time and embedded here directly.
func SFTPUsersJSON(mountPath, passwordHash string) string {
	return fmt.Sprintf(`{
  "version": 17,
  "users": [
    {
      "status": 1,
      "username": %q,
      "password": %q,
      "home_dir": %q,
      "permissions": {"/": ["*"]}
    }
  ]
}
`, SFTPUsername, passwordHash, mountPath)
}

// buildSFTPContainer builds the PSS-restricted compliant SFTPGo sidecar. It
// mounts the same world PVC as the game container at the profile's
// storage.mountPath and serves SFTP on port 2022.
func buildSFTPContainer(profile *operatorv1.GameProfile) corev1.Container {
	return corev1.Container{
		Name:  SFTPContainerName,
		Image: SFTPImage,
		Command: []string{
			"sftpgo", "serve",
			// --loaddata-from preloads the users dump (with a pre-hashed
			// password) at startup; the memory provider starts empty, so it
			// always applies. It is a CLI flag, not an env-configurable key.
			"--loaddata-from", sftpCredentialsMountPath + "/" + SFTPUsersKey,
		},
		Ports: []corev1.ContainerPort{
			{
				Name:          SFTPPortName,
				ContainerPort: SFTPPort,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		// SFTPGo auto-creates its host key (id_rsa) in the working directory;
		// point it at the writable config emptyDir (root FS is read-only).
		WorkingDir: sftpConfigMountPath,
		Env: []corev1.EnvVar{
			{Name: "SFTPGO_DATA_PROVIDER__DRIVER", Value: "memory"},
			{Name: "SFTPGO_SFTPD__BINDINGS__0__PORT", Value: fmt.Sprintf("%d", SFTPPort)},
			// Disable the HTTP admin/REST service entirely.
			{Name: "SFTPGO_HTTPD__BINDINGS__0__PORT", Value: "0"},
			{Name: "SFTPGO_TELEMETRY__ENABLED", Value: "false"},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("10m"),
				corev1.ResourceMemory: resource.MustParse("32Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		SecurityContext: &corev1.SecurityContext{
			RunAsNonRoot:             new(true),
			RunAsUser:                ptr.To[int64](1000),
			RunAsGroup:               ptr.To[int64](1000),
			AllowPrivilegeEscalation: new(false),
			ReadOnlyRootFilesystem:   new(true),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{Port: intstr.FromInt32(SFTPPort)},
			},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: DataVolumeName, MountPath: profile.Spec.Storage.MountPath},
			{Name: sftpCredentialsVolumeName, MountPath: sftpCredentialsMountPath, ReadOnly: true},
			{Name: sftpConfigVolumeName, MountPath: sftpConfigMountPath},
		},
	}
}

// applySFTPSidecar injects the SFTP sidecar, its volumes, and the pod-level
// fsGroup settings (so the non-root sidecar can write its config dir and
// access the world volume) into the pod spec.
func applySFTPSidecar(podSpec *corev1.PodSpec, profile *operatorv1.GameProfile, server *operatorv1.GameServer) {
	podSpec.Containers = append(podSpec.Containers, buildSFTPContainer(profile))
	podSpec.Volumes = append(podSpec.Volumes,
		corev1.Volume{
			Name: sftpCredentialsVolumeName,
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: SFTPSecretName(server.Name),
					Items: []corev1.KeyToPath{
						{Key: SFTPUsersKey, Path: SFTPUsersKey},
					},
				},
			},
		},
		corev1.Volume{
			Name:         sftpConfigVolumeName,
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	)
	if podSpec.SecurityContext == nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{}
	}
	podSpec.SecurityContext.FSGroup = ptr.To[int64](1000)
	podSpec.SecurityContext.FSGroupChangePolicy = ptr.To(corev1.FSGroupChangeOnRootMismatch)
}

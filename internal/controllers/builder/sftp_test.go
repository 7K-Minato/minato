package builder

import (
	"encoding/json"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func sftpProfile(sftp bool) *operatorv1.GameProfile {
	return &operatorv1.GameProfile{
		Spec: operatorv1.GameProfileSpec{
			DisplayName:  "Test",
			Image:        "example.com/game:latest",
			Storage:      operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "1Gi"},
			Agent:        operatorv1.AgentSpec{Image: "example.com/agent:latest", Version: "0.1.0"},
			Capabilities: &operatorv1.CapabilitiesSpec{SFTP: sftp},
			Ports:        []operatorv1.PortSpec{{Name: "game", ContainerPort: 7777}},
		},
	}
}

func findContainer(podSpec corev1.PodSpec, name string) *corev1.Container {
	for i := range podSpec.Containers {
		if podSpec.Containers[i].Name == name {
			return &podSpec.Containers[i]
		}
	}
	return nil
}

// assertSFTPVolumeMounts verifies the data PVC, credentials and config mounts.
func assertSFTPVolumeMounts(t *testing.T, volumeMounts []corev1.VolumeMount) {
	t.Helper()
	mounts := map[string]corev1.VolumeMount{}
	for _, m := range volumeMounts {
		mounts[m.Name] = m
	}
	if m, ok := mounts[DataVolumeName]; !ok || m.MountPath != "/data" {
		t.Fatalf("expected data volume mounted at /data, got %#v", volumeMounts)
	}
	if m, ok := mounts[sftpCredentialsVolumeName]; !ok || !m.ReadOnly {
		t.Fatalf("expected read-only credentials mount, got %#v", volumeMounts)
	}
	if _, ok := mounts[sftpConfigVolumeName]; !ok {
		t.Fatalf("expected config volume mount, got %#v", volumeMounts)
	}
}

// assertSFTPPSSRestricted verifies the sidecar satisfies the PSS restricted
// baseline (non-root, no privilege escalation, drop ALL, seccomp, RO rootfs).
func assertSFTPPSSRestricted(t *testing.T, sc *corev1.SecurityContext) {
	t.Helper()
	if sc == nil {
		t.Fatal("expected securityContext on sftp sidecar")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Fatal("expected runAsNonRoot=true")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser == 0 {
		t.Fatal("expected non-zero runAsUser")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Fatal("expected allowPrivilegeEscalation=false")
	}
	if sc.Capabilities == nil || len(sc.Capabilities.Drop) != 1 || sc.Capabilities.Drop[0] != "ALL" {
		t.Fatalf("expected capabilities drop ALL, got %#v", sc.Capabilities)
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Fatal("expected seccompProfile RuntimeDefault")
	}
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		t.Fatal("expected readOnlyRootFilesystem=true")
	}
}

func TestSFTPSidecarInjectedWhenCapabilityEnabled(t *testing.T) {
	server := &operatorv1.GameServer{ObjectMeta: metav1.ObjectMeta{Name: "srv", Namespace: "default"}}

	podSpec, err := BuildGameServerPodSpec(sftpProfile(true), server)
	if err != nil {
		t.Fatalf("BuildGameServerPodSpec returned error: %v", err)
	}

	sftp := findContainer(podSpec, SFTPContainerName)
	if sftp == nil {
		t.Fatalf("expected sftp sidecar container %q", SFTPContainerName)
	}
	if sftp.Image != SFTPImage {
		t.Fatalf("expected image %q, got %q", SFTPImage, sftp.Image)
	}

	// Port
	if len(sftp.Ports) != 1 || sftp.Ports[0].ContainerPort != SFTPPort || sftp.Ports[0].Protocol != corev1.ProtocolTCP {
		t.Fatalf("expected sftp port %d/TCP, got %#v", SFTPPort, sftp.Ports)
	}

	// Mounts: same data PVC at the profile mountPath, plus credentials and config volumes.
	assertSFTPVolumeMounts(t, sftp.VolumeMounts)

	// Credentials: the pre-hashed users dump is mounted from the secret and
	// preloaded via --loaddata-from.
	joined := strings.Join(sftp.Command, " ")
	if !strings.Contains(joined, "--loaddata-from "+sftpCredentialsMountPath+"/"+SFTPUsersKey) {
		t.Fatalf("expected --loaddata-from pointing at the mounted users dump, got %#v", sftp.Command)
	}
	if sftp.WorkingDir != sftpConfigMountPath {
		t.Fatalf("expected workingDir %q (writable host-key dir), got %q", sftpConfigMountPath, sftp.WorkingDir)
	}

	// PSS restricted compliance.
	assertSFTPPSSRestricted(t, sftp.SecurityContext)

	// Volumes: credentials secret (users.json only) and writable config emptyDir.
	volumes := map[string]corev1.Volume{}
	for _, v := range podSpec.Volumes {
		volumes[v.Name] = v
	}
	credVol, ok := volumes[sftpCredentialsVolumeName]
	if !ok || credVol.Secret == nil || credVol.Secret.SecretName != SFTPSecretName(server.Name) {
		t.Fatalf("expected credentials secret volume, got %#v", podSpec.Volumes)
	}
	cfgVol, ok := volumes[sftpConfigVolumeName]
	if !ok || cfgVol.EmptyDir == nil {
		t.Fatalf("expected config emptyDir volume, got %#v", podSpec.Volumes)
	}

	// fsGroup so the non-root sidecar can write its config dir and access the
	// world volume.
	if podSpec.SecurityContext == nil || podSpec.SecurityContext.FSGroup == nil || *podSpec.SecurityContext.FSGroup != 1000 {
		t.Fatalf("expected pod fsGroup 1000, got %#v", podSpec.SecurityContext)
	}
	if podSpec.SecurityContext.FSGroupChangePolicy == nil ||
		*podSpec.SecurityContext.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch {
		t.Fatalf("expected fsGroupChangePolicy OnRootMismatch, got %#v", podSpec.SecurityContext)
	}
}

func TestSFTPSidecarAbsentWithoutCapability(t *testing.T) {
	server := &operatorv1.GameServer{ObjectMeta: metav1.ObjectMeta{Name: "srv", Namespace: "default"}}

	for name, profile := range map[string]*operatorv1.GameProfile{
		"capability off":  sftpProfile(false),
		"no capabilities": func() *operatorv1.GameProfile { p := sftpProfile(false); p.Spec.Capabilities = nil; return p }(),
	} {
		t.Run(name, func(t *testing.T) {
			podSpec, err := BuildGameServerPodSpec(profile, server)
			if err != nil {
				t.Fatalf("BuildGameServerPodSpec returned error: %v", err)
			}
			if findContainer(podSpec, SFTPContainerName) != nil {
				t.Fatal("expected no sftp sidecar")
			}
			if len(podSpec.Containers) != 2 {
				t.Fatalf("expected 2 containers, got %d", len(podSpec.Containers))
			}
			if podSpec.SecurityContext != nil {
				t.Fatalf("expected no pod securityContext, got %#v", podSpec.SecurityContext)
			}
		})
	}
}

func TestSFTPUsersJSON(t *testing.T) {
	rendered := SFTPUsersJSON("/data", "$2a$10$somebcrypthash")
	var dump struct {
		Version int `json:"version"`
		Users   []struct {
			Username string `json:"username"`
			Password string `json:"password"`
			HomeDir  string `json:"home_dir"`
			Status   int    `json:"status"`
		} `json:"users"`
	}
	if err := json.Unmarshal([]byte(rendered), &dump); err != nil {
		t.Fatalf("users JSON is not valid: %v", err)
	}
	if len(dump.Users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(dump.Users))
	}
	u := dump.Users[0]
	if u.Username != SFTPUsername || u.Password != "$2a$10$somebcrypthash" || u.HomeDir != "/data" || u.Status != 1 {
		t.Fatalf("unexpected user: %+v", u)
	}
}

func TestSFTPSecretName(t *testing.T) {
	if got := SFTPSecretName("mc-1"); got != "minato-mc-1-sftp" {
		t.Fatalf("unexpected secret name %q", got)
	}
}

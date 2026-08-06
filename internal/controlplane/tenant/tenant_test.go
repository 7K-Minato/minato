package tenant

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))
	return scheme
}

func TestLoadConfig(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    Config
		wantErr string
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			want: Config{
				MaxServers:            10,
				CPU:                   "8",
				Memory:                "32Gi",
				Storage:               "200Gi",
				GamePorts:             []int32{25565},
				ControlPlaneNamespace: "minato",
			},
		},
		{
			name: "custom values",
			env: map[string]string{
				"TENANT_QUOTA_MAX_SERVERS":      "25",
				"TENANT_QUOTA_CPU":              "16",
				"TENANT_QUOTA_MEMORY":           "64Gi",
				"TENANT_QUOTA_STORAGE":          "1Ti",
				"TENANT_GAME_PORTS":             "25565, 27015",
				"TENANT_CONTROLPLANE_NAMESPACE": "minato-system",
			},
			want: Config{
				MaxServers:            25,
				CPU:                   "16",
				Memory:                "64Gi",
				Storage:               "1Ti",
				GamePorts:             []int32{25565, 27015},
				ControlPlaneNamespace: "minato-system",
			},
		},
		{
			name: "zero/empty disables constraints",
			env: map[string]string{
				"TENANT_QUOTA_MAX_SERVERS": "0",
				"TENANT_QUOTA_CPU":         "0",
				"TENANT_QUOTA_MEMORY":      "",
				"TENANT_QUOTA_STORAGE":     "",
			},
			want: Config{
				MaxServers:            0,
				CPU:                   "",
				Memory:                "",
				Storage:               "",
				GamePorts:             []int32{25565},
				ControlPlaneNamespace: "minato",
			},
		},
		{
			name:    "invalid max servers",
			env:     map[string]string{"TENANT_QUOTA_MAX_SERVERS": "ten"},
			wantErr: "TENANT_QUOTA_MAX_SERVERS",
		},
		{
			name:    "negative max servers",
			env:     map[string]string{"TENANT_QUOTA_MAX_SERVERS": "-1"},
			wantErr: "TENANT_QUOTA_MAX_SERVERS",
		},
		{
			name:    "invalid cpu quantity",
			env:     map[string]string{"TENANT_QUOTA_CPU": "eight"},
			wantErr: "TENANT_QUOTA_CPU",
		},
		{
			name:    "invalid memory quantity",
			env:     map[string]string{"TENANT_QUOTA_MEMORY": "lots"},
			wantErr: "TENANT_QUOTA_MEMORY",
		},
		{
			name:    "invalid game port",
			env:     map[string]string{"TENANT_GAME_PORTS": "abc"},
			wantErr: "TENANT_GAME_PORTS",
		},
		{
			name:    "game port out of range",
			env:     map[string]string{"TENANT_GAME_PORTS": "70000"},
			wantErr: "TENANT_GAME_PORTS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg, err := LoadConfig()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, cfg)
		})
	}
}

func TestBuildNamespace(t *testing.T) {
	ns := BuildNamespace("tenant-alpha")
	assert.Equal(t, "tenant-alpha", ns.Name)
	assert.Equal(t, "tenant-alpha", ns.Labels[LabelTenant])
	assert.Equal(t, ManagedByValue, ns.Labels[LabelManagedBy])
	assert.Equal(t, "restricted", ns.Labels["pod-security.kubernetes.io/enforce"])
	assert.Equal(t, "restricted", ns.Labels["pod-security.kubernetes.io/audit"])
	assert.Equal(t, "restricted", ns.Labels["pod-security.kubernetes.io/warn"])
}

func TestBuildResourceQuota(t *testing.T) {
	t.Run("full config", func(t *testing.T) {
		q := BuildResourceQuota("tenant-alpha", Config{
			MaxServers: 5,
			CPU:        "4",
			Memory:     "16Gi",
			Storage:    "100Gi",
		})
		require.NotNil(t, q)
		assert.Equal(t, QuotaName, q.Name)
		assert.Equal(t, "tenant-alpha", q.Namespace)
		maxServers := q.Spec.Hard["count/gameservers.operator.minato.io"]
		cpu := q.Spec.Hard[corev1.ResourceRequestsCPU]
		memory := q.Spec.Hard[corev1.ResourceRequestsMemory]
		storage := q.Spec.Hard[corev1.ResourceRequestsStorage]
		assert.Equal(t, "5", maxServers.String())
		assert.Equal(t, "4", cpu.String())
		assert.Equal(t, "16Gi", memory.String())
		assert.Equal(t, "100Gi", storage.String())
	})

	t.Run("partial config skips constraints", func(t *testing.T) {
		q := BuildResourceQuota("tenant-alpha", Config{MaxServers: 3})
		require.NotNil(t, q)
		assert.Len(t, q.Spec.Hard, 1)
		_, hasCPU := q.Spec.Hard[corev1.ResourceRequestsCPU]
		assert.False(t, hasCPU)
	})

	t.Run("all disabled returns nil", func(t *testing.T) {
		assert.Nil(t, BuildResourceQuota("tenant-alpha", Config{}))
	})
}

func TestBuildNetworkPolicy(t *testing.T) {
	np := BuildNetworkPolicy("tenant-alpha", Config{
		GamePorts:             []int32{25565, 27015},
		ControlPlaneNamespace: "minato",
	})
	assert.Equal(t, NetworkPolicyName, np.Name)
	assert.Equal(t, "tenant-alpha", np.Namespace)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	// Ingress: same-namespace rule, control-plane agent rule, game-port rule.
	require.Len(t, np.Spec.Ingress, 3)

	sameNS := np.Spec.Ingress[0]
	require.Len(t, sameNS.From, 1)
	assert.Equal(t, "tenant-alpha", sameNS.From[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"])

	agent := np.Spec.Ingress[1]
	require.Len(t, agent.From, 1)
	assert.Equal(t, "minato", agent.From[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"])
	require.Len(t, agent.Ports, 1)
	assert.Equal(t, int32(AgentPort), agent.Ports[0].Port.IntVal)

	game := np.Spec.Ingress[2]
	assert.Empty(t, game.From, "game ports must be reachable from any source (internet)")
	assert.Len(t, game.Ports, 4, "each game port yields TCP+UDP")

	// Egress: DNS, agent, game ports.
	require.Len(t, np.Spec.Egress, 3)
	assert.Len(t, np.Spec.Egress[0].Ports, 2, "DNS over UDP+TCP")
	assert.Equal(t, int32(53), np.Spec.Egress[0].Ports[0].Port.IntVal)
}

func TestBuildNetworkPolicy_NoGamePorts(t *testing.T) {
	np := BuildNetworkPolicy("tenant-alpha", Config{ControlPlaneNamespace: "minato"})
	assert.Len(t, np.Spec.Ingress, 2, "no game-port ingress rule without game ports")
	assert.Len(t, np.Spec.Egress, 2, "no game-port egress rule without game ports")
}

func TestEnsureNamespace_CreatesHardenedNamespace(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	cfg := Config{
		MaxServers:            2,
		CPU:                   "1",
		Memory:                "1Gi",
		Storage:               "10Gi",
		GamePorts:             []int32{25565},
		ControlPlaneNamespace: "minato",
	}

	require.NoError(t, EnsureNamespace(context.Background(), c, "tenant-new", cfg))

	var ns corev1.Namespace
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "tenant-new"}, &ns))
	assert.Equal(t, "tenant-new", ns.Labels[LabelTenant])
	assert.Equal(t, "restricted", ns.Labels["pod-security.kubernetes.io/enforce"])

	var quota corev1.ResourceQuota
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: QuotaName, Namespace: "tenant-new"}, &quota))
	maxServers := quota.Spec.Hard["count/gameservers.operator.minato.io"]
	assert.Equal(t, "2", maxServers.String())

	var np networkingv1.NetworkPolicy
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: NetworkPolicyName, Namespace: "tenant-new"}, &np))
}

func TestEnsureNamespace_ExistingNamespaceNotReconciled(t *testing.T) {
	existing := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "tenant-existing",
			Labels: map[string]string{"team": "custom"},
		},
	}
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).WithObjects(existing).Build()

	require.NoError(t, EnsureNamespace(context.Background(), c, "tenant-existing", Config{}))

	// Labels untouched, no quota/netpol created.
	var ns corev1.Namespace
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{Name: "tenant-existing"}, &ns))
	assert.Equal(t, "custom", ns.Labels["team"])
	_, ok := ns.Labels[LabelTenant]
	assert.False(t, ok, "existing namespace must not be relabeled")

	var quotas corev1.ResourceQuotaList
	require.NoError(t, c.List(context.Background(), &quotas))
	assert.Empty(t, quotas.Items)

	var nps networkingv1.NetworkPolicyList
	require.NoError(t, c.List(context.Background(), &nps))
	assert.Empty(t, nps.Items)
}

func TestEnsureNamespace_Idempotent(t *testing.T) {
	c := fake.NewClientBuilder().WithScheme(testScheme(t)).Build()
	require.NoError(t, EnsureNamespace(context.Background(), c, "tenant-x", Config{}))
	require.NoError(t, EnsureNamespace(context.Background(), c, "tenant-x", Config{}), "second call must be a no-op")
}

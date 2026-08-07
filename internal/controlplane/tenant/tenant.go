// Package tenant implements creation-time hardening for tenant namespaces
// auto-provisioned by the control plane: labels, Pod Security Standards,
// a ResourceQuota, and a default-deny NetworkPolicy.
//
// Hardening is applied at namespace creation time ONLY. If the namespace
// already exists, nothing is reconciled or overwritten — operators manage
// existing namespaces via GitOps.
package tenant

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// LabelTenant marks a namespace as belonging to a minato tenant.
	LabelTenant = "minato.io/tenant"
	// LabelManagedBy marks namespaces hardened by the control plane.
	LabelManagedBy = "app.kubernetes.io/managed-by"
	// ManagedByValue is the value used for LabelManagedBy.
	ManagedByValue = "minato-controlplane"

	// QuotaName is the ResourceQuota created in each new tenant namespace.
	QuotaName = "minato-tenant-quota"
	// NetworkPolicyName is the default NetworkPolicy created in each new tenant namespace.
	NetworkPolicyName = "minato-tenant-default"

	// AgentPort is the agent gRPC port the control plane dials into.
	AgentPort = 9876

	defaultMaxServers            = 10
	defaultCPU                   = "8"
	defaultMemory                = "32Gi"
	defaultStorage               = "200Gi"
	defaultGamePorts             = "25565"
	defaultControlPlaneNamespace = "minato"
	gameServerCountQuotaResource = "count/gameservers.operator.minato.io"
)

// Config controls how a new tenant namespace is hardened. Zero-valued quota
// fields skip the corresponding constraint.
type Config struct {
	// MaxServers caps the number of GameServers (count/gameservers.operator.minato.io). 0 = skip.
	MaxServers int
	// CPU caps requests.cpu (e.g. "8"). Empty = skip.
	CPU string
	// Memory caps requests.memory (e.g. "32Gi"). Empty = skip.
	Memory string
	// Storage caps requests.storage (e.g. "200Gi"). Empty = skip.
	Storage string
	// GamePorts are the ports exposed to the internet for game traffic (TCP+UDP).
	GamePorts []int32
	// ControlPlaneNamespace is where the control plane runs; it gets ingress to the agent port.
	ControlPlaneNamespace string
}

// LoadConfig parses tenant hardening configuration from the environment.
// Unset variables fall back to sane defaults; explicitly setting a quota
// variable to "" or "0" disables that constraint. Invalid values are startup
// configuration errors.
//
//	TENANT_QUOTA_MAX_SERVERS       (default "10")
//	TENANT_QUOTA_CPU               (default "8")
//	TENANT_QUOTA_MEMORY            (default "32Gi")
//	TENANT_QUOTA_STORAGE           (default "200Gi")
//	TENANT_GAME_PORTS              (default "25565", comma-separated)
//	TENANT_CONTROLPLANE_NAMESPACE  (default "minato")
func LoadConfig() (Config, error) {
	cfg := Config{
		ControlPlaneNamespace: os.Getenv("TENANT_CONTROLPLANE_NAMESPACE"),
	}
	if cfg.ControlPlaneNamespace == "" {
		cfg.ControlPlaneNamespace = defaultControlPlaneNamespace
	}

	maxServers, err := envInt("TENANT_QUOTA_MAX_SERVERS", defaultMaxServers)
	if err != nil {
		return Config{}, err
	}
	if maxServers < 0 {
		return Config{}, fmt.Errorf("TENANT_QUOTA_MAX_SERVERS must be >= 0, got %d", maxServers)
	}
	cfg.MaxServers = maxServers

	if cfg.CPU, err = envQuantity("TENANT_QUOTA_CPU", defaultCPU); err != nil {
		return Config{}, err
	}
	if cfg.Memory, err = envQuantity("TENANT_QUOTA_MEMORY", defaultMemory); err != nil {
		return Config{}, err
	}
	if cfg.Storage, err = envQuantity("TENANT_QUOTA_STORAGE", defaultStorage); err != nil {
		return Config{}, err
	}

	ports, err := envPorts("TENANT_GAME_PORTS", defaultGamePorts)
	if err != nil {
		return Config{}, err
	}
	cfg.GamePorts = ports

	return cfg, nil
}

// withDefaults fills zero fields with the same defaults LoadConfig uses, so a
// zero Config is still safe to provision with (used by tests constructing the
// API directly).
func (c Config) withDefaults() Config {
	if c.CPU == "" && c.Memory == "" && c.Storage == "" && c.MaxServers == 0 && len(c.GamePorts) == 0 && c.ControlPlaneNamespace == "" {
		return Config{
			MaxServers:            defaultMaxServers,
			CPU:                   defaultCPU,
			Memory:                defaultMemory,
			Storage:               defaultStorage,
			GamePorts:             []int32{25565},
			ControlPlaneNamespace: defaultControlPlaneNamespace,
		}
	}
	if c.ControlPlaneNamespace == "" {
		c.ControlPlaneNamespace = defaultControlPlaneNamespace
	}
	return c
}

// EnsureNamespace creates the tenant namespace with hardening applied if it
// does not exist. If the namespace already exists it is left untouched —
// existing namespaces are managed by operators via GitOps.
func EnsureNamespace(ctx context.Context, c client.Client, namespace string, cfg Config) error {
	cfg = cfg.withDefaults()

	ns := &corev1.Namespace{}
	err := c.Get(ctx, types.NamespacedName{Name: namespace}, ns)
	switch {
	case err == nil:
		// Already exists: creation-time-only semantics, do not reconcile.
		return nil
	case !apierrors.IsNotFound(err):
		return err
	}

	if err := c.Create(ctx, BuildNamespace(namespace)); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create tenant namespace: %w", err)
	}

	if quota := BuildResourceQuota(namespace, cfg); quota != nil {
		if err := c.Create(ctx, quota); err != nil && !apierrors.IsAlreadyExists(err) {
			return fmt.Errorf("create tenant resource quota: %w", err)
		}
	}

	if err := c.Create(ctx, BuildNetworkPolicy(namespace, cfg)); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create tenant network policy: %w", err)
	}

	return nil
}

// BuildNamespace returns the tenant namespace with tenant/managed-by labels and
// restricted Pod Security Standards labels (mirroring the chart's namespace template).
func BuildNamespace(name string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
			Labels: map[string]string{
				LabelTenant:                          name,
				LabelManagedBy:                       ManagedByValue,
				"pod-security.kubernetes.io/enforce": "restricted",
				"pod-security.kubernetes.io/audit":   "restricted",
				"pod-security.kubernetes.io/warn":    "restricted",
			},
		},
	}
}

// BuildResourceQuota returns the per-tenant ResourceQuota, or nil if every
// constraint is disabled.
func BuildResourceQuota(namespace string, cfg Config) *corev1.ResourceQuota {
	hard := corev1.ResourceList{}
	if cfg.MaxServers > 0 {
		hard[corev1.ResourceName(gameServerCountQuotaResource)] = *resource.NewQuantity(int64(cfg.MaxServers), resource.DecimalSI)
	}
	if cfg.CPU != "" {
		hard[corev1.ResourceRequestsCPU] = resource.MustParse(cfg.CPU)
	}
	if cfg.Memory != "" {
		hard[corev1.ResourceRequestsMemory] = resource.MustParse(cfg.Memory)
	}
	if cfg.Storage != "" {
		hard[corev1.ResourceRequestsStorage] = resource.MustParse(cfg.Storage)
	}
	if len(hard) == 0 {
		return nil
	}
	return &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:      QuotaName,
			Namespace: namespace,
			Labels: map[string]string{
				LabelTenant:    namespace,
				LabelManagedBy: ManagedByValue,
			},
		},
		Spec: corev1.ResourceQuotaSpec{Hard: hard},
	}
}

// BuildNetworkPolicy returns the default-deny NetworkPolicy for a new tenant
// namespace.
//
// Posture (documented in docs/operations/multi-tenancy.md):
//   - Ingress: allowed from pods in the same namespace; the control plane
//     namespace may reach the agent gRPC port (9876); game ports are open to
//     ANY source because game traffic arrives directly from the internet via
//     NodePort/LoadBalancer Services. Cross-tenant pod-to-pod traffic is denied.
//   - Egress: DNS (53 TCP/UDP), the agent port, and the game ports. Anything
//     broader must be layered in by the operator via a GitOps-managed policy.
func BuildNetworkPolicy(namespace string, cfg Config) *networkingv1.NetworkPolicy {
	tcp := corev1.ProtocolTCP
	udp := corev1.ProtocolUDP

	gamePortRules := make([]networkingv1.NetworkPolicyPort, 0, 2*len(cfg.GamePorts))
	for _, p := range cfg.GamePorts {
		port := intstr.FromInt32(p)
		gamePortRules = append(gamePortRules,
			networkingv1.NetworkPolicyPort{Protocol: &tcp, Port: &port},
			networkingv1.NetworkPolicyPort{Protocol: &udp, Port: &port},
		)
	}

	agentPort := intstr.FromInt32(AgentPort)
	dnsPort := intstr.FromInt32(53)

	ingress := []networkingv1.NetworkPolicyIngressRule{
		// Same-namespace pods.
		{
			From: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": namespace},
				},
			}},
		},
		// Control plane -> agent gRPC.
		{
			From: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": cfg.ControlPlaneNamespace},
				},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &agentPort}},
		},
	}
	// Game traffic from the internet: no `from` means all sources.
	if len(gamePortRules) > 0 {
		ingress = append(ingress, networkingv1.NetworkPolicyIngressRule{Ports: gamePortRules})
	}

	egress := []networkingv1.NetworkPolicyEgressRule{
		{Ports: []networkingv1.NetworkPolicyPort{
			{Protocol: &udp, Port: &dnsPort},
			{Protocol: &tcp, Port: &dnsPort},
		}},
		{Ports: []networkingv1.NetworkPolicyPort{{Protocol: &tcp, Port: &agentPort}}},
	}
	if len(gamePortRules) > 0 {
		egress = append(egress, networkingv1.NetworkPolicyEgressRule{Ports: gamePortRules})
	}

	return &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkPolicyName,
			Namespace: namespace,
			Labels: map[string]string{
				LabelTenant:    namespace,
				LabelManagedBy: ManagedByValue,
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: ingress,
			Egress:  egress,
		},
	}
}

// envInt reads an integer env var; unset = fallback, ""/"0" = 0 (disabled).
func envInt(key string, fallback int) (int, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return fallback, nil
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid integer %q: %w", key, v, err)
	}
	return n, nil
}

// envQuantity reads a resource.Quantity env var; unset = fallback, "" = skipped constraint.
func envQuantity(key, fallback string) (string, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		v = fallback
	}
	v = strings.TrimSpace(v)
	if v == "" || v == "0" {
		return "", nil
	}
	if _, err := resource.ParseQuantity(v); err != nil {
		return "", fmt.Errorf("%s: invalid quantity %q: %w", key, v, err)
	}
	return v, nil
}

// envPorts reads a comma-separated port list; unset = fallback list, "" = none.
func envPorts(key, fallback string) ([]int32, error) {
	v, ok := os.LookupEnv(key)
	if !ok {
		v = fallback
	}
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, nil
	}
	var ports []int32
	for part := range strings.SplitSeq(v, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("%s: invalid port %q: %w", key, part, err)
		}
		if n < 1 || n > 65535 {
			return nil, fmt.Errorf("%s: port %d out of range (1-65535)", key, n)
		}
		ports = append(ports, int32(n))
	}
	return ports, nil
}

package controllers

import (
	"context"
	"testing"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func serviceMonitorTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := newTestScheme()
	require.NoError(t, apiextensionsv1.AddToScheme(scheme))
	return scheme
}

func prometheusCRD() *apiextensionsv1.CustomResourceDefinition {
	return &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "servicemonitors.monitoring.coreos.com"},
	}
}

func profileWithObservability() *operatorv1.GameProfile {
	profile := newTestProfile()
	profile.Spec.Observability = &operatorv1.ObservabilitySpec{
		AgentMetrics: &operatorv1.AgentMetricsSpec{Port: 9090, Path: "/agent-metrics"},
		ServiceMonitor: &operatorv1.ServiceMonitorSpec{
			Enabled:  true,
			Interval: "15s",
		},
	}
	return profile
}

func TestReconcileServiceMonitor_CreatedWhenEnabledAndDetected(t *testing.T) {
	scheme := serviceMonitorTestScheme(t)
	ctx := context.Background()

	ResetPrometheusDetection()
	t.Cleanup(ResetPrometheusDetection)

	profile := profileWithObservability()
	server := newTestGameServer()
	server.Finalizers = []string{gameServerFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(prometheusCRD(), profile, server).
		WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: server.Name, Namespace: server.Namespace}}
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	sm := &monitoringv1.ServiceMonitor{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, sm))

	require.Len(t, sm.Spec.Endpoints, 1)
	endpoint := sm.Spec.Endpoints[0]
	assert.Equal(t, agentMetricsPortName, endpoint.Port)
	assert.Equal(t, "/agent-metrics", endpoint.Path)
	assert.Equal(t, monitoringv1.Duration("15s"), endpoint.Interval)
	assert.Equal(t, buildGameServerLabels(server, profile), sm.Spec.Selector.MatchLabels)

	// Owned by the GameServer for garbage collection.
	require.Len(t, sm.OwnerReferences, 1)
	assert.Equal(t, server.Name, sm.OwnerReferences[0].Name)
	assert.Equal(t, "GameServer", sm.OwnerReferences[0].Kind)

	// The agent Service must expose the named metrics port the endpoint scrapes.
	agentSvc := &corev1.Service{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Name: server.Name + "-agent", Namespace: server.Namespace}, agentSvc))
	var metricsPort *corev1.ServicePort
	for i := range agentSvc.Spec.Ports {
		if agentSvc.Spec.Ports[i].Name == agentMetricsPortName {
			metricsPort = &agentSvc.Spec.Ports[i]
		}
	}
	require.NotNil(t, metricsPort, "agent service must expose the metrics port")
	assert.Equal(t, int32(9090), metricsPort.Port)
}

func TestReconcileServiceMonitor_SkippedWhenOperatorNotDetected(t *testing.T) {
	scheme := serviceMonitorTestScheme(t)
	ctx := context.Background()

	ResetPrometheusDetection()
	t.Cleanup(ResetPrometheusDetection)

	profile := profileWithObservability()
	server := newTestGameServer()
	server.Finalizers = []string{gameServerFinalizer}

	recorder := record.NewFakeRecorder(10)
	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(profile, server).
		WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme, Recorder: recorder}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: server.Name, Namespace: server.Namespace}}
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err, "missing Prometheus Operator must not fail the reconcile")

	sm := &monitoringv1.ServiceMonitor{}
	err = cl.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, sm)
	assert.Error(t, err, "ServiceMonitor must not be created")

	select {
	case event := <-recorder.Events:
		assert.Contains(t, event, "PrometheusOperatorNotDetected")
	default:
		t.Fatal("expected a PrometheusOperatorNotDetected warning event")
	}
}

func TestReconcileServiceMonitor_NotCreatedWhenDisabled(t *testing.T) {
	scheme := serviceMonitorTestScheme(t)
	ctx := context.Background()

	ResetPrometheusDetection()
	t.Cleanup(ResetPrometheusDetection)

	profile := newTestProfile() // no observability section
	server := newTestGameServer()
	server.Finalizers = []string{gameServerFinalizer}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(prometheusCRD(), profile, server).
		WithStatusSubresource(&operatorv1.GameServer{}).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: server.Name, Namespace: server.Namespace}}
	_, err := reconciler.Reconcile(ctx, req)
	require.NoError(t, err)

	sm := &monitoringv1.ServiceMonitor{}
	err = cl.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, sm)
	assert.Error(t, err, "ServiceMonitor must not be created when the profile does not enable it")

	// The agent service must not gain a metrics port either.
	agentSvc := &corev1.Service{}
	require.NoError(t, cl.Get(ctx, types.NamespacedName{Name: server.Name + "-agent", Namespace: server.Namespace}, agentSvc))
	for _, p := range agentSvc.Spec.Ports {
		assert.NotEqual(t, agentMetricsPortName, p.Name)
	}
}

func TestReconcileServiceMonitor_RemovedWhenProfileDisablesIt(t *testing.T) {
	scheme := serviceMonitorTestScheme(t)
	ctx := context.Background()

	ResetPrometheusDetection()
	t.Cleanup(ResetPrometheusDetection)

	profile := newTestProfile()
	server := newTestGameServer()
	existing := &monitoringv1.ServiceMonitor{
		ObjectMeta: metav1.ObjectMeta{Name: server.Name, Namespace: server.Namespace},
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(profile, server, existing).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	err := reconciler.reconcileServiceMonitor(ctx, server, profile, buildGameServerLabels(server, profile))
	require.NoError(t, err)

	sm := &monitoringv1.ServiceMonitor{}
	err = cl.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, sm)
	assert.Error(t, err, "stale ServiceMonitor must be removed when the profile disables it")
}

func TestReconcileServiceMonitor_DefaultPathAndInterval(t *testing.T) {
	profile := newTestProfile()
	profile.Spec.Observability = &operatorv1.ObservabilitySpec{
		AgentMetrics:   &operatorv1.AgentMetricsSpec{Port: 9090},
		ServiceMonitor: &operatorv1.ServiceMonitorSpec{Enabled: true},
	}

	spec := buildServiceMonitorSpec(profile, map[string]string{"k": "v"})
	require.Len(t, spec.Endpoints, 1)
	assert.Equal(t, "/metrics", spec.Endpoints[0].Path)
	assert.Equal(t, monitoringv1.Duration(""), spec.Endpoints[0].Interval)
}

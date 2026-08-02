package controllers

import (
	"context"

	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

// agentMetricsPortName is the Service port name the generated ServiceMonitor
// scrapes. It is added to the agent Service (<server>-agent) when the
// GameProfile declares observability.agentMetrics.
const agentMetricsPortName = "metrics"

// serviceMonitorEnabled reports whether the profile requests a ServiceMonitor
// for its game servers.
func serviceMonitorEnabled(profile *operatorv1.GameProfile) bool {
	return profile.Spec.Observability != nil &&
		profile.Spec.Observability.ServiceMonitor != nil &&
		profile.Spec.Observability.ServiceMonitor.Enabled
}

// agentMetricsSpec returns the declared agent metrics endpoint, or nil.
func agentMetricsSpec(profile *operatorv1.GameProfile) *operatorv1.AgentMetricsSpec {
	if profile.Spec.Observability == nil {
		return nil
	}
	return profile.Spec.Observability.AgentMetrics
}

// addAgentMetricsPort appends the agent metrics port to the agent Service so
// the ServiceMonitor has a named port to scrape. The port is only added when
// the profile declares observability.agentMetrics.port.
func addAgentMetricsPort(svc *corev1.Service, profile *operatorv1.GameProfile) {
	spec := agentMetricsSpec(profile)
	if spec == nil || spec.Port <= 0 {
		return
	}
	for _, p := range svc.Spec.Ports {
		if p.Name == agentMetricsPortName {
			return
		}
	}
	svc.Spec.Ports = append(svc.Spec.Ports, corev1.ServicePort{
		Name:       agentMetricsPortName,
		Port:       spec.Port,
		TargetPort: intstr.FromInt32(spec.Port),
		Protocol:   corev1.ProtocolTCP,
	})
}

// reconcileServiceMonitor creates or updates a ServiceMonitor scraping the
// agent metrics endpoint declared by the profile. The ServiceMonitor is owned
// by the GameServer and garbage-collected with it.
//
// When the profile does not enable ServiceMonitors, any previously created
// ServiceMonitor is removed. When the Prometheus Operator CRDs are not
// installed, the step is skipped with a log line and a Warning event instead
// of failing the reconcile.
func (r *GameServerReconciler) reconcileServiceMonitor(
	ctx context.Context,
	server *operatorv1.GameServer,
	profile *operatorv1.GameProfile,
	labelsMap map[string]string,
) error {
	logger := log.FromContext(ctx)
	key := client.ObjectKey{Name: server.Name, Namespace: server.Namespace}

	if !serviceMonitorEnabled(profile) {
		sm := &monitoringv1.ServiceMonitor{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
		if err := r.Delete(ctx, sm); err != nil && !apierrors.IsNotFound(err) {
			return err
		}
		return nil
	}

	if !DetectPrometheusOperator(ctx, r.Client) {
		logger.Info("skipping ServiceMonitor creation: Prometheus Operator not detected",
			"gameserver", server.Name, "namespace", server.Namespace)
		if r.Recorder != nil {
			r.Recorder.Eventf(server, corev1.EventTypeWarning, "PrometheusOperatorNotDetected",
				"GameProfile %s enables serviceMonitor but the Prometheus Operator is not installed; skipping ServiceMonitor creation", profile.Name)
		}
		return nil
	}

	spec := agentMetricsSpec(profile)
	if spec == nil || spec.Port <= 0 {
		logger.Info("skipping ServiceMonitor creation: profile declares no agentMetrics port",
			"gameserver", server.Name, "profile", profile.Name)
		return nil
	}

	sm := &monitoringv1.ServiceMonitor{ObjectMeta: metav1.ObjectMeta{Name: key.Name, Namespace: key.Namespace}}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, sm, func() error {
		sm.Labels = labelsMap
		sm.Spec = buildServiceMonitorSpec(profile, labelsMap)
		return controllerutil.SetControllerReference(server, sm, r.Scheme)
	})
	return err
}

func buildServiceMonitorSpec(profile *operatorv1.GameProfile, labelsMap map[string]string) monitoringv1.ServiceMonitorSpec {
	spec := agentMetricsSpec(profile)
	path := spec.Path
	if path == "" {
		path = "/metrics"
	}

	endpoint := monitoringv1.Endpoint{
		Port: agentMetricsPortName,
		Path: path,
	}
	if interval := profile.Spec.Observability.ServiceMonitor.Interval; interval != "" {
		endpoint.Interval = monitoringv1.Duration(interval)
	}

	return monitoringv1.ServiceMonitorSpec{
		Selector: metav1.LabelSelector{MatchLabels: labelsMap},
		Endpoints: []monitoringv1.Endpoint{
			endpoint,
		},
	}
}

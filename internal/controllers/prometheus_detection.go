package controllers

import (
	"context"
	"sync"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var (
	prometheusDetected     bool
	prometheusDetectedOnce sync.Once
)

// IsPrometheusOperatorDetected returns whether the Prometheus Operator is installed.
func IsPrometheusOperatorDetected() bool {
	return prometheusDetected
}

// DetectPrometheusOperator checks if the Prometheus Operator CRDs are present.
func DetectPrometheusOperator(ctx context.Context, c client.Reader) bool {
	prometheusDetectedOnce.Do(func() {
		logger := log.FromContext(ctx)

		crd := &apiextensionsv1.CustomResourceDefinition{}
		err := c.Get(ctx, types.NamespacedName{Name: "servicemonitors.monitoring.coreos.com"}, crd)
		if err != nil {
			if apierrors.IsNotFound(err) {
				prometheusDetected = false
				logger.Info("Prometheus Operator not detected (ServiceMonitor CRD not found)")
				return
			}
			logger.Error(err, "failed to detect Prometheus Operator")
			prometheusDetected = false
			return
		}

		prometheusDetected = true
		logger.Info("Prometheus Operator detected")
	})

	return prometheusDetected
}

// ResetPrometheusDetection resets the detection state (for testing).
func ResetPrometheusDetection() {
	prometheusDetectedOnce = sync.Once{}
	prometheusDetected = false
}

// MustRegisterPrometheusCRDs adds the Prometheus Operator types to the scheme.
func MustRegisterPrometheusCRDs() error {
	// This is a placeholder - in a real implementation, you would import
	// the Prometheus Operator types and add them to the scheme
	// For now, we just return nil since we're using unstructured objects
	return nil
}

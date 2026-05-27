package controllers

import (
	"context"
	"sync"
	"time"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const prometheusDetectionCacheDuration = 5 * time.Minute

var (
	prometheusDetected    bool
	prometheusDetectedMu  sync.RWMutex
	prometheusLastChecked time.Time
)

// IsPrometheusOperatorDetected returns whether the Prometheus Operator is installed.
func IsPrometheusOperatorDetected() bool {
	prometheusDetectedMu.RLock()
	defer prometheusDetectedMu.RUnlock()
	return prometheusDetected
}

// DetectPrometheusOperator checks if the Prometheus Operator CRDs are present.
// It caches the result and only re-checks every 5 minutes so that installing
// the operator after the controller starts is eventually detected.
func DetectPrometheusOperator(ctx context.Context, c client.Reader) bool {
	prometheusDetectedMu.Lock()
	defer prometheusDetectedMu.Unlock()

	if time.Since(prometheusLastChecked) < prometheusDetectionCacheDuration {
		return prometheusDetected
	}

	logger := log.FromContext(ctx)
	prometheusLastChecked = time.Now()

	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := c.Get(ctx, types.NamespacedName{Name: "servicemonitors.monitoring.coreos.com"}, crd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			prometheusDetected = false
			logger.Info("Prometheus Operator not detected (ServiceMonitor CRD not found)")
			return false
		}
		logger.Error(err, "failed to detect Prometheus Operator")
		prometheusDetected = false
		return false
	}

	prometheusDetected = true
	logger.Info("Prometheus Operator detected")
	return prometheusDetected
}

// ResetPrometheusDetection resets the detection state (for testing).
func ResetPrometheusDetection() {
	prometheusDetectedMu.Lock()
	defer prometheusDetectedMu.Unlock()
	prometheusDetected = false
	prometheusLastChecked = time.Time{}
}

// MustRegisterPrometheusCRDs adds the Prometheus Operator types to the scheme.
func MustRegisterPrometheusCRDs() error {
	// This is a placeholder - in a real implementation, you would import
	// the Prometheus Operator types and add them to the scheme
	// For now, we just return nil since we're using unstructured objects
	return nil
}

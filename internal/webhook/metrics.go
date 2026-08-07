package webhook

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	crmetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// Admission webhook metrics implementing docs/operations/metrics-schema.md.
// Registered with controller-runtime's registry, so they are exposed on the
// operator manager's metrics endpoint alongside the operator metrics.
var (
	webhookRequestsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "minato_webhook_requests_total",
		Help: "Total admission webhook validation requests.",
	}, []string{"webhook", "operation", "result"})

	webhookRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "minato_webhook_request_duration_seconds",
		Help:    "Duration of admission webhook validation requests.",
		Buckets: prometheus.DefBuckets,
	}, []string{"webhook", "operation"})
)

func init() {
	crmetrics.Registry.MustRegister(webhookRequestsTotal, webhookRequestDuration)
}

// observeWebhook records one webhook validation request. result is derived
// from err: "allowed" when the object passed, "denied" when validation
// rejected it. Both allowed and denied requests are errors-free/admitted at
// the admission layer distinction; here any validation error is a denial.
func observeWebhook(webhook, operation string, start time.Time, err error) {
	result := "allowed"
	if err != nil {
		result = "denied"
	}
	webhookRequestsTotal.WithLabelValues(webhook, operation, result).Inc()
	webhookRequestDuration.WithLabelValues(webhook, operation).Observe(time.Since(start).Seconds())
}

// validate wraps fn with metric recording for the given webhook/operation.
func observeValidation(webhook, operation string, fn func() (admission.Warnings, error)) (admission.Warnings, error) {
	start := time.Now()
	warnings, err := fn()
	observeWebhook(webhook, operation, start, err)
	return warnings, err
}

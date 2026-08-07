package cloudapi

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// minato_cloudapi_request_duration_seconds measures calls to the minato-cloud
// API. This package is used by minato-ctl (CLI) and other non-operator
// binaries, so it registers into the default prometheus registry — the
// controller-runtime registry is not available to its callers.
var cloudAPIRequestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "minato_cloudapi_request_duration_seconds",
	Help:    "Duration of minato-cloud API requests.",
	Buckets: prometheus.DefBuckets,
}, []string{"operation", "result"})

func init() {
	prometheus.MustRegister(cloudAPIRequestDuration)
}

// observeRequest records one cloud API call. result is "ok" or "error".
func observeRequest(operation string, start time.Time, err error) {
	result := "ok"
	if err != nil {
		result = "error"
	}
	cloudAPIRequestDuration.WithLabelValues(operation, result).Observe(time.Since(start).Seconds())
}

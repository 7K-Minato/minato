package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registrar metrics live in a dedicated registry (not controller-runtime's
// and not the default global registry) so the registrar binary exposes only
// its own series on its /metrics listener.
var metricsRegistry = prometheus.NewRegistry()

var (
	heartbeatsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "minato_registrar_heartbeats_total",
		Help: "Total heartbeats sent to minato cloud.",
	}, []string{"result"})

	metricsPushTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "minato_registrar_metrics_push_total",
		Help: "Total GameServer metrics pushes to minato cloud.",
	}, []string{"result"})
)

func init() {
	metricsRegistry.MustRegister(heartbeatsTotal, metricsPushTotal)
}

// startMetricsServer serves the registrar's /metrics endpoint on addr. It is
// scraped in-cluster, so no auth is applied. Empty addr disables it.
func startMetricsServer(ctx context.Context, addr string) {
	if addr == "" {
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(metricsRegistry, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() {
		log.Printf("metrics listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server error: %v", err)
		}
	}()
}

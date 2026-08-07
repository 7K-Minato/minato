package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetricsMiddleware(t *testing.T) {
	r := chi.NewRouter()
	r.Use(metricsMiddleware)
	r.Get("/api/v1/widgets/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/api/v1/widgets/{id}", "418"))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/widgets/42", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusTeapot {
		t.Fatalf("unexpected status: %d", rec.Code)
	}
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "/api/v1/widgets/{id}", "418"))
	if after != before+1 {
		t.Fatalf("expected counter increment, before=%f after=%f", before, after)
	}
	if testutil.CollectAndCount(httpRequestDuration) == 0 {
		t.Fatal("duration histogram must have observations")
	}
}

func TestMetricsMiddleware_UnmatchedRoute(t *testing.T) {
	r := chi.NewRouter()
	r.Use(metricsMiddleware)
	r.Get("/exists", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "unmatched", "404"))

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("GET", "unmatched", "404"))
	if after != before+1 {
		t.Fatalf("expected unmatched counter increment, before=%f after=%f", before, after)
	}
}

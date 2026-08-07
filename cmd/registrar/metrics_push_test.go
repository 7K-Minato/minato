package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchGameServerSummaries(t *testing.T) {
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/gameservers" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method %q", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer cp-key" {
			t.Errorf("missing/wrong auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"metadata": map[string]any{"name": "srv-1", "namespace": "tenant-a"},
				"spec":     map[string]any{"profile": "mc"},
				"status":   map[string]any{"state": "Running", "players": 3, "playerCapacity": 20},
			},
		})
	}))
	defer cp.Close()

	cfg := testConfig("http://cloud.invalid")
	cfg.ControlPlaneURL = cp.URL
	r := &registrar{cfg: cfg, client: cp.Client()}

	summaries, err := r.fetchGameServerSummaries(context.Background())
	if err != nil {
		t.Fatalf("fetchGameServerSummaries: %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected 1 summary, got %d", len(summaries))
	}
	s := summaries[0]
	if s.Name != "srv-1" || s.Namespace != "tenant-a" || s.Profile != "mc" ||
		s.State != "Running" || s.Players != 3 || s.PlayerCapacity != 20 {
		t.Fatalf("unexpected summary: %+v", s)
	}
}

func TestBuildMetricsPushPayload(t *testing.T) {
	now := time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)
	p := buildMetricsPushPayload(now, nil)
	if p.CollectedAt != "2026-08-07T12:00:00Z" {
		t.Fatalf("unexpected collectedAt: %q", p.CollectedAt)
	}
	if p.Servers == nil {
		t.Fatal("servers must serialize as [] not null")
	}
}

func TestPushMetrics(t *testing.T) {
	serversJSON := `[{"metadata":{"name":"srv-1","namespace":"tenant-a"},"spec":{"profile":"mc"},"status":{"state":"Running","players":1,"playerCapacity":10}}]`

	cases := []struct {
		name         string
		cloudStatus  int
		wantErr      bool
		wantPushPath string
	}{
		{name: "success", cloudStatus: http.StatusNoContent, wantErr: false, wantPushPath: "/api/v1/clusters/eu-1/metrics"},
		{name: "unsupported (404 tolerated)", cloudStatus: http.StatusNotFound, wantErr: false},
		{name: "server error", cloudStatus: http.StatusInternalServerError, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(serversJSON))
			}))
			defer cp.Close()

			var gotPath, gotAuth string
			var gotBody metricsPushPayload
			cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotAuth = r.Header.Get("Authorization")
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				w.WriteHeader(tc.cloudStatus)
			}))
			defer cloud.Close()

			cfg := testConfig(cloud.URL)
			cfg.ControlPlaneURL = cp.URL
			r := &registrar{cfg: cfg, client: cloud.Client()}

			err := r.pushMetrics(context.Background())
			if tc.wantErr && err == nil {
				t.Fatal("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantPushPath != "" && gotPath != tc.wantPushPath {
				t.Fatalf("unexpected push path: %q", gotPath)
			}
			if gotAuth != "Bearer tok" {
				t.Fatalf("unexpected auth header: %q", gotAuth)
			}
			if len(gotBody.Servers) != 1 || gotBody.Servers[0].Name != "srv-1" {
				t.Fatalf("unexpected push body: %+v", gotBody)
			}
			if gotBody.CollectedAt == "" {
				t.Fatal("collectedAt must be set")
			}
		})
	}
}

func TestPushMetrics_HeartbeatUnaffected(t *testing.T) {
	// Control plane down: push fails, heartbeat against the cloud still works.
	cp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	cp.Close() // immediately unreachable

	cloud := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer cloud.Close()

	cfg := testConfig(cloud.URL)
	cfg.ControlPlaneURL = cp.URL
	r := &registrar{cfg: cfg, client: cloud.Client()}

	if err := r.pushMetrics(context.Background()); err == nil {
		t.Fatal("expected push error")
	}
	ok, err := r.heartbeat(context.Background())
	if err != nil || !ok {
		t.Fatalf("heartbeat must be unaffected by push failure: ok=%v err=%v", ok, err)
	}
}

func TestRunMetricsPush_SkipsWithoutControlPlane(t *testing.T) {
	cfg := testConfig("http://cloud.invalid")
	cfg.ControlPlaneURL = ""
	cfg.ControlPlaneKey = ""
	cfg.MetricsPushInterval = 10 * time.Millisecond
	r := &registrar{cfg: cfg, client: &http.Client{Timeout: time.Second}}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	done := make(chan struct{})
	go func() {
		r.runMetricsPush(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("runMetricsPush did not return after ctx cancel")
	}
}

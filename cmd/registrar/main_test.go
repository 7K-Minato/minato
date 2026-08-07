package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(url string) config {
	return config{
		CloudURL:          url,
		RegisterToken:     "tok",
		Name:              "eu-1",
		Region:            "eu",
		ControlPlaneURL:   "http://cp:8080",
		ControlPlaneKey:   "cp-key",
		CapacityMax:       50,
		HeartbeatInterval: 50 * time.Millisecond,
		RequestTimeout:    2 * time.Second,
	}
}

func TestRegister(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "clu_1", "name": "eu-1"})
	}))
	defer srv.Close()

	r := &registrar{cfg: testConfig(srv.URL), client: srv.Client()}
	if err := r.register(context.Background()); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got["name"] != "eu-1" || got["controlplaneUrl"] != "http://cp:8080" || got["apiKey"] != "cp-key" {
		t.Fatalf("unexpected register body: %v", got)
	}
}

func TestHeartbeat(t *testing.T) {
	var beats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/clusters/eu-1/heartbeat" {
			beats.Add(1)
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := &registrar{cfg: testConfig(srv.URL), client: srv.Client()}
	ok, err := r.heartbeat(context.Background())
	if err != nil || !ok {
		t.Fatalf("heartbeat: ok=%v err=%v", ok, err)
	}
	if beats.Load() != 1 {
		t.Fatalf("expected 1 beat, got %d", beats.Load())
	}
}

func TestHeartbeatUnknownCluster(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	r := &registrar{cfg: testConfig(srv.URL), client: srv.Client()}
	ok, err := r.heartbeat(context.Background())
	if err != nil {
		t.Fatalf("404 should not be an error: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false on 404")
	}
}

func TestRunRegistersThenHeartbeats(t *testing.T) {
	var registers, beats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/clusters/register":
			registers.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "clu_1"})
		case "/api/v1/clusters/eu-1/heartbeat":
			beats.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r := &registrar{cfg: testConfig(srv.URL), client: srv.Client()}
	r.run(ctx)

	if registers.Load() != 1 {
		t.Fatalf("expected 1 register, got %d", registers.Load())
	}
	if beats.Load() < 2 {
		t.Fatalf("expected multiple heartbeats, got %d", beats.Load())
	}
}

func TestRunReregistersOn404(t *testing.T) {
	var registers, beats atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/clusters/register":
			registers.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "clu_1"})
		case "/api/v1/clusters/eu-1/heartbeat":
			if beats.Add(1) == 1 {
				w.WriteHeader(http.StatusNotFound) // cloud forgot us once
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	r := &registrar{cfg: testConfig(srv.URL), client: srv.Client()}
	r.run(ctx)

	if registers.Load() != 2 {
		t.Fatalf("expected re-register after 404, got %d registers", registers.Load())
	}
}

func TestLoadConfigRequired(t *testing.T) {
	t.Setenv("CLOUD_URL", "")
	t.Setenv("REGISTER_TOKEN", "")
	t.Setenv("CLUSTER_NAME", "")
	t.Setenv("CONTROLPLANE_API_KEY", "")
	if _, err := loadConfig(); err == nil {
		t.Fatal("expected error for missing required env")
	}

	t.Setenv("CLOUD_URL", "http://cloud:8080")
	t.Setenv("REGISTER_TOKEN", "tok")
	t.Setenv("CLUSTER_NAME", "eu-1")
	t.Setenv("CONTROLPLANE_API_KEY", "k")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	if cfg.ControlPlaneURL != "http://minato-controlplane:8080" || cfg.CapacityMax != 100 || cfg.HeartbeatInterval != 30*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

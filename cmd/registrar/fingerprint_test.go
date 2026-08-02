package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestResolveClusterFingerprintOverride(t *testing.T) {
	got := resolveClusterFingerprint("fp-explicit", "/nonexistent/token", "/nonexistent/ca")
	if got != "fp-explicit" {
		t.Fatalf("expected override to win, got %q", got)
	}
}

func TestResolveClusterFingerprintNoToken(t *testing.T) {
	got := resolveClusterFingerprint("", t.TempDir()+"/missing-token", t.TempDir()+"/missing-ca")
	if got != "" {
		t.Fatalf("expected empty fingerprint without token, got %q", got)
	}
}

func TestKubeSystemUID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/namespaces/kube-system" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]any{"uid": "9f0c1a2e-1234-5678-9abc-def012345678"},
		})
	}))
	defer srv.Close()

	uid, err := kubeSystemUID(context.Background(), srv.Client(), srv.URL, "tok")
	if err != nil {
		t.Fatalf("kubeSystemUID: %v", err)
	}
	if uid != "9f0c1a2e-1234-5678-9abc-def012345678" {
		t.Fatalf("unexpected uid: %q", uid)
	}
}

func TestKubeSystemUIDErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	if _, err := kubeSystemUID(context.Background(), srv.Client(), srv.URL, "tok"); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}

func TestKubeSystemUIDEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{}})
	}))
	defer srv.Close()

	if _, err := kubeSystemUID(context.Background(), srv.Client(), srv.URL, "tok"); err == nil {
		t.Fatal("expected error on empty uid")
	}
}

func TestBuildRegisterPayloadFingerprint(t *testing.T) {
	cfg := testConfig("http://cloud:8080")

	with := buildRegisterPayload(cfg, "fp-1")
	if with.ClusterFingerprint != "fp-1" {
		t.Fatalf("expected fingerprint in payload, got %q", with.ClusterFingerprint)
	}
	raw, err := json.Marshal(with)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["clusterFingerprint"] != "fp-1" {
		t.Fatalf("expected clusterFingerprint in JSON, got %v", m)
	}

	without := buildRegisterPayload(cfg, "")
	raw, err = json.Marshal(without)
	if err != nil {
		t.Fatal(err)
	}
	m = map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["clusterFingerprint"]; ok {
		t.Fatalf("expected clusterFingerprint omitted when empty, got %v", m)
	}
	if m["name"] != "eu-1" || m["apiKey"] != "cp-key" {
		t.Fatalf("unexpected register payload: %v", m)
	}
}

func TestBuildHeartbeatPayloadFingerprint(t *testing.T) {
	cfg := testConfig("http://cloud:8080")

	raw, err := json.Marshal(buildHeartbeatPayload(cfg, "fp-1"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["clusterFingerprint"] != "fp-1" {
		t.Fatalf("expected clusterFingerprint in JSON, got %v", m)
	}

	raw, err = json.Marshal(buildHeartbeatPayload(cfg, ""))
	if err != nil {
		t.Fatal(err)
	}
	m = map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["clusterFingerprint"]; ok {
		t.Fatalf("expected clusterFingerprint omitted when empty, got %v", m)
	}
}

func TestInClusterBaseURL(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")
	if got := inClusterBaseURL(); got != "https://kubernetes.default.svc:443" {
		t.Fatalf("unexpected default: %q", got)
	}

	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "6443")
	if got := inClusterBaseURL(); got != "https://10.96.0.1:6443" {
		t.Fatalf("unexpected override: %q", got)
	}
}

func TestRegisterSendsFingerprint(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"id": "clu_1"})
	}))
	defer srv.Close()

	r := &registrar{cfg: testConfig(srv.URL), fingerprint: "fp-1", client: srv.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.register(ctx); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got["clusterFingerprint"] != "fp-1" {
		t.Fatalf("expected fingerprint in register body, got %v", got)
	}
}

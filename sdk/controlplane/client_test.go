package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, "test-key", 5*time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, srv
}

func TestCreateAndGetGameServer(t *testing.T) {
	var created map[string]any
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/gameservers/tenant-1":
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Errorf("decode create body: %v", err)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(created)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/gameservers/tenant-1/mc-1":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": "mc-1", "namespace": "tenant-1"},
				"spec":     map[string]any{"profile": "minecraft"},
				"status":   map[string]any{"state": "Running", "endpoints": []map[string]any{{"name": "game", "address": "1.2.3.4", "port": 25565}}},
			})
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	})

	gs := &GameServer{Metadata: ObjectMeta{Name: "mc-1", Labels: map[string]string{"team": "a"}}}
	gs.Spec.Profile = "minecraft"
	gs.Spec.Env = map[string]string{"EULA": "true"}
	gs.Spec.Storage.Size = "10Gi"
	gs.Spec.Lifecycle.AutoStart = true

	if _, err := c.CreateGameServer(t.Context(), "tenant-1", gs); err != nil {
		t.Fatalf("create: %v", err)
	}
	meta, _ := created["metadata"].(map[string]any)
	if meta["name"] != "mc-1" {
		t.Fatalf("unexpected create body: %v", created)
	}
	spec, _ := created["spec"].(map[string]any)
	if spec["profile"] != "minecraft" {
		t.Fatalf("unexpected spec: %v", spec)
	}
	lc, _ := spec["lifecycle"].(map[string]any)
	if lc["autoStart"] != true {
		t.Fatalf("expected autoStart true: %v", lc)
	}

	got, err := c.GetGameServer(t.Context(), "tenant-1", "mc-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.State != "Running" || len(got.Status.Endpoints) != 1 || got.Status.Endpoints[0].Port != 25565 {
		t.Fatalf("unexpected status: %+v", got.Status)
	}
}

func TestErrorEnvelope(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "gameserver not found"})
	})

	_, err := c.GetGameServer(t.Context(), "tenant-1", "missing")
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Status != http.StatusNotFound || apiErr.Message != "gameserver not found" {
		t.Fatalf("unexpected error: %+v", apiErr)
	}
}

func TestExecuteActionAndSnapshots(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/gameservers/ns/gs/actions/restart":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]string{"name": "gs-restart-123"})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/gameservers/ns/gs/snapshots":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"metadata": map[string]any{"name": "snap1"}, "spec": map[string]any{"gameServerRef": "gs"}, "status": map[string]any{"state": "Ready"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/gameservers/ns/gs/snapshots":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"name": "snap2"}, "spec": map[string]any{"gameServerRef": "gs"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	ref, err := c.ExecuteAction(t.Context(), "ns", "gs", "restart", map[string]string{"reason": "test"})
	if err != nil || ref.Name != "gs-restart-123" {
		t.Fatalf("execute action: %v %+v", err, ref)
	}

	snaps, err := c.ListSnapshots(t.Context(), "ns", "gs")
	if err != nil || len(snaps) != 1 || snaps[0].Status.State != "Ready" {
		t.Fatalf("list snapshots: %v %+v", err, snaps)
	}

	snap, err := c.CreateSnapshot(t.Context(), "ns", "gs")
	if err != nil || snap.Metadata.Name != "snap2" {
		t.Fatalf("create snapshot: %v %+v", err, snap)
	}
}

func TestListProfilesIncludesParams(t *testing.T) {
	c, _ := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{
				"metadata": map[string]any{"name": "minecraft"},
				"spec": map[string]any{
					"displayName": "Minecraft",
					"image":       "mc:latest",
					"actions": []map[string]any{
						{"name": "say", "description": "Broadcast", "params": map[string]any{"message": map[string]any{"type": "string", "required": true}}},
					},
				},
			},
		})
	})

	profiles, err := c.ListProfiles(t.Context())
	if err != nil || len(profiles) != 1 {
		t.Fatalf("list profiles: %v %+v", err, profiles)
	}
	a := profiles[0].Spec.Actions[0]
	if a.Name != "say" || !a.Params["message"].Required {
		t.Fatalf("action params lost: %+v", a)
	}
}

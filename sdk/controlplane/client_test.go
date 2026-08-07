package controlplane

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, "test-key", 5*time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c
}

func TestCreateAndGetGameServer(t *testing.T) {
	var created map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestUpdateGameServerLifecycle(t *testing.T) {
	var gotBody map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPatch && r.URL.Path == "/api/v1/gameservers/ns/gs" {
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decode patch body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": "gs", "namespace": "ns"},
				"spec": map[string]any{
					"profile":   "minecraft",
					"lifecycle": map[string]any{"autoStart": false, "idleTimeoutSeconds": 120},
				},
				"status": map[string]any{"state": "Stopped"},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	autoStart := false
	idle := int32(120)
	gs, err := c.UpdateGameServerLifecycle(t.Context(), "ns", "gs", &autoStart, &idle)
	if err != nil {
		t.Fatalf("patch lifecycle: %v", err)
	}
	lc, _ := gotBody["spec"].(map[string]any)["lifecycle"].(map[string]any)
	if lc["autoStart"] != false || lc["idleTimeoutSeconds"] != float64(120) {
		t.Fatalf("unexpected patch body: %v", gotBody)
	}
	if gs.Status.State != "Stopped" || gs.Spec.Lifecycle.AutoStart != false || gs.Spec.Lifecycle.IdleTimeoutSeconds != 120 {
		t.Fatalf("unexpected response: %+v", gs)
	}

	// nil fields must be omitted from the request body
	gotBody = nil
	if _, err := c.UpdateGameServerLifecycle(t.Context(), "ns", "gs", nil, nil); err != nil {
		t.Fatalf("patch lifecycle nil: %v", err)
	}
	lc, _ = gotBody["spec"].(map[string]any)["lifecycle"].(map[string]any)
	if _, ok := lc["autoStart"]; ok {
		t.Fatalf("autoStart must be omitted when nil: %v", gotBody)
	}
	if _, ok := lc["idleTimeoutSeconds"]; ok {
		t.Fatalf("idleTimeoutSeconds must be omitted when nil: %v", gotBody)
	}
}

func TestFleetWriteEndpoints(t *testing.T) {
	var gotBody map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/gameserverfleets/ns":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decode create body: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(gotBody)
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/gameserverfleets/ns/f1":
			if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
				t.Errorf("decode patch body: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"metadata": map[string]any{"name": "f1", "namespace": "ns"},
				"spec":     map[string]any{"profile": "minecraft", "replicas": 5},
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/gameserverfleets/ns/f1":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	fleet := &GameServerFleet{Metadata: ObjectMeta{Name: "f1"}}
	fleet.Spec.Profile = "minecraft"
	fleet.Spec.Replicas = 2
	created, err := c.CreateGameServerFleet(t.Context(), "ns", fleet)
	if err != nil {
		t.Fatalf("create fleet: %v", err)
	}
	spec, _ := gotBody["spec"].(map[string]any)
	if spec["profile"] != "minecraft" || spec["replicas"] != float64(2) {
		t.Fatalf("unexpected create body: %v", gotBody)
	}
	if created.Metadata.Name != "f1" {
		t.Fatalf("unexpected created fleet: %+v", created)
	}

	scaled, err := c.ScaleGameServerFleet(t.Context(), "ns", "f1", 5)
	if err != nil {
		t.Fatalf("scale fleet: %v", err)
	}
	spec, _ = gotBody["spec"].(map[string]any)
	if spec["replicas"] != float64(5) {
		t.Fatalf("unexpected patch body: %v", gotBody)
	}
	if scaled.Spec.Replicas != 5 {
		t.Fatalf("unexpected scaled fleet: %+v", scaled)
	}

	if err := c.DeleteGameServerFleet(t.Context(), "ns", "f1"); err != nil {
		t.Fatalf("delete fleet: %v", err)
	}
}

func TestTraceContextPropagation(t *testing.T) {
	var gotTraceparent string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotTraceparent = r.Header.Get("traceparent")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{})
	})

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prevProp := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prevProp) })
	ctx, span := tp.Tracer("test").Start(t.Context(), "parent")
	defer span.End()

	if _, err := c.ListGameServers(ctx); err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotTraceparent == "" {
		t.Fatal("control plane request did not carry a traceparent header")
	}
	if !strings.Contains(gotTraceparent, span.SpanContext().TraceID().String()) {
		t.Fatalf("traceparent %q does not reference parent trace %s", gotTraceparent, span.SpanContext().TraceID())
	}
}

func TestGetSFTPInfo(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/gameservers/tenant-1/mc-1/sftp":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"host":     "mc-1.games.example.com",
				"port":     2022,
				"username": "minato",
				"password": "0123456789abcdef0123456789abcdef",
			})
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	})

	info, err := c.GetSFTPInfo(t.Context(), "tenant-1", "mc-1")
	if err != nil {
		t.Fatalf("get sftp info: %v", err)
	}
	if info.Host != "mc-1.games.example.com" || info.Port != 2022 ||
		info.Username != "minato" || info.Password != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unexpected sftp info: %+v", info)
	}

	if _, err := c.GetSFTPInfo(t.Context(), "tenant-1", "missing"); err == nil {
		t.Fatal("expected error for missing server")
	} else {
		var apiErr *APIError
		if !errors.As(err, &apiErr) || apiErr.Status != http.StatusNotFound {
			t.Fatalf("expected 404 APIError, got %v", err)
		}
	}
}

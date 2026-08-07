package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func lifecycleTestServer() *operatorv1.GameServer {
	return &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec: operatorv1.GameServerSpec{
			Profile: "minecraft",
			Lifecycle: operatorv1.LifecycleSpec{
				AutoStart:          new(true),
				IdleTimeoutSeconds: 300,
			},
		},
	}
}

func TestUpdateGameServerLifecycle(t *testing.T) {
	cases := []struct {
		name           string
		body           string
		wantStatus     int
		wantAutoStart  *bool
		wantIdleSecs   int32
		checkUnchanged bool
	}{
		{
			name:          "set autoStart false",
			body:          `{"spec":{"lifecycle":{"autoStart":false}}}`,
			wantStatus:    http.StatusOK,
			wantAutoStart: new(false),
			wantIdleSecs:  300, // untouched
		},
		{
			name:          "set idleTimeoutSeconds",
			body:          `{"spec":{"lifecycle":{"idleTimeoutSeconds":60}}}`,
			wantStatus:    http.StatusOK,
			wantAutoStart: new(true), // untouched
			wantIdleSecs:  60,
		},
		{
			name:          "set both",
			body:          `{"spec":{"lifecycle":{"autoStart":true,"idleTimeoutSeconds":0}}}`,
			wantStatus:    http.StatusOK,
			wantAutoStart: new(true),
			wantIdleSecs:  0,
		},
		{
			name:       "reject profile change",
			body:       `{"spec":{"profile":"other"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reject status change",
			body:       `{"status":{"state":"Running"}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reject metadata change",
			body:       `{"metadata":{"labels":{"a":"b"}}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reject unknown lifecycle field",
			body:       `{"spec":{"lifecycle":{"restartPolicy":"Always"}}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reject negative idleTimeoutSeconds",
			body:       `{"spec":{"lifecycle":{"idleTimeoutSeconds":-5}}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reject empty patch",
			body:       `{"spec":{"lifecycle":{}}}`,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "reject invalid json",
			body:       `not-json`,
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api, cl := setupTestAPI(lifecycleTestServer())
			r := newRouter(api)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/gameservers/default/gs1", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(rec, req)

			if rec.Code != c.wantStatus {
				t.Fatalf("expected %d, got %d: %s", c.wantStatus, rec.Code, rec.Body.String())
			}

			var updated operatorv1.GameServer
			if err := cl.Get(context.Background(), types.NamespacedName{Name: "gs1", Namespace: "default"}, &updated); err != nil {
				t.Fatalf("get gameserver: %v", err)
			}
			if c.wantStatus == http.StatusOK {
				if c.wantAutoStart == nil {
					if updated.Spec.Lifecycle.AutoStart != nil {
						t.Fatalf("expected autoStart untouched (nil), got %v", *updated.Spec.Lifecycle.AutoStart)
					}
				} else if updated.Spec.Lifecycle.AutoStart == nil || *updated.Spec.Lifecycle.AutoStart != *c.wantAutoStart {
					t.Fatalf("expected autoStart %v, got %+v", *c.wantAutoStart, updated.Spec.Lifecycle.AutoStart)
				}
				if updated.Spec.Lifecycle.IdleTimeoutSeconds != c.wantIdleSecs {
					t.Fatalf("expected idleTimeoutSeconds %d, got %d", c.wantIdleSecs, updated.Spec.Lifecycle.IdleTimeoutSeconds)
				}
			}
		})
	}
}

func TestUpdateGameServerLifecycle_NotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gameservers/default/missing", strings.NewReader(`{"spec":{"lifecycle":{"autoStart":false}}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestCreateGameServerFleet_Success(t *testing.T) {
	profile := &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "minecraft"},
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Minecraft",
			Image:       "minecraft:latest",
			Storage:     operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "10Gi"},
			Agent:       operatorv1.AgentSpec{Image: "minato/minecraft-agent", Version: "v1"},
		},
	}
	api, c := setupTestAPI(profile)
	r := newRouter(api)

	fleet := operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-new"},
		Spec:       operatorv1.GameServerFleetSpec{Profile: "minecraft", Replicas: 3},
	}
	body, _ := json.Marshal(fleet)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameserverfleets/tenant-f", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created operatorv1.GameServerFleet
	if err := c.Get(context.Background(), types.NamespacedName{Name: "fleet-new", Namespace: "tenant-f"}, &created); err != nil {
		t.Fatalf("expected created fleet: %v", err)
	}
	if created.Spec.Replicas != 3 {
		t.Fatalf("unexpected replicas: %d", created.Spec.Replicas)
	}

	// Namespace auto-created like gameservers
	var ns corev1.Namespace
	if err := c.Get(context.Background(), types.NamespacedName{Name: "tenant-f"}, &ns); err != nil {
		t.Fatalf("expected namespace to be auto-created: %v", err)
	}
}

func TestCreateGameServerFleet_ProfileMissing(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	fleet := operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet-new"},
		Spec:       operatorv1.GameServerFleetSpec{Profile: "missing", Replicas: 1},
	}
	body, _ := json.Marshal(fleet)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameserverfleets/default", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestCreateGameServerFleet_NegativeReplicas(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameserverfleets/default",
		strings.NewReader(`{"metadata":{"name":"f"},"spec":{"profile":"minecraft","replicas":-1}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateGameServerFleet(t *testing.T) {
	newFleet := func() *operatorv1.GameServerFleet {
		return &operatorv1.GameServerFleet{
			ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "default"},
			Spec:       operatorv1.GameServerFleetSpec{Profile: "minecraft", Replicas: 2},
		}
	}

	cases := []struct {
		name         string
		body         string
		wantStatus   int
		wantReplicas int32
	}{
		{"scale up", `{"spec":{"replicas":5}}`, http.StatusOK, 5},
		{"scale to zero", `{"spec":{"replicas":0}}`, http.StatusOK, 0},
		{"reject profile change", `{"spec":{"profile":"other"}}`, http.StatusBadRequest, 2},
		{"reject negative replicas", `{"spec":{"replicas":-2}}`, http.StatusBadRequest, 2},
		{"reject empty patch", `{"spec":{}}`, http.StatusBadRequest, 2},
		{"reject template change", `{"spec":{"template":{"spec":{"env":{"A":"B"}}}}}`, http.StatusBadRequest, 2},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			api, cl := setupTestAPI(newFleet())
			r := newRouter(api)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/gameserverfleets/default/fleet1", strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(rec, req)

			if rec.Code != c.wantStatus {
				t.Fatalf("expected %d, got %d: %s", c.wantStatus, rec.Code, rec.Body.String())
			}

			var updated operatorv1.GameServerFleet
			if err := cl.Get(context.Background(), types.NamespacedName{Name: "fleet1", Namespace: "default"}, &updated); err != nil {
				t.Fatalf("get fleet: %v", err)
			}
			if updated.Spec.Replicas != c.wantReplicas {
				t.Fatalf("expected replicas %d, got %d", c.wantReplicas, updated.Spec.Replicas)
			}
		})
	}
}

func TestUpdateGameServerFleet_NotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/gameserverfleets/default/missing", strings.NewReader(`{"spec":{"replicas":2}}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestDeleteGameServerFleet_Success(t *testing.T) {
	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "default"},
		Spec:       operatorv1.GameServerFleetSpec{Profile: "minecraft", Replicas: 1},
	}
	api, c := setupTestAPI(fleet)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gameserverfleets/default/fleet1", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var remaining operatorv1.GameServerFleet
	if err := c.Get(context.Background(), types.NamespacedName{Name: "fleet1", Namespace: "default"}, &remaining); err == nil {
		t.Fatalf("expected fleet to be deleted")
	}
}

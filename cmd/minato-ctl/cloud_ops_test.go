package main

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCloudSnapshots(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_snap"))
	srv, reqs := newFakeCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/me/tenants":
			_, _ = w.Write([]byte(singleTenantJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tenants/t-1/servers/srv-1/snapshots":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"metadata": map[string]any{"name": "snap-1"}, "status": map[string]any{"state": "Ready", "size": "1Gi"}},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tenants/t-1/servers/srv-1/snapshots":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"name": "snap-2"}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	if err := runCloud(t, "snapshots", "list", "srv-1", "--url", srv.URL); err != nil {
		t.Fatalf("snapshots list: %v", err)
	}
	if err := runCloud(t, "snapshots", "create", "srv-1", "--url", srv.URL); err != nil {
		t.Fatalf("snapshots create: %v", err)
	}
	last := (*reqs)[len(*reqs)-1]
	if last.Method != http.MethodPost || last.Path != "/api/v1/tenants/t-1/servers/srv-1/snapshots" {
		t.Fatalf("unexpected create request: %+v", last)
	}
	if last.Auth != "Bearer mk_snap" {
		t.Fatalf("missing auth header: %+v", last)
	}
}

func TestCloudActions(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_act"))
	srv, reqs := newFakeCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/me/tenants":
			_, _ = w.Write([]byte(singleTenantJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tenants/t-1/servers/srv-1/actions":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "restart", "description": "Restart the server"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tenants/t-1/servers/srv-1/actions/say":
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "exec-1"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	if err := runCloud(t, "actions", "list", "srv-1", "--url", srv.URL); err != nil {
		t.Fatalf("actions list: %v", err)
	}
	if err := runCloud(t, "actions", "run", "srv-1", "say", "--url", srv.URL, "--param", "message=hello"); err != nil {
		t.Fatalf("actions run: %v", err)
	}
	last := (*reqs)[len(*reqs)-1]
	if last.Method != http.MethodPost || last.Path != "/api/v1/tenants/t-1/servers/srv-1/actions/say" {
		t.Fatalf("unexpected run request: %+v", last)
	}
	var body map[string]string
	if err := json.Unmarshal(last.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["message"] != "hello" {
		t.Fatalf("unexpected action params: %v", body)
	}
}

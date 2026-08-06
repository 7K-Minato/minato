package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func serversFake(t *testing.T, tenants string) (*[]recordedRequest, string) {
	fake := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/me/tenants":
			_, _ = w.Write([]byte(tenants))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tenants/t-1/servers":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "srv-1", "tenant_id": "t-1", "cluster_id": "c-1", "namespace": "ns",
					"name": "lobby", "profile": "minecraft", "status": "Running"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tenants/t-1/servers":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "srv-2", "tenant_id": "t-1", "cluster_id": "c-1", "namespace": "ns",
				"name": "new", "profile": "minecraft", "status": "Provisioning",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/tenants/t-1/servers/srv-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such route"})
		}
	})
	srv, reqs := newFakeCloud(t, fake)
	return reqs, srv.URL
}

const singleTenantJSON = `[{"id":"t-1","slug":"acme","name":"Acme"}]`

func TestCloudServersList(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_srv"))
	reqs, url := serversFake(t, singleTenantJSON)

	if err := runCloud(t, "servers", "list", "--url", url); err != nil {
		t.Fatalf("servers list: %v", err)
	}
	// First call resolves the tenant via me/tenants, second lists servers.
	if len(*reqs) != 2 {
		t.Fatalf("expected 2 requests, got %+v", *reqs)
	}
	if (*reqs)[0].Path != "/api/v1/me/tenants" || (*reqs)[1].Path != "/api/v1/tenants/t-1/servers" {
		t.Fatalf("unexpected paths: %+v", *reqs)
	}
	for _, r := range *reqs {
		if r.Auth != "Bearer mk_srv" {
			t.Fatalf("missing auth header: %+v", r)
		}
	}
}

func TestCloudServersTenantResolutionBySlug(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_srv"))
	reqs, url := serversFake(t, `[{"id":"t-1","slug":"acme","name":"Acme"},{"id":"t-2","slug":"other","name":"Other"}]`)

	if err := runCloud(t, "servers", "list", "--url", url, "--tenant", "acme"); err != nil {
		t.Fatalf("servers list: %v", err)
	}
	if (*reqs)[1].Path != "/api/v1/tenants/t-1/servers" {
		t.Fatalf("expected tenant t-1 resolution by slug: %+v", *reqs)
	}
}

func TestCloudServersAmbiguousTenant(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_srv"))
	_, url := serversFake(t, `[{"id":"t-1","slug":"acme","name":"Acme"},{"id":"t-2","slug":"other","name":"Other"}]`)

	err := runCloud(t, "servers", "list", "--url", url)
	if err == nil || !strings.Contains(err.Error(), "--tenant") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}

func TestCloudServersCreatePayload(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_srv"))
	reqs, url := serversFake(t, singleTenantJSON)

	err := runCloud(t, "servers", "create", "--url", url,
		"--name", "new", "--profile", "minecraft",
		"--tier", "small", "--region", "eu", "--storage", "20Gi",
		"--env", "DIFFICULTY=hard,MAX_PLAYERS=10")
	if err != nil {
		t.Fatalf("servers create: %v", err)
	}
	if len(*reqs) != 2 || (*reqs)[1].Method != http.MethodPost {
		t.Fatalf("expected POST after tenant resolution: %+v", *reqs)
	}
	var body map[string]any
	if err := json.Unmarshal((*reqs)[1].Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["name"] != "new" || body["profile"] != "minecraft" || body["tier"] != "small" ||
		body["region"] != "eu" || body["storageSize"] != "20Gi" {
		t.Fatalf("unexpected create payload: %v", body)
	}
	env, ok := body["env"].(map[string]any)
	if !ok || env["DIFFICULTY"] != "hard" || env["MAX_PLAYERS"] != "10" {
		t.Fatalf("unexpected env payload: %v", body["env"])
	}
}

func TestCloudServersDelete(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_srv"))
	reqs, url := serversFake(t, singleTenantJSON)

	if err := runCloud(t, "servers", "delete", "srv-1", "--url", url); err != nil {
		t.Fatalf("servers delete: %v", err)
	}
	last := (*reqs)[len(*reqs)-1]
	if last.Method != http.MethodDelete || last.Path != "/api/v1/tenants/t-1/servers/srv-1" {
		t.Fatalf("unexpected delete request: %+v", last)
	}
}

func TestCloudServersQuotaError(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_srv"))
	srv, _ := newFakeCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/me/tenants" {
			_, _ = w.Write([]byte(singleTenantJSON))
			return
		}
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server quota exceeded"})
	})

	err := runCloud(t, "servers", "create", "--url", srv.URL, "--name", "x", "--profile", "minecraft")
	if err == nil || !strings.Contains(err.Error(), "forbidden") || !strings.Contains(err.Error(), "server quota exceeded") {
		t.Fatalf("expected 403 quota mapping, got %v", err)
	}
}

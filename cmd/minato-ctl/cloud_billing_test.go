package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestCloudCatalogPlansSubscription(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_bill"))
	srv, reqs := newFakeCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/me/tenants":
			_, _ = w.Write([]byte(singleTenantJSON))
		case r.URL.Path == "/api/v1/tenants/t-1/catalog":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"name": "minecraft", "displayName": "Minecraft", "category": "sandbox",
					"capabilities": []string{"backup"}, "regions": []string{"eu", "us"},
					"tiers": []map[string]any{{"name": "small"}, {"name": "large"}}},
			})
		case r.URL.Path == "/api/v1/plans":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "pro", "display_name": "Pro", "max_servers": 5, "max_storage_gb": 100,
					"isolation": "shared", "monthly_price_cents": 999},
			})
		case r.URL.Path == "/api/v1/tenants/t-1/subscription":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"tenant_id": "t-1", "plan_id": "pro", "status": "active",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	for _, args := range [][]string{
		{"catalog"},
		{"plans"},
		{"subscription"},
	} {
		if err := runCloud(t, append(args, "--url", srv.URL)...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	// plans is not tenant-scoped: it must not hit me/tenants.
	seenPlans := false
	for _, r := range *reqs {
		if r.Path == "/api/v1/plans" {
			seenPlans = true
			if r.Auth != "Bearer mk_bill" {
				t.Fatalf("plans without auth: %+v", r)
			}
		}
	}
	if !seenPlans {
		t.Fatalf("plans endpoint not called: %+v", *reqs)
	}
}

func TestCloudSubscriptionPaymentRequired(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_bill"))
	srv, _ := newFakeCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/me/tenants" {
			_, _ = w.Write([]byte(singleTenantJSON))
			return
		}
		w.WriteHeader(http.StatusPaymentRequired)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no active subscription"})
	})

	err := runCloud(t, "subscription", "--url", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "subscribe") {
		t.Fatalf("expected 402 mapping, got %v", err)
	}
}

func TestCloudAPIKeys(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_admin"))
	srv, reqs := newFakeCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/me/tenants":
			_, _ = w.Write([]byte(singleTenantJSON))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tenants/t-1/apikeys":
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"id": "k-1", "tenant_id": "t-1", "name": "ci", "prefix": "abcd1234",
					"scopes": []string{"servers:read"}, "created_by": "me", "created_at": "2026-08-01T00:00:00Z"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/tenants/t-1/apikeys":
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"id": "k-2", "tenant_id": "t-1", "name": "deploy", "prefix": "beef5678",
				"scopes": []string{"servers:write"}, "created_by": "me", "created_at": "2026-08-02T00:00:00Z",
				"key": "mk_beef5678_secret",
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/api/v1/tenants/t-1/apikeys/k-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})

	if err := runCloud(t, "apikeys", "list", "--url", srv.URL); err != nil {
		t.Fatalf("apikeys list: %v", err)
	}
	if err := runCloud(t, "apikeys", "create", "--name", "deploy", "--scope", "servers:write,snapshots", "--url", srv.URL); err != nil {
		t.Fatalf("apikeys create: %v", err)
	}
	if err := runCloud(t, "apikeys", "delete", "k-1", "--url", srv.URL); err != nil {
		t.Fatalf("apikeys delete: %v", err)
	}

	var create *recordedRequest
	for i := range *reqs {
		r := &(*reqs)[i]
		if r.Method == http.MethodPost && strings.HasSuffix(r.Path, "/apikeys") {
			create = r
		}
	}
	if create == nil {
		t.Fatalf("create request not recorded: %+v", *reqs)
	}
	var body map[string]any
	if err := json.Unmarshal(create.Body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["name"] != "deploy" {
		t.Fatalf("unexpected apikey payload: %v", body)
	}
	scopes, ok := body["scopes"].([]any)
	if !ok || len(scopes) != 2 || scopes[0] != "servers:write" || scopes[1] != "snapshots" {
		t.Fatalf("unexpected scopes: %v", body["scopes"])
	}
}

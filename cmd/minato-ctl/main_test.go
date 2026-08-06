package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNewClient(t *testing.T) {
	serverAddr = "http://localhost:8080"
	apiKey = "k"
	if _, err := newClient(); err != nil {
		t.Fatalf("newClient: %v", err)
	}

	serverAddr = "://bad-url"
	if _, err := newClient(); err == nil {
		t.Fatal("expected error for invalid server address")
	}
}

func TestListServersViaSDK(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/api/v1/gameservers" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{
				{"metadata": map[string]any{"name": "gs1", "namespace": "minato"}, "spec": map[string]any{"profile": "minecraft"}},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	serverAddr = server.URL
	apiKey = "test-key"

	c, err := newClient()
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	servers, err := c.ListGameServers(context.Background())
	if err != nil {
		t.Fatalf("ListGameServers: %v", err)
	}
	if len(servers) != 1 || servers[0].Metadata.Name != "gs1" || servers[0].Spec.Profile != "minecraft" {
		t.Fatalf("unexpected servers: %+v", servers)
	}
	if err := printJSON(servers); err != nil {
		t.Fatalf("printJSON: %v", err)
	}
}

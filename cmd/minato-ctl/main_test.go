package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/test" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	serverAddr = server.URL

	// Should not error on valid response
	err := getJSON("/api/v1/test")
	if err != nil {
		t.Fatalf("getJSON failed: %v", err)
	}
}

func TestGetJSON_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "not found"})
	}))
	defer server.Close()

	serverAddr = server.URL

	// Should not fail on 404, just print the error response
	err := getJSON("/api/v1/missing")
	if err != nil {
		t.Fatalf("getJSON should not fail on 404: %v", err)
	}
}

func TestPostJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}

		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	defer server.Close()

	serverAddr = server.URL

	data := map[string]string{"key": "value"}
	err := postJSON("/api/v1/test", data)
	if err != nil {
		t.Fatalf("postJSON failed: %v", err)
	}
}

func TestPostJSON_NilData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "created"})
	}))
	defer server.Close()

	serverAddr = server.URL

	err := postJSON("/api/v1/test", nil)
	if err != nil {
		t.Fatalf("postJSON with nil data failed: %v", err)
	}
}

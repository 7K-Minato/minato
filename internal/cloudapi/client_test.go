package cloudapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *[]string) {
	t.Helper()
	var auths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auths = append(auths, r.Header.Get("Authorization"))
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	c, err := NewClient(srv.URL, "mk_test", 5*time.Second)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return c, &auths
}

func TestAuthHeaderAttached(t *testing.T) {
	c, auths := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "t-1", "slug": "acme", "name": "Acme"},
		})
	})
	if _, err := c.ListMyTenants(context.Background()); err != nil {
		t.Fatalf("ListMyTenants: %v", err)
	}
	if len(*auths) != 1 || (*auths)[0] != "Bearer mk_test" {
		t.Fatalf("auth header: %v", *auths)
	}
}

func TestResolveTenant(t *testing.T) {
	tenants := `[{"id":"t-1","slug":"acme","name":"Acme"},{"id":"t-2","slug":"other","name":"Other"}]`
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(tenants))
	})
	ctx := context.Background()

	for _, ref := range []string{"t-2", "other", "Other", "OTHER"} {
		got, err := c.ResolveTenant(ctx, ref)
		if err != nil || got.Id != "t-2" {
			t.Fatalf("ref %q: got %+v, err %v", ref, got, err)
		}
	}
	if _, err := c.ResolveTenant(ctx, ""); err == nil || !strings.Contains(err.Error(), "multiple tenants") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
	if _, err := c.ResolveTenant(ctx, "nope"); err == nil || !strings.Contains(err.Error(), "no tenant matching") {
		t.Fatalf("expected no-match error, got %v", err)
	}
}

func TestResolveTenantSingleDefault(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"t-1","slug":"acme","name":"Acme"}]`))
	})
	got, err := c.ResolveTenant(context.Background(), "")
	if err != nil || got.Id != "t-1" {
		t.Fatalf("got %+v, err %v", got, err)
	}
}

func TestAPIErrorEnvelope(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "server quota exceeded"})
	})
	_, err := c.ListMyTenants(context.Background())
	var ae *APIError
	if !errors.As(err, &ae) {
		t.Fatalf("expected APIError, got %T %v", err, err)
	}
	if ae.Status != http.StatusForbidden || ae.Message != "server quota exceeded" {
		t.Fatalf("unexpected APIError: %+v", ae)
	}
}

func TestNewClientBadURL(t *testing.T) {
	if _, err := NewClient("://bad", "", time.Second); err == nil {
		t.Fatal("expected error for invalid base URL")
	}
}

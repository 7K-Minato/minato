package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controlplane/auth"
	"github.com/7k-minato/minato/internal/controlplane/rbac"
)

// withUser injects an authenticated user into the request context, standing
// in for auth.Middleware in handler tests.
func withUser(user *auth.User, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user != nil {
			r = r.WithContext(auth.WithUser(r.Context(), user))
		}
		next.ServeHTTP(w, r)
	})
}

func TestListGameServers_FiltersByNamespaceScope(t *testing.T) {
	gsA := &operatorv1.GameServer{ObjectMeta: metav1.ObjectMeta{Name: "gs-a", Namespace: "tenant-a"}}
	gsB := &operatorv1.GameServer{ObjectMeta: metav1.ObjectMeta{Name: "gs-b", Namespace: "tenant-b"}}
	api, _ := setupTestAPI(gsA, gsB)
	r := newRouter(api)

	do := func(user *auth.User) []operatorv1.GameServer {
		t.Helper()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers", nil)
		withUser(user, r).ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", rec.Code)
		}
		var items []operatorv1.GameServer
		if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		return items
	}

	scoped := &auth.User{ID: "1", Username: "ci", Role: "viewer", Namespaces: []string{"tenant-*"}}
	items := do(scoped)
	if len(items) != 2 { // both tenant-a and tenant-b match the glob
		t.Fatalf("expected 2 items for glob-scoped user, got %d", len(items))
	}

	scoped = &auth.User{ID: "1", Username: "ci", Role: "viewer", Namespaces: []string{"tenant-a"}}
	items = do(scoped)
	if len(items) != 1 || items[0].Name != "gs-a" {
		t.Fatalf("expected only gs-a, got %v", items)
	}

	items = do(&auth.User{ID: "2", Username: "admin", Role: "admin"})
	if len(items) != 2 {
		t.Fatalf("expected 2 items for cluster-wide user, got %d", len(items))
	}

	items = do(nil)
	if len(items) != 2 {
		t.Fatalf("expected 2 items for anonymous (auth none mode), got %d", len(items))
	}
}

func TestListGameServerFleets_FiltersByNamespaceScope(t *testing.T) {
	fA := &operatorv1.GameServerFleet{ObjectMeta: metav1.ObjectMeta{Name: "f-a", Namespace: "tenant-a"}}
	fB := &operatorv1.GameServerFleet{ObjectMeta: metav1.ObjectMeta{Name: "f-b", Namespace: "other"}}
	api, _ := setupTestAPI(fA, fB)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameserverfleets", nil)
	user := &auth.User{ID: "1", Username: "ci", Role: "viewer", Namespaces: []string{"tenant-*"}}
	withUser(user, r).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var items []operatorv1.GameServerFleet
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 1 || items[0].Name != "f-a" {
		t.Fatalf("expected only f-a, got %v", items)
	}
}

func TestNamespaceGuard_CRUDFlow(t *testing.T) {
	api, c := setupTestAPI()
	handler := withUser(
		&auth.User{ID: "1", Username: "ci", Role: "admin", Namespaces: []string{"tenant-a"}},
		rbac.NamespaceGuard()(newRouter(api)),
	)

	gs := operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-new"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	body, _ := json.Marshal(gs)

	// Create in allowed namespace succeeds.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/tenant-a", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 in allowed namespace, got %d: %s", rec.Code, rec.Body.String())
	}
	var created operatorv1.GameServer
	if err := c.Get(context.Background(), client.ObjectKey{Name: "gs-new", Namespace: "tenant-a"}, &created); err != nil {
		t.Fatalf("expected created GameServer: %v", err)
	}

	// Create in a denied namespace is rejected with 403.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/tenant-b", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 in denied namespace, got %d", rec.Code)
	}

	// Read in a denied namespace is rejected with 403.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/tenant-b/gs-new", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 reading denied namespace, got %d", rec.Code)
	}

	// Delete in the allowed namespace succeeds.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/gameservers/tenant-a/gs-new", nil)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 deleting in allowed namespace, got %d", rec.Code)
	}
}

func TestCreateAPIKey_WithScopeAndExpiry(t *testing.T) {
	api, c := setupTestAPI()
	api.keyStorage = auth.NewAPIKeyStorage(c, "minato")
	handler := withUser(&auth.User{ID: "1", Username: "alice", Role: "admin"}, newRouter(api))

	expires := time.Now().Add(time.Hour).UTC()
	body, _ := json.Marshal(map[string]any{
		"name":       "tenant-ci",
		"role":       "operator",
		"namespaces": []string{"tenant-*"},
		"expiresAt":  expires,
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if resp["key"] == "" {
		t.Fatalf("expected key value in response")
	}
	if resp["expiresAt"] == nil {
		t.Fatalf("expected expiresAt in response: %v", resp)
	}

	var secret corev1.Secret
	if err := c.Get(context.Background(), client.ObjectKey{Name: "minato-apikey-tenant-ci", Namespace: "minato"}, &secret); err != nil {
		t.Fatalf("expected key secret: %v", err)
	}
	if secret.StringData["namespaces"] != "tenant-*" {
		t.Fatalf("expected namespaces in secret, got %v", secret.StringData)
	}
	if secret.StringData["expiresAt"] == "" {
		t.Fatalf("expected expiresAt in secret")
	}
}

func TestCreateAPIKey_PastExpiryRejected(t *testing.T) {
	api, c := setupTestAPI()
	api.keyStorage = auth.NewAPIKeyStorage(c, "minato")
	handler := withUser(&auth.User{ID: "1", Username: "alice", Role: "admin"}, newRouter(api))

	body, _ := json.Marshal(map[string]any{
		"name":      "bad",
		"expiresAt": time.Now().Add(-time.Hour).UTC(),
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/apikeys", bytes.NewReader(body))
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestListAPIKeys_ShowsExpiry(t *testing.T) {
	api, c := setupTestAPI()
	api.keyStorage = auth.NewAPIKeyStorage(c, "minato")
	handler := withUser(&auth.User{ID: "1", Username: "alice", Role: "admin"}, newRouter(api))

	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	meta, _ := json.Marshal(auth.APIKeyEntry{Name: "ci", UserID: "1", Username: "alice", Role: "admin", ExpiresAt: &expires})
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "minato-apikey-ci",
			Namespace: "minato",
			Labels:    map[string]string{"minato.io/apikey": "true"},
		},
		Data: map[string][]byte{"key": []byte("minato_x"), "metadata": meta},
	}
	if err := c.Create(context.Background(), secret); err != nil {
		t.Fatalf("failed to seed key secret: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apikeys", nil)
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var keys []auth.APIKeyEntry
	if err := json.Unmarshal(rec.Body.Bytes(), &keys); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}
	if keys[0].ExpiresAt == nil {
		t.Fatalf("expected expiresAt in listing")
	}
	if keys[0].KeyID != "" {
		t.Fatalf("key material must not be exposed in listing")
	}
}

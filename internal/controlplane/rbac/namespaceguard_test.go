package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/7k-minato/minato/internal/controlplane/auth"
)

func TestNamespaceFromPath(t *testing.T) {
	cases := []struct {
		path, want string
	}{
		{"/api/v1/gameservers", ""},
		{"/api/v1/gameservers/tenant-a", "tenant-a"},
		{"/api/v1/gameservers/tenant-a/gs1", "tenant-a"},
		{"/api/v1/gameservers/tenant-a/gs1/console", "tenant-a"},
		{"/api/v1/gameserverfleets/tenant-b/fleet1", "tenant-b"},
		{"/api/v1/profiles", ""},
		{"/api/v1/profiles/minecraft", ""},
		{"/api/v1/apikeys", ""},
		{"/api/v1/apikeys/abc", ""},
		{"/healthz", ""},
	}
	for _, c := range cases {
		if got := NamespaceFromPath(c.path); got != c.want {
			t.Errorf("NamespaceFromPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestNamespaceGuard(t *testing.T) {
	handler := NamespaceGuard()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	newReq := func(path string, user *auth.User) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if user != nil {
			req = req.WithContext(auth.WithUser(req.Context(), user))
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	clusterWide := &auth.User{ID: "1", Username: "admin", Role: "admin"}
	scoped := &auth.User{ID: "2", Username: "ci", Role: "admin", Namespaces: []string{"tenant-a"}}
	glob := &auth.User{ID: "3", Username: "ci", Role: "admin", Namespaces: []string{"tenant-*"}}

	cases := []struct {
		name string
		path string
		user *auth.User
		want int
	}{
		{"cluster-wide any namespace", "/api/v1/gameservers/tenant-b/gs1", clusterWide, http.StatusOK},
		{"scoped allowed namespace", "/api/v1/gameservers/tenant-a/gs1", scoped, http.StatusOK},
		{"scoped denied namespace", "/api/v1/gameservers/tenant-b/gs1", scoped, http.StatusForbidden},
		{"scoped denied create", "/api/v1/gameservers/tenant-b", scoped, http.StatusForbidden},
		{"glob matches", "/api/v1/gameservers/tenant-x/gs1", glob, http.StatusOK},
		{"glob no match", "/api/v1/gameservers/prod/gs1", glob, http.StatusForbidden},
		{"list path passes through", "/api/v1/gameservers", scoped, http.StatusOK},
		{"profiles pass through", "/api/v1/profiles/minecraft", scoped, http.StatusOK},
		{"apikeys pass through", "/api/v1/apikeys", scoped, http.StatusOK},
		{"anonymous passes through", "/api/v1/gameservers/tenant-b/gs1", nil, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := newReq(c.path, c.user); rec.Code != c.want {
				t.Fatalf("GET %s: expected %d, got %d", c.path, c.want, rec.Code)
			}
		})
	}
}

func TestRouteGuard_ClusterWideRules(t *testing.T) {
	handler := RouteGuard(DefaultRules)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	newReq := func(method, path string, user *auth.User) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		req = req.WithContext(auth.WithUser(req.Context(), user))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	clusterWideAdmin := &auth.User{ID: "1", Username: "admin", Role: "admin"}
	scopedAdmin := &auth.User{ID: "2", Username: "ci", Role: "admin", Namespaces: []string{"tenant-a"}}

	cases := []struct {
		name         string
		method, path string
		user         *auth.User
		want         int
	}{
		{"cluster-wide admin lists apikeys", http.MethodGet, "/api/v1/apikeys", clusterWideAdmin, http.StatusOK},
		{"scoped admin cannot list apikeys", http.MethodGet, "/api/v1/apikeys", scopedAdmin, http.StatusForbidden},
		{"scoped admin cannot create apikey", http.MethodPost, "/api/v1/apikeys", scopedAdmin, http.StatusForbidden},
		{"scoped admin cannot delete apikey", http.MethodDelete, "/api/v1/apikeys/ci", scopedAdmin, http.StatusForbidden},
		{"scoped admin still manages allowed ns", http.MethodPost, "/api/v1/gameservers/tenant-a", scopedAdmin, http.StatusOK},
		{"cluster-wide admin creates gameserver", http.MethodPost, "/api/v1/gameservers/tenant-b", clusterWideAdmin, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := newReq(c.method, c.path, c.user); rec.Code != c.want {
				t.Fatalf("%s %s: expected %d, got %d", c.method, c.path, c.want, rec.Code)
			}
		})
	}
}

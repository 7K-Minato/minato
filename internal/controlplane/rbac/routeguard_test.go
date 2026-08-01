package rbac

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/7k-minato/minato/internal/controlplane/auth"
)

func TestMatchPath(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"/api/v1/gameservers", "/api/v1/gameservers", true},
		{"/api/v1/gameservers/{namespace}", "/api/v1/gameservers/default", true},
		{"/api/v1/gameservers/{namespace}", "/api/v1/gameservers/default/gs1", false},
		{"/api/v1/gameservers/{namespace}/{name}", "/api/v1/gameservers/default/gs1", true},
		{"/api/v1/gameservers/{namespace}/{name}/console", "/api/v1/gameservers/default/gs1/console", true},
		{"/api/v1/gameservers/{namespace}/{name}", "/api/v1/gameservers/default/gs1/console", false},
		{"/api/v1/apikeys/{keyId}", "/api/v1/apikeys/abc", true},
		{"/api/v1/apikeys", "/api/v1/apikeys/abc", false},
	}
	for _, c := range cases {
		if got := MatchPath(c.pattern, c.path); got != c.want {
			t.Errorf("MatchPath(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}

func TestRouteGuard(t *testing.T) {
	handler := RouteGuard(DefaultRules)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	newReq := func(method, path, role string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, nil)
		if role != "" {
			req = req.WithContext(auth.WithUser(req.Context(), &auth.User{ID: "u", Username: "u", Role: role}))
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	cases := []struct {
		name         string
		method, path string
		role         string
		want         int
	}{
		{"viewer reads gameservers", http.MethodGet, "/api/v1/gameservers", "viewer", http.StatusOK},
		{"viewer cannot create gameserver", http.MethodPost, "/api/v1/gameservers/default", "viewer", http.StatusForbidden},
		{"operator cannot create gameserver", http.MethodPost, "/api/v1/gameservers/default", "operator", http.StatusForbidden},
		{"admin creates gameserver", http.MethodPost, "/api/v1/gameservers/default", "admin", http.StatusOK},
		{"admin deletes gameserver", http.MethodDelete, "/api/v1/gameservers/default/gs1", "admin", http.StatusOK},
		{"viewer cannot delete gameserver", http.MethodDelete, "/api/v1/gameservers/default/gs1", "viewer", http.StatusForbidden},
		{"operator executes action", http.MethodPost, "/api/v1/gameservers/default/gs1/actions/restart", "operator", http.StatusOK},
		{"viewer cannot execute action", http.MethodPost, "/api/v1/gameservers/default/gs1/actions/restart", "viewer", http.StatusForbidden},
		{"operator creates snapshot", http.MethodPost, "/api/v1/gameservers/default/gs1/snapshots", "operator", http.StatusOK},
		{"viewer opens console forbidden", http.MethodGet, "/api/v1/gameservers/default/gs1/console", "viewer", http.StatusForbidden},
		{"operator opens console", http.MethodGet, "/api/v1/gameservers/default/gs1/console", "operator", http.StatusOK},
		{"operator cannot list apikeys", http.MethodGet, "/api/v1/apikeys", "operator", http.StatusForbidden},
		{"admin lists apikeys", http.MethodGet, "/api/v1/apikeys", "admin", http.StatusOK},
		{"admin deletes apikey", http.MethodDelete, "/api/v1/apikeys/abc", "admin", http.StatusOK},
		{"unauthenticated rejected on guarded route", http.MethodGet, "/api/v1/apikeys", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if rec := newReq(c.method, c.path, c.role); rec.Code != c.want {
				t.Fatalf("%s %s as %q: expected %d, got %d", c.method, c.path, c.role, c.want, rec.Code)
			}
		})
	}
}

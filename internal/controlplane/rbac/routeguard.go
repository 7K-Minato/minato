package rbac

import (
	"net/http"
	"slices"
	"strings"

	"github.com/7k-minato/minato/internal/controlplane/auth"
)

// RouteRule binds an HTTP method and path template to the roles allowed to access it.
// Path templates use chi-style parameters, e.g. /api/v1/gameservers/{namespace}.
// Method "*" matches any method.
type RouteRule struct {
	Method  string
	Pattern string
	Roles   []string
}

// DefaultRules is the access policy for the control plane API.
// The first matching rule wins; unmatched routes only require authentication.
var DefaultRules = []RouteRule{
	{Method: http.MethodPost, Pattern: "/api/v1/gameservers/{namespace}", Roles: []string{"admin"}},
	{Method: http.MethodDelete, Pattern: "/api/v1/gameservers/{namespace}/{name}", Roles: []string{"admin"}},
	{Method: http.MethodGet, Pattern: "/api/v1/gameservers/{namespace}/{name}/console", Roles: []string{"operator", "admin"}},
	{Method: http.MethodPost, Pattern: "/api/v1/gameservers/{namespace}/{name}/actions/{action}", Roles: []string{"operator", "admin"}},
	{Method: http.MethodPost, Pattern: "/api/v1/gameservers/{namespace}/{name}/snapshots", Roles: []string{"operator", "admin"}},
	{Method: "*", Pattern: "/api/v1/apikeys", Roles: []string{"admin"}},
	{Method: "*", Pattern: "/api/v1/apikeys/{keyId}", Roles: []string{"admin"}},
}

// RouteGuard returns middleware enforcing route-level role requirements.
// It must run after auth.Middleware so the user is present in the context.
func RouteGuard(rules []RouteRule) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for _, rule := range rules {
				if rule.Method != "*" && rule.Method != r.Method {
					continue
				}
				if !MatchPath(rule.Pattern, r.URL.Path) {
					continue
				}
				user := auth.GetUser(r.Context())
				if user == nil {
					http.Error(w, "Unauthorized", http.StatusUnauthorized)
					return
				}
				if !slices.Contains(rule.Roles, user.Role) {
					http.Error(w, "Forbidden", http.StatusForbidden)
					return
				}
				break
			}
			next.ServeHTTP(w, r)
		})
	}
}

// MatchPath reports whether path matches a chi-style template like
// /api/v1/gameservers/{namespace}/{name}. Templated segments match any single
// non-empty segment.
func MatchPath(pattern, path string) bool {
	pSegs := strings.Split(strings.Trim(pattern, "/"), "/")
	sSegs := strings.Split(strings.Trim(path, "/"), "/")
	if len(pSegs) != len(sSegs) {
		return false
	}
	for i := range pSegs {
		if strings.HasPrefix(pSegs[i], "{") && strings.HasSuffix(pSegs[i], "}") {
			if sSegs[i] == "" {
				return false
			}
			continue
		}
		if pSegs[i] != sSegs[i] {
			return false
		}
	}
	return true
}

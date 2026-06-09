// Package rbac provides role-based access control for the minato control plane.
package rbac

import (
	"net/http"
	"slices"

	"github.com/7k-group/minato/internal/controlplane/auth"
)

// Role defines permissions for a role.
type Role struct {
	Name        string       `json:"name"`
	Permissions []Permission `json:"permissions"`
	Extends     string       `json:"extends,omitempty"`
}

// Permission defines a single permission.
type Permission struct {
	Resource   string   `json:"resource"`
	Verbs      []string `json:"verbs"`
	Namespaces []string `json:"namespaces,omitempty"`
}

// Predefined roles.
var (
	ViewerRole = &Role{
		Name: "viewer",
		Permissions: []Permission{
			{Resource: "gameservers", Verbs: []string{"get", "list"}},
			{Resource: "gameserverfleets", Verbs: []string{"get", "list"}},
			{Resource: "profiles", Verbs: []string{"get", "list"}},
			{Resource: "snapshots", Verbs: []string{"get", "list"}},
			{Resource: "actions", Verbs: []string{"get", "list"}},
			{Resource: "executions", Verbs: []string{"get", "list"}},
		},
	}

	OperatorRole = &Role{
		Name:    "operator",
		Extends: "viewer",
		Permissions: []Permission{
			{Resource: "actions", Verbs: []string{"execute"}},
			{Resource: "snapshots", Verbs: []string{"create"}},
		},
	}

	AdminRole = &Role{
		Name:    "admin",
		Extends: "operator",
		Permissions: []Permission{
			{Resource: "gameservers", Verbs: []string{"create", "delete"}},
			{Resource: "gameserverfleets", Verbs: []string{"create", "delete"}},
			{Resource: "apikeys", Verbs: []string{"create", "delete", "list"}},
		},
	}
)

// roleMap holds all known roles.
var roleMap = map[string]*Role{
	"viewer":   ViewerRole,
	"operator": OperatorRole,
	"admin":    AdminRole,
}

// RegisterRole adds a custom role.
func RegisterRole(role *Role) {
	roleMap[role.Name] = role
}

// GetRole returns a role by name.
func GetRole(name string) *Role {
	return roleMap[name]
}

// HasPermission checks if a role has permission for a resource and verb.
func HasPermission(roleName, resource, verb string) bool {
	role := GetRole(roleName)
	if role == nil {
		return false
	}

	// Check inherited permissions
	if role.Extends != "" {
		if HasPermission(role.Extends, resource, verb) {
			return true
		}
	}

	// Check direct permissions
	for _, perm := range role.Permissions {
		if perm.Resource == resource || perm.Resource == "*" {
			for _, v := range perm.Verbs {
				if v == verb || v == "*" {
					return true
				}
			}
		}
	}

	return false
}

// RequireRole returns middleware that requires at least one of the given roles.
func RequireRole(roles ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := auth.GetUser(r.Context())
			if user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			if slices.Contains(roles, user.Role) {
				next.ServeHTTP(w, r)
				return
			}

			http.Error(w, "Forbidden", http.StatusForbidden)
		})
	}
}

// ResourceFromPath maps an HTTP path to a resource name.
func ResourceFromPath(path string) string {
	// Simple mapping - can be expanded
	switch {
	case contains(path, "/gameservers/") && contains(path, "/actions"):
		return "actions"
	case contains(path, "/gameservers/") && contains(path, "/snapshots"):
		return "snapshots"
	case contains(path, "/gameservers/") && contains(path, "/console"):
		return "console"
	case contains(path, "/gameservers"):
		return "gameservers"
	case contains(path, "/gameserverfleets"):
		return "gameserverfleets"
	case contains(path, "/profiles"):
		return "profiles"
	case contains(path, "/apikeys"):
		return "apikeys"
	default:
		return ""
	}
}

// VerbFromMethod maps an HTTP method to a verb.
func VerbFromMethod(method string) string {
	switch method {
	case "GET":
		return "get"
	case "POST":
		return "create"
	case "PUT", "PATCH":
		return "update"
	case "DELETE":
		return "delete"
	default:
		return ""
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

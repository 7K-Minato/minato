// Package audit provides structured audit logging for the minato control plane.
package audit

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/7k-minato/minato/internal/controlplane/auth"
)

// Event represents an audit event.
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	User      string    `json:"user"`
	Role      string    `json:"role"`
	Action    string    `json:"action"`
	Resource  string    `json:"resource"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	ClientIP  string    `json:"clientIP"`
	UserAgent string    `json:"userAgent"`
	Result    string    `json:"result"`
	Error     string    `json:"error,omitempty"`
}

// Middleware returns middleware that logs all requests as audit events.
func Middleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip health endpoints
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" {
				next.ServeHTTP(w, r)
				return
			}

			// Create a response wrapper to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			start := time.Now()
			next.ServeHTTP(wrapped, r)
			duration := time.Since(start)

			// Build audit event
			user := auth.GetUser(r.Context())
			username := "anonymous"
			role := ""
			if user != nil {
				username = user.Username
				role = user.Role
			}

			event := Event{
				Timestamp: time.Now(),
				Level:     "audit",
				User:      username,
				Role:      role,
				Method:    r.Method,
				Path:      r.URL.Path,
				ClientIP:  r.RemoteAddr,
				UserAgent: r.UserAgent(),
				Result:    "success",
			}

			if wrapped.statusCode >= 400 {
				event.Result = "failure"
			}

			// Log the event (JSON to stdout)
			logAudit(event)

			_ = duration // Could be used for performance metrics
		})
	}
}

// logAudit outputs an audit event as JSON.
func logAudit(event Event) {
	data, _ := json.Marshal(event)
	// Use standard logger or custom output
	// For now, just print to stdout
	println(string(data))
}

// responseWriter wraps http.ResponseWriter to capture status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

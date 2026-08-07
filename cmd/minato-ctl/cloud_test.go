package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// recordedRequest captures one request seen by the fake cloud API.
type recordedRequest struct {
	Method string
	Path   string
	Auth   string
	Body   []byte
}

// newFakeCloud starts an httptest server that records requests before
// delegating to handler.
func newFakeCloud(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var reqs []recordedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil {
			body, _ = io.ReadAll(r.Body)
		}
		reqs = append(reqs, recordedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Auth:   r.Header.Get("Authorization"),
			Body:   body,
		})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, &reqs
}

// useCloudEnv isolates cloud CLI state: a temp config file, reset globals and
// blanked cloud env vars. If cfg is non-nil it is written to the config file.
func useCloudEnv(t *testing.T, cfg *cloudConfig) {
	t.Helper()

	oldPath, oldURL, oldTenant, oldJSON := cloudConfigPath, cloudURL, cloudTenant, cloudJSON
	cloudConfigPath = filepath.Join(t.TempDir(), "config.json")
	cloudURL, cloudTenant, cloudJSON = "", "", false
	t.Setenv("MINATO_CLOUD_URL", "")
	t.Setenv("MINATO_CLOUD_API_KEY", "")
	t.Cleanup(func() {
		cloudConfigPath, cloudURL, cloudTenant, cloudJSON = oldPath, oldURL, oldTenant, oldJSON
	})

	if cfg != nil {
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatalf("marshal config: %v", err)
		}
		if err := os.WriteFile(cloudConfigPath, data, 0o600); err != nil {
			t.Fatalf("write config: %v", err)
		}
	}
}

// cloudConfigWithKey returns a config holding an API key and no URL override.
func cloudConfigWithKey(key string) *cloudConfig {
	cfg := &cloudConfig{}
	cfg.Cloud.APIKey = key
	return cfg
}

// runCloud executes `minato-ctl cloud <args...>` against the current env.
func runCloud(t *testing.T, args ...string) error {
	t.Helper()
	cmd := cloudCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
}

// tenantsHandler serves GET /api/v1/me/tenants with a single tenant.
func singleTenantHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]map[string]any{
		{"id": "t-1", "slug": "acme", "name": "Acme"},
	})
}

// fakeCloudAPI is a fake cloud API covering the routes used by the command
// groups. tenantJSON is returned by GET /api/v1/me/tenants.
type fakeCloudAPI struct {
	t *testing.T
}

func (f *fakeCloudAPI) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/me/tenants":
		singleTenantHandler(w, r)
	case r.Method == http.MethodGet && r.URL.Path == "/api/v1/tenants/t-1/servers":
		_ = json.NewEncoder(w).Encode([]map[string]any{
			{"id": "srv-1", "tenant_id": "t-1", "cluster_id": "c-1", "namespace": "ns",
				"name": "lobby", "profile": "minecraft", "status": "Running"},
		})
	default:
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "no such route"})
	}
}

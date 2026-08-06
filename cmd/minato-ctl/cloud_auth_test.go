package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/7k-minato/minato/internal/cloudapi"
)

func TestCloudURLOrTokenPrecedence(t *testing.T) {
	useCloudEnv(t, nil)
	cfg := &cloudConfig{}
	cfg.Cloud.URL = "http://from-config"
	cfg.Cloud.APIKey = "mk_config"
	cfg.Cloud.SessionToken = "sess"

	if got := resolveCloudURL(cfg); got != "http://from-config" {
		t.Fatalf("config URL: got %q", got)
	}
	t.Setenv("MINATO_CLOUD_URL", "http://from-env")
	if got := resolveCloudURL(cfg); got != "http://from-env" {
		t.Fatalf("env URL: got %q", got)
	}
	cloudURL = "http://from-flag"
	if got := resolveCloudURL(cfg); got != "http://from-flag" {
		t.Fatalf("flag URL: got %q", got)
	}
	cloudURL = ""

	t.Setenv("MINATO_CLOUD_API_KEY", "mk_env")
	if tok, mode := resolveCloudToken(cfg); tok != "mk_env" || mode != "api-key (env)" {
		t.Fatalf("env token: got %q/%q", tok, mode)
	}
	t.Setenv("MINATO_CLOUD_API_KEY", "")
	if tok, mode := resolveCloudToken(cfg); tok != "mk_config" || mode != "api-key" {
		t.Fatalf("config api key: got %q/%q", tok, mode)
	}
	cfg.Cloud.APIKey = ""
	if tok, mode := resolveCloudToken(cfg); tok != "sess" || mode != "session" {
		t.Fatalf("session token: got %q/%q", tok, mode)
	}

	useDefault := &cloudConfig{}
	t.Setenv("MINATO_CLOUD_URL", "")
	if got := resolveCloudURL(useDefault); got != defaultCloudURL {
		t.Fatalf("default URL: got %q", got)
	}
}

func TestCloudLoginAPIKey(t *testing.T) {
	useCloudEnv(t, nil)
	fake := &fakeCloudAPI{t: t}
	srv, reqs := newFakeCloud(t, fake.serveHTTP)

	if err := runCloud(t, "login", "--url", srv.URL, "--api-key", "mk_abc123_secret"); err != nil {
		t.Fatalf("login: %v", err)
	}

	info, err := os.Stat(cloudConfigPath)
	if err != nil {
		t.Fatalf("stat config: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config mode: got %o, want 600", info.Mode().Perm())
	}
	cfg, err := loadCloudConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Cloud.APIKey != "mk_abc123_secret" || cfg.Cloud.URL != srv.URL {
		t.Fatalf("stored config: %+v", cfg.Cloud)
	}
	if len(*reqs) != 1 || (*reqs)[0].Path != "/api/v1/me/tenants" {
		t.Fatalf("expected verification call to me/tenants: %+v", *reqs)
	}
	if (*reqs)[0].Auth != "Bearer mk_abc123_secret" {
		t.Fatalf("auth header: %q", (*reqs)[0].Auth)
	}
}

func TestCloudLoginSessionToken(t *testing.T) {
	useCloudEnv(t, nil)
	fake := &fakeCloudAPI{t: t}
	srv, _ := newFakeCloud(t, fake.serveHTTP)

	old := cloudStdin
	cloudStdin = strings.NewReader("keycloak-id-token\n")
	t.Cleanup(func() { cloudStdin = old })

	if err := runCloud(t, "login", "--url", srv.URL); err != nil {
		t.Fatalf("login: %v", err)
	}
	cfg, err := loadCloudConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Cloud.SessionToken != "keycloak-id-token" || cfg.Cloud.APIKey != "" {
		t.Fatalf("stored config: %+v", cfg.Cloud)
	}
}

func TestCloudLoginRejectsBadAPIKey(t *testing.T) {
	useCloudEnv(t, nil)
	if err := runCloud(t, "login", "--api-key", "not-a-key"); err == nil || !strings.Contains(err.Error(), "mk_") {
		t.Fatalf("expected mk_ prefix error, got %v", err)
	}
}

func TestCloudLogout(t *testing.T) {
	cfg := cloudConfigWithKey("mk_x")
	cfg.Cloud.SessionToken = "sess"
	useCloudEnv(t, cfg)

	if err := runCloud(t, "logout"); err != nil {
		t.Fatalf("logout: %v", err)
	}
	after, err := loadCloudConfig()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if after.Cloud.APIKey != "" || after.Cloud.SessionToken != "" {
		t.Fatalf("credentials not cleared: %+v", after.Cloud)
	}
}

func TestCloudWhoami(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_who"))
	fake := &fakeCloudAPI{t: t}
	srv, reqs := newFakeCloud(t, fake.serveHTTP)

	if err := runCloud(t, "whoami", "--url", srv.URL); err != nil {
		t.Fatalf("whoami: %v", err)
	}
	if len(*reqs) != 1 || (*reqs)[0].Path != "/api/v1/me/tenants" {
		t.Fatalf("expected me/tenants call: %+v", *reqs)
	}
	if (*reqs)[0].Auth != "Bearer mk_who" {
		t.Fatalf("auth header: %q", (*reqs)[0].Auth)
	}
}

func TestCloudErrMapping(t *testing.T) {
	cases := []struct {
		status int
		want   string
	}{
		{401, "cloud login"},
		{402, "subscribe"},
		{403, "role"},
		{404, "not found"},
	}
	for _, tc := range cases {
		err := cloudErr(&cloudapi.APIError{Status: tc.status, Message: "boom"})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("status %d: got %v, want substring %q", tc.status, err, tc.want)
		}
	}
	if err := cloudErr(errors.New("plain")); err == nil || err.Error() != "plain" {
		t.Fatalf("non-API error should pass through, got %v", err)
	}
}

func TestCloudErrFromServer(t *testing.T) {
	useCloudEnv(t, cloudConfigWithKey("mk_bad"))
	srv, _ := newFakeCloud(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid token"})
	})

	err := runCloud(t, "whoami", "--url", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "minato-ctl cloud login") || !strings.Contains(err.Error(), "invalid token") {
		t.Fatalf("expected 401 mapping with server message, got %v", err)
	}
}

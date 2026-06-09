// Package auth provides authentication middleware and providers for the minato control plane.
package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrUnauthorized is returned when authentication fails.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrInvalidCredentials is returned when credentials are malformed.
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// User represents an authenticated user.
type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email,omitempty"`
	Role     string `json:"role"`
	Source   string `json:"source"` // basic, oidc, apikey
}

// IsAnonymous returns true if the user is not authenticated.
func (u *User) IsAnonymous() bool {
	return u == nil || u.ID == ""
}

type contextKey struct{}

var userContextKey = &contextKey{}

// WithUser attaches a User to the context.
func WithUser(ctx context.Context, user *User) context.Context {
	return context.WithValue(ctx, userContextKey, user)
}

// GetUser retrieves the User from the context.
func GetUser(ctx context.Context) *User {
	if u, ok := ctx.Value(userContextKey).(*User); ok {
		return u
	}
	return nil
}

// Provider is the interface for authentication mechanisms.
type Provider interface {
	// Authenticate validates the request and returns a User.
	Authenticate(r *http.Request) (*User, error)
	// Name returns the provider identifier.
	Name() string
}

// Chain tries multiple providers in order.
type Chain struct {
	providers []Provider
}

// NewChain creates an empty auth chain.
func NewChain() *Chain {
	return &Chain{}
}

// Add appends a provider to the chain.
func (c *Chain) Add(p Provider) {
	c.providers = append(c.providers, p)
}

// Authenticate tries each provider until one succeeds.
func (c *Chain) Authenticate(r *http.Request) (*User, error) {
	for _, p := range c.providers {
		if user, err := p.Authenticate(r); err == nil {
			return user, nil
		}
	}
	return nil, ErrUnauthorized
}

// Middleware returns chi middleware that authenticates requests.
// Health endpoints (/healthz, /readyz) are always public.
func Middleware(chain *Chain) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Always allow public endpoints
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || r.URL.Path == "/auth/config" {
				next.ServeHTTP(w, r)
				return
			}

			user, err := chain.Authenticate(r)
			if err != nil {
				w.Header().Set("WWW-Authenticate", "Bearer, Basic")
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			ctx := WithUser(r.Context(), user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ExtractBearer extracts a Bearer token from the Authorization header.
func ExtractBearer(r *http.Request) string {
	header := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if strings.HasPrefix(header, prefix) {
		return strings.TrimSpace(header[len(prefix):])
	}
	return ""
}

// HashPassword hashes a plaintext password using bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword compares a plaintext password with a bcrypt hash.
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ConstantTimeCompare compares two strings in constant time.
func ConstantTimeCompare(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Config holds authentication configuration.
type Config struct {
	Mode   string       `json:"mode"` // none, basic, oidc, apikey, or comma-separated list
	Basic  BasicConfig  `json:"basic"`
	OIDC   OIDCConfig   `json:"oidc"`
	APIKey APIKeyConfig `json:"apikey"`
}

// BasicConfig holds basic auth configuration.
type BasicConfig struct {
	Enabled  bool   `json:"enabled"`
	User     string `json:"user"`
	Password string `json:"password"` // plaintext or bcrypt hash
	Role     string `json:"role"`
}

// OIDCConfig holds OIDC configuration.
type OIDCConfig struct {
	Enabled   bool   `json:"enabled"`
	IssuerURL string `json:"issuerUrl"`
	ClientID  string `json:"clientId"`
	RoleClaim string `json:"roleClaim"` // e.g., "groups"
}

// APIKeyConfig holds API key configuration.
type APIKeyConfig struct {
	Enabled bool `json:"enabled"`
}

// LoadConfig loads auth configuration from environment variables.
func LoadConfig() *Config {
	cfg := &Config{
		Mode: getEnv("AUTH_MODE", "none"),
		Basic: BasicConfig{
			Enabled:  getEnvBool("AUTH_BASIC_ENABLED", false),
			User:     getEnv("AUTH_BASIC_USER", ""),
			Password: getEnv("AUTH_BASIC_PASSWORD", ""),
			Role:     getEnv("AUTH_BASIC_ROLE", "admin"),
		},
		OIDC: OIDCConfig{
			Enabled:   getEnvBool("AUTH_OIDC_ENABLED", false),
			IssuerURL: getEnv("AUTH_OIDC_ISSUER_URL", ""),
			ClientID:  getEnv("AUTH_OIDC_CLIENT_ID", ""),
			RoleClaim: getEnv("AUTH_OIDC_ROLE_CLAIM", "groups"),
		},
		APIKey: APIKeyConfig{
			Enabled: getEnvBool("AUTH_APIKEY_ENABLED", false),
		},
	}

	return cfg
}

// BuildChain creates an auth chain from configuration.
func BuildChain(cfg *Config) (*Chain, error) {
	return BuildChainWithStorage(cfg, nil)
}

// BuildChainWithStorage creates an auth chain with optional API key storage.
func BuildChainWithStorage(cfg *Config, keyStorage *APIKeyStorage) (*Chain, error) {
	chain := NewChain()

	modes := strings.SplitSeq(cfg.Mode, ",")
	for mode := range modes {
		mode = strings.TrimSpace(strings.ToLower(mode))
		switch mode {
		case "none":
			chain.Add(NewNoneProvider())
		case "basic":
			if cfg.Basic.Enabled {
				p, err := NewBasicProvider(cfg.Basic)
				if err != nil {
					return nil, fmt.Errorf("basic auth: %w", err)
				}
				chain.Add(p)
			}
		case "oidc":
			if cfg.OIDC.Enabled {
				p, err := NewOIDCProvider(cfg.OIDC)
				if err != nil {
					return nil, fmt.Errorf("oidc auth: %w", err)
				}
				chain.Add(p)
			}
		case "apikey":
			if cfg.APIKey.Enabled && keyStorage != nil {
				chain.Add(NewAPIKeyProvider(keyStorage))
			}
		}
	}

	// If no providers configured, default to none
	if len(chain.providers) == 0 {
		chain.Add(NewNoneProvider())
	}

	return chain, nil
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if value := os.Getenv(key); value != "" {
		return strings.ToLower(value) == "true" || value == "1"
	}
	return fallback
}

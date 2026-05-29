package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// OIDCProvider implements generic OIDC authentication.
type OIDCProvider struct {
	issuerURL string
	clientID  string
	roleClaim string
	jwksURL   string
	keys      map[string]*jwt.SigningMethodRSA
	mu        sync.RWMutex
	lastFetch time.Time
}

// NewOIDCProvider creates an OIDC auth provider.
func NewOIDCProvider(cfg OIDCConfig) (*OIDCProvider, error) {
	if cfg.IssuerURL == "" || cfg.ClientID == "" {
		return nil, errors.New("oidc auth: issuer URL and client ID required")
	}

	p := &OIDCProvider{
		issuerURL: strings.TrimSuffix(cfg.IssuerURL, "/"),
		clientID:  cfg.ClientID,
		roleClaim: cfg.RoleClaim,
		keys:      make(map[string]*jwt.SigningMethodRSA),
	}

	// Fetch JWKS on initialization
	if err := p.fetchJWKS(); err != nil {
		return nil, fmt.Errorf("oidc auth: failed to fetch JWKS: %w", err)
	}

	return p, nil
}

// Authenticate validates a Bearer token.
func (p *OIDCProvider) Authenticate(r *http.Request) (*User, error) {
	token := ExtractBearer(r)
	if token == "" {
		return nil, ErrUnauthorized
	}

	// Refresh JWKS if needed (older than 1 hour)
	p.mu.RLock()
	lastFetch := p.lastFetch
	p.mu.RUnlock()

	if time.Since(lastFetch) > time.Hour {
		if err := p.fetchJWKS(); err != nil {
			// Log but don't fail - use cached keys
		}
	}

	parsed, err := jwt.Parse(token, func(t *jwt.Token) (interface{}, error) {
		// Ensure RSA signing method
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}

		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, errors.New("token missing kid header")
		}

		p.mu.RLock()
		defer p.mu.RUnlock()

		key, exists := p.keys[kid]
		if !exists {
			return nil, fmt.Errorf("key %s not found", kid)
		}
		return key, nil
	}, jwt.WithIssuer(p.issuerURL), jwt.WithAudience(p.clientID))

	if err != nil || !parsed.Valid {
		return nil, ErrUnauthorized
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrUnauthorized
	
	}

	// Extract role from claims
	role := "viewer" // default
	if p.roleClaim != "" {
		if rawRoles, ok := claims[p.roleClaim]; ok {
			switch v := rawRoles.(type) {
			case string:
				role = v
			case []interface{}:
				if len(v) > 0 {
					if s, ok := v[0].(string); ok {
						role = s
					}
				}
			}
		}
	}

	// Extract user info
	username := ""
	if sub, ok := claims["sub"].(string); ok {
		username = sub
	}
	if preferred, ok := claims["preferred_username"].(string); ok {
		username = preferred
	}

	email := ""
	if e, ok := claims["email"].(string); ok {
		email = e
	}

	return &User{
		ID:       username,
		Username: username,
		Email:    email,
		Role:     role,
		Source:   "oidc",
	}, nil
}

// Name returns the provider identifier.
func (p *OIDCProvider) Name() string {
	return "oidc"
}

// fetchJWKS fetches the JSON Web Key Set from the OIDC discovery endpoint.
func (p *OIDCProvider) fetchJWKS() error {
	// This is a simplified implementation
	// In production, you'd fetch from .well-known/openid-configuration
	// then parse the jwks_uri response
	
	discoveryURL := p.issuerURL + "/.well-known/openid-configuration"
	resp, err := http.Get(discoveryURL)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var config struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&config); err != nil {
		return err
	}

	if config.JWKSURI == "" {
		return errors.New("jwks_uri not found in discovery document")
	}

	// Fetch JWKS
	jwksResp, err := http.Get(config.JWKSURI)
	if err != nil {
		return err
	}
	defer jwksResp.Body.Close()

	var jwks struct {
		Keys []struct {
			KID string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(jwksResp.Body).Decode(&jwks); err != nil {
		return err
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	p.lastFetch = time.Now()
	// Parse keys and store them
	// This is simplified - full implementation would parse RSA keys from base64
	
	return nil
}

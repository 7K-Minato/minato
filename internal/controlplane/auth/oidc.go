package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

// OIDCProvider implements generic OIDC authentication using the provider's
// remote key set (via discovery) for signature verification.
type OIDCProvider struct {
	verifier  *oidc.IDTokenVerifier
	roleClaim string
}

// oidcClaims holds the standard claims we extract from a verified ID token.
type oidcClaims struct {
	Sub               string `json:"sub"`
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

// NewOIDCProvider creates an OIDC auth provider. It performs OIDC discovery
// against the issuer at startup; a failure here is a configuration error and
// prevents the control plane from starting.
func NewOIDCProvider(cfg OIDCConfig) (*OIDCProvider, error) {
	return NewOIDCProviderWithContext(context.Background(), cfg)
}

// NewOIDCProviderWithContext creates an OIDC auth provider with a caller-supplied
// context for the initial discovery request.
func NewOIDCProviderWithContext(ctx context.Context, cfg OIDCConfig) (*OIDCProvider, error) {
	if cfg.IssuerURL == "" || cfg.ClientID == "" {
		return nil, errors.New("oidc auth: issuer URL and client ID required")
	}

	issuer := strings.TrimSuffix(cfg.IssuerURL, "/")
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc auth: discovery failed for issuer %q: %w", issuer, err)
	}

	return &OIDCProvider{
		verifier: provider.Verifier(&oidc.Config{
			ClientID: cfg.ClientID,
		}),
		roleClaim: cfg.RoleClaim,
	}, nil
}

// Authenticate validates a Bearer token. The verifier checks the signature
// (via the provider's remote key set, with automatic key rotation), issuer,
// audience (client ID), and expiry. Any failure yields 401.
func (p *OIDCProvider) Authenticate(r *http.Request) (*User, error) {
	raw := ExtractBearer(r)
	if raw == "" {
		return nil, ErrUnauthorized
	}

	idToken, err := p.verifier.Verify(r.Context(), raw)
	if err != nil {
		return nil, ErrUnauthorized
	}

	var claims oidcClaims
	if err := idToken.Claims(&claims); err != nil {
		return nil, ErrUnauthorized
	}

	// Extract role from the configured claim (dynamic name, so re-decode into a map).
	role := "viewer" // default
	if p.roleClaim != "" {
		var all map[string]any
		if err := idToken.Claims(&all); err == nil {
			if rawRoles, ok := all[p.roleClaim]; ok {
				switch v := rawRoles.(type) {
				case string:
					role = v
				case []any:
					if len(v) > 0 {
						if s, ok := v[0].(string); ok {
							role = s
						}
					}
				}
			}
		}
	}

	username := claims.Sub
	if claims.PreferredUsername != "" {
		username = claims.PreferredUsername
	} else if claims.Name != "" {
		username = claims.Name
	}

	return &User{
		ID:       claims.Sub,
		Username: username,
		Email:    claims.Email,
		Role:     role,
		Source:   "oidc",
	}, nil
}

// Name returns the provider identifier.
func (p *OIDCProvider) Name() string {
	return "oidc"
}

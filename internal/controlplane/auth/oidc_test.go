package auth

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testIssuer is an httptest-backed fake OIDC provider: it serves the discovery
// document and a JWKS containing a locally generated RSA public key.
type testIssuer struct {
	server *httptest.Server
	key    *rsa.PrivateKey
	kid    string
}

func newTestIssuer(t *testing.T) *testIssuer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	ti := &testIssuer{key: key, kid: "test-key-1"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":   ti.server.URL,
			"jwks_uri": ti.server.URL + "/keys",
		})
	})
	mux.HandleFunc("/keys", func(w http.ResponseWriter, r *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes())
		e := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}) // 65537
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA",
				"use": "sig",
				"alg": "RS256",
				"kid": ti.kid,
				"n":   n,
				"e":   e,
			}},
		})
	})

	ti.server = httptest.NewServer(mux)
	t.Cleanup(ti.server.Close)
	return ti
}

// sign issues a signed token with the given claims overrides.
func (ti *testIssuer) sign(t *testing.T, mutate func(jwt.MapClaims)) string {
	t.Helper()

	claims := jwt.MapClaims{
		"iss":                ti.server.URL,
		"sub":                "user-123",
		"aud":                "minato-controlplane",
		"exp":                time.Now().Add(time.Hour).Unix(),
		"iat":                time.Now().Unix(),
		"email":              "alice@example.com",
		"name":               "Alice Example",
		"preferred_username": "alice",
		"groups":             []string{"operator"},
	}
	if mutate != nil {
		mutate(claims)
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = ti.kid
	signed, err := token.SignedString(ti.key)
	require.NoError(t, err)
	return signed
}

func TestOIDCProvider(t *testing.T) {
	ti := newTestIssuer(t)

	cfg := OIDCConfig{
		Enabled:   true,
		IssuerURL: ti.server.URL,
		ClientID:  "minato-controlplane",
		RoleClaim: "groups",
	}
	p, err := NewOIDCProvider(cfg)
	require.NoError(t, err)
	assert.Equal(t, "oidc", p.Name())

	bearerReq := func(token string) *http.Request {
		req, _ := http.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	tests := []struct {
		name      string
		token     func() string
		wantErr   bool
		checkUser func(t *testing.T, u *User)
	}{
		{
			name:  "valid token",
			token: func() string { return ti.sign(t, nil) },
			checkUser: func(t *testing.T, u *User) {
				assert.Equal(t, "user-123", u.ID)
				assert.Equal(t, "alice", u.Username)
				assert.Equal(t, "alice@example.com", u.Email)
				assert.Equal(t, "operator", u.Role)
				assert.Equal(t, "oidc", u.Source)
			},
		},
		{
			name: "valid token without preferred_username falls back to name",
			token: func() string {
				return ti.sign(t, func(c jwt.MapClaims) { delete(c, "preferred_username") })
			},
			checkUser: func(t *testing.T, u *User) {
				assert.Equal(t, "Alice Example", u.Username)
			},
		},
		{
			name: "expired token",
			token: func() string {
				return ti.sign(t, func(c jwt.MapClaims) {
					c["exp"] = time.Now().Add(-time.Hour).Unix()
				})
			},
			wantErr: true,
		},
		{
			name: "wrong audience",
			token: func() string {
				return ti.sign(t, func(c jwt.MapClaims) { c["aud"] = "some-other-client" })
			},
			wantErr: true,
		},
		{
			name: "wrong issuer",
			token: func() string {
				return ti.sign(t, func(c jwt.MapClaims) { c["iss"] = "https://evil.example.com" })
			},
			wantErr: true,
		},
		{
			name:    "garbage token",
			token:   func() string { return "not-a-jwt" },
			wantErr: true,
		},
		{
			name: "token signed by unknown key",
			token: func() string {
				other, err := rsa.GenerateKey(rand.Reader, 2048)
				require.NoError(t, err)
				tok := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
					"iss": ti.server.URL,
					"sub": "mallory",
					"aud": "minato-controlplane",
					"exp": time.Now().Add(time.Hour).Unix(),
				})
				tok.Header["kid"] = ti.kid
				signed, err := tok.SignedString(other)
				require.NoError(t, err)
				return signed
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user, err := p.Authenticate(bearerReq(tt.token()))
			if tt.wantErr {
				assert.ErrorIs(t, err, ErrUnauthorized)
				return
			}
			require.NoError(t, err)
			tt.checkUser(t, user)
		})
	}

	// Missing bearer token
	empty, _ := http.NewRequest("GET", "/", nil)
	_, err = p.Authenticate(empty)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestNewOIDCProvider_ConfigErrors(t *testing.T) {
	// Missing issuer/client ID
	_, err := NewOIDCProvider(OIDCConfig{})
	assert.ErrorContains(t, err, "issuer URL and client ID required")

	_, err = NewOIDCProvider(OIDCConfig{IssuerURL: "https://issuer.example.com"})
	assert.ErrorContains(t, err, "issuer URL and client ID required")

	// Unreachable issuer: discovery failure must surface as an init error.
	_, err = NewOIDCProvider(OIDCConfig{
		IssuerURL: fmt.Sprintf("http://127.0.0.1:%d", 1), // nothing listening
		ClientID:  "minato",
	})
	assert.ErrorContains(t, err, "discovery failed")
}

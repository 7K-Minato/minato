package auth

import "net/http"

// NoneProvider allows all requests (development mode).
type NoneProvider struct{}

// NewNoneProvider creates a no-op auth provider.
func NewNoneProvider() *NoneProvider {
	return &NoneProvider{}
}

// Authenticate returns an anonymous user.
func (p *NoneProvider) Authenticate(r *http.Request) (*User, error) {
	return &User{ID: "anonymous", Username: "anonymous", Role: "admin", Source: "none"}, nil
}

// Name returns the provider identifier.
func (p *NoneProvider) Name() string {
	return "none"
}

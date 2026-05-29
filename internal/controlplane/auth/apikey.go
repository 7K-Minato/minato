package auth

import (
	"net/http"
)

// APIKeyProvider implements API key authentication using Kubernetes Secret storage.
type APIKeyProvider struct {
	storage *APIKeyStorage
}

// NewAPIKeyProvider creates an API key provider backed by Kubernetes Secret storage.
func NewAPIKeyProvider(storage *APIKeyStorage) *APIKeyProvider {
	return &APIKeyProvider{storage: storage}
}

// Authenticate validates an API key from the X-API-Key header.
func (p *APIKeyProvider) Authenticate(r *http.Request) (*User, error) {
	keyValue := r.Header.Get("X-API-Key")
	if keyValue == "" {
		return nil, ErrUnauthorized
	}

	entry, err := p.storage.GetKey(r.Context(), keyValue)
	if err != nil {
		return nil, ErrUnauthorized
	}

	return &User{
		ID:       entry.UserID,
		Username: entry.Username,
		Role:     entry.Role,
		Source:   "apikey",
	}, nil
}

// Name returns the provider identifier.
func (p *APIKeyProvider) Name() string {
	return "apikey"
}

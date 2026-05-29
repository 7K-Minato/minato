package auth

import (
	"errors"
	"net/http"
	"strings"
)

// BasicProvider implements single shared credential authentication.
type BasicProvider struct {
	user     string
	password string // bcrypt hash
	role     string
}

// NewBasicProvider creates a basic auth provider.
func NewBasicProvider(cfg BasicConfig) (*BasicProvider, error) {
	if cfg.User == "" || cfg.Password == "" {
		return nil, errors.New("basic auth: user and password required")
	}

	password := cfg.Password
	// If password doesn't start with $2a$ (bcrypt prefix), hash it
	if !strings.HasPrefix(password, "$2a$") && !strings.HasPrefix(password, "$2b$") && !strings.HasPrefix(password, "$2y$") {
		hashed, err := HashPassword(password)
		if err != nil {
			return nil, err
		}
		password = hashed
	}

	return &BasicProvider{
		user:     cfg.User,
		password: password,
		role:     cfg.Role,
	}, nil
}

// Authenticate validates basic auth credentials.
func (p *BasicProvider) Authenticate(r *http.Request) (*User, error) {
	user, pass, ok := r.BasicAuth()
	if !ok {
		return nil, ErrUnauthorized
	}

	if user != p.user {
		return nil, ErrUnauthorized
	}

	if !CheckPassword(pass, p.password) {
		return nil, ErrUnauthorized
	}

	return &User{
		ID:       user,
		Username: user,
		Role:     p.role,
		Source:   "basic",
	}, nil
}

// Name returns the provider identifier.
func (p *BasicProvider) Name() string {
	return "basic"
}

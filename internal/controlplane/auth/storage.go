package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	apiKeySecretPrefix = "minato-apikey-"
	apiKeySecretLabel  = "minato.io/apikey"
	apiKeyLength       = 32
)

// APIKeyStorage handles persistence of API keys using Kubernetes Secrets.
type APIKeyStorage struct {
	client    client.Client
	namespace string
}

// APIKeyEntry represents a stored API key.
type APIKeyEntry struct {
	Name      string    `json:"name"`     // Human-readable name (e.g., "ci-cd-pipeline")
	KeyID     string    `json:"keyId"`    // The actual key value (minato_...)
	UserID    string    `json:"userId"`   // Who created it
	Username  string    `json:"username"` // Human-readable username
	Role      string    `json:"role"`     // Role assigned to this key
	CreatedAt time.Time `json:"createdAt"`
	// Namespaces optionally restricts the key to a set of namespace patterns
	// (exact names or trailing-"*" prefix globs). Empty means cluster-wide.
	Namespaces []string `json:"namespaces,omitempty"`
	// ExpiresAt optionally sets an expiry time; expired keys are rejected
	// with 401.
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
}

// Expired reports whether the key has passed its expiry time.
func (e *APIKeyEntry) Expired() bool {
	return e.ExpiresAt != nil && time.Now().After(*e.ExpiresAt)
}

// NewAPIKeyStorage creates a new API key storage backend.
func NewAPIKeyStorage(c client.Client, namespace string) *APIKeyStorage {
	return &APIKeyStorage{
		client:    c,
		namespace: namespace,
	}
}

// GenerateKey creates a new API key for a user.
// Returns the key value (which should be shown to the user ONCE) and the entry.
// namespaces and expiresAt are optional: pass nil for a cluster-wide key
// without expiry.
func (s *APIKeyStorage) GenerateKey(ctx context.Context, name, userID, username, role string, namespaces []string, expiresAt *time.Time) (*APIKeyEntry, string, error) {
	// Generate random key
	keyValue, err := generateRandomKey()
	if err != nil {
		return nil, "", fmt.Errorf("generate key: %w", err)
	}

	entry := &APIKeyEntry{
		Name:       name,
		KeyID:      keyValue,
		UserID:     userID,
		Username:   username,
		Role:       role,
		CreatedAt:  time.Now(),
		Namespaces: namespaces,
		ExpiresAt:  expiresAt,
	}

	// Store in Kubernetes Secret
	secret := s.toSecret(entry)
	if err := s.client.Create(ctx, secret); err != nil {
		return nil, "", fmt.Errorf("store key: %w", err)
	}

	return entry, keyValue, nil
}

// GetKey looks up an API key by its value.
func (s *APIKeyStorage) GetKey(ctx context.Context, keyValue string) (*APIKeyEntry, error) {
	// List all apikey secrets and find matching key
	var secrets corev1.SecretList
	if err := s.client.List(ctx, &secrets,
		client.InNamespace(s.namespace),
		client.MatchingLabels{apiKeySecretLabel: "true"},
	); err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}

	for _, secret := range secrets.Items {
		if string(secret.Data["key"]) == keyValue {
			entry := s.fromSecret(&secret)
			if entry.Expired() {
				return nil, ErrUnauthorized
			}
			return entry, nil
		}
	}

	return nil, ErrUnauthorized
}

// ListKeys returns all API keys (without the actual key values).
func (s *APIKeyStorage) ListKeys(ctx context.Context) ([]*APIKeyEntry, error) {
	var secrets corev1.SecretList
	if err := s.client.List(ctx, &secrets,
		client.InNamespace(s.namespace),
		client.MatchingLabels{apiKeySecretLabel: "true"},
	); err != nil {
		return nil, fmt.Errorf("list keys: %w", err)
	}

	result := make([]*APIKeyEntry, 0, len(secrets.Items))
	for i := range secrets.Items {
		entry := s.fromSecret(&secrets.Items[i])
		entry.KeyID = "" // Don't expose actual key values
		result = append(result, entry)
	}

	return result, nil
}

// DeleteKey removes an API key.
func (s *APIKeyStorage) DeleteKey(ctx context.Context, name string) error {
	secretName := apiKeySecretPrefix + name
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: s.namespace,
		},
	}
	if err := s.client.Delete(ctx, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return fmt.Errorf("key not found: %s", name)
		}
		return fmt.Errorf("delete key: %w", err)
	}
	return nil
}

// toSecret converts an APIKeyEntry to a Kubernetes Secret.
func (s *APIKeyStorage) toSecret(entry *APIKeyEntry) *corev1.Secret {
	data, _ := json.Marshal(entry)
	stringData := map[string]string{
		"key":      entry.KeyID,
		"metadata": string(data),
	}
	if len(entry.Namespaces) > 0 {
		stringData["namespaces"] = strings.Join(entry.Namespaces, ",")
	}
	if entry.ExpiresAt != nil {
		stringData["expiresAt"] = entry.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiKeySecretPrefix + entry.Name,
			Namespace: s.namespace,
			Labels: map[string]string{
				apiKeySecretLabel:      "true",
				"minato.io/created-by": entry.Username,
			},
		},
		StringData: stringData,
	}
}

// fromSecret converts a Kubernetes Secret to an APIKeyEntry.
func (s *APIKeyStorage) fromSecret(secret *corev1.Secret) *APIKeyEntry {
	entry := &APIKeyEntry{}
	if data, ok := secret.Data["metadata"]; ok {
		_ = json.Unmarshal(data, entry)
	}
	// Also populate from secret data directly as fallback
	if entry.KeyID == "" && secret.Data["key"] != nil {
		entry.KeyID = string(secret.Data["key"])
	}
	if entry.Name == "" {
		entry.Name = strings.TrimPrefix(secret.Name, apiKeySecretPrefix)
	}
	// Explicit top-level data fields take precedence over the metadata JSON,
	// so operators can scope or expire a key by editing the Secret directly.
	if v := secret.Data["namespaces"]; len(v) > 0 {
		entry.Namespaces = ParseNamespaces(string(v))
	}
	if v := secret.Data["expiresAt"]; len(v) > 0 {
		if t, err := time.Parse(time.RFC3339, string(v)); err == nil {
			entry.ExpiresAt = &t
		}
	}
	return entry
}

// generateRandomKey creates a cryptographically secure random API key.
func generateRandomKey() (string, error) {
	bytes := make([]byte, apiKeyLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return "minato_" + hex.EncodeToString(bytes), nil
}

package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const testStorageNS = "minato"

func newTestStorage(t *testing.T, objs ...client.Object) *APIKeyStorage {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return NewAPIKeyStorage(c, testStorageNS)
}

// keySecret builds a Secret the way the API server would persist it
// (StringData converted to Data).
func keySecret(t *testing.T, entry *APIKeyEntry, keyValue string, extra map[string]string) *corev1.Secret {
	t.Helper()
	meta, err := json.Marshal(entry)
	require.NoError(t, err)
	data := map[string][]byte{
		"key":      []byte(keyValue),
		"metadata": meta,
	}
	for k, v := range extra {
		data[k] = []byte(v)
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      apiKeySecretPrefix + entry.Name,
			Namespace: testStorageNS,
			Labels:    map[string]string{apiKeySecretLabel: "true"},
		},
		Data: data,
	}
}

func TestGenerateKey_WithScopeAndExpiry(t *testing.T) {
	s := newTestStorage(t)
	expires := time.Now().Add(time.Hour).UTC().Truncate(time.Second)

	entry, keyValue, err := s.GenerateKey(context.Background(), "ci", "uid", "alice", "operator", []string{"tenant-*"}, &expires)
	require.NoError(t, err)
	assert.NotEmpty(t, keyValue)
	assert.Equal(t, []string{"tenant-*"}, entry.Namespaces)
	require.NotNil(t, entry.ExpiresAt)

	var secret corev1.Secret
	require.NoError(t, s.client.Get(context.Background(),
		client.ObjectKey{Name: apiKeySecretPrefix + "ci", Namespace: testStorageNS}, &secret))
	assert.Equal(t, "tenant-*", secret.StringData["namespaces"])
	assert.Equal(t, expires.Format(time.RFC3339), secret.StringData["expiresAt"])
}

func TestGenerateKey_ClusterWideDefault(t *testing.T) {
	s := newTestStorage(t)
	entry, _, err := s.GenerateKey(context.Background(), "admin-key", "uid", "alice", "admin", nil, nil)
	require.NoError(t, err)
	assert.Empty(t, entry.Namespaces)
	assert.Nil(t, entry.ExpiresAt)

	var secret corev1.Secret
	require.NoError(t, s.client.Get(context.Background(),
		client.ObjectKey{Name: apiKeySecretPrefix + "admin-key", Namespace: testStorageNS}, &secret))
	_, hasNS := secret.StringData["namespaces"]
	_, hasExp := secret.StringData["expiresAt"]
	assert.False(t, hasNS)
	assert.False(t, hasExp)
}

func TestGetKey_AttachesNamespaces(t *testing.T) {
	entry := &APIKeyEntry{Name: "ci", UserID: "uid", Username: "alice", Role: "operator"}
	secret := keySecret(t, entry, "minato_scoped", map[string]string{"namespaces": "tenant-a, tenant-*"})
	s := newTestStorage(t, secret)

	got, err := s.GetKey(context.Background(), "minato_scoped")
	require.NoError(t, err)
	assert.Equal(t, []string{"tenant-a", "tenant-*"}, got.Namespaces)
}

func TestGetKey_Expired(t *testing.T) {
	entry := &APIKeyEntry{Name: "old", UserID: "uid", Username: "alice", Role: "admin"}
	past := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	secret := keySecret(t, entry, "minato_expired", map[string]string{"expiresAt": past})
	s := newTestStorage(t, secret)

	_, err := s.GetKey(context.Background(), "minato_expired")
	assert.ErrorIs(t, err, ErrUnauthorized)
}

func TestGetKey_NotExpired(t *testing.T) {
	entry := &APIKeyEntry{Name: "fresh", UserID: "uid", Username: "alice", Role: "admin"}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	secret := keySecret(t, entry, "minato_fresh", map[string]string{"expiresAt": future})
	s := newTestStorage(t, secret)

	got, err := s.GetKey(context.Background(), "minato_fresh")
	require.NoError(t, err)
	require.NotNil(t, got.ExpiresAt)
	assert.False(t, got.Expired())
}

func TestFromSecret_TopLevelFieldsOverrideMetadata(t *testing.T) {
	entry := &APIKeyEntry{Name: "ci", UserID: "uid", Username: "alice", Role: "admin", Namespaces: []string{"json-ns"}}
	secret := keySecret(t, entry, "minato_x", map[string]string{"namespaces": "override-ns"})
	s := newTestStorage(t, secret)

	got, err := s.GetKey(context.Background(), "minato_x")
	require.NoError(t, err)
	assert.Equal(t, []string{"override-ns"}, got.Namespaces)
}

func TestListKeys_ShowsExpiryHidesKeyMaterial(t *testing.T) {
	future := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	entry := &APIKeyEntry{Name: "ci", UserID: "uid", Username: "alice", Role: "admin", ExpiresAt: &future}
	secret := keySecret(t, entry, "minato_secret_value", nil)
	s := newTestStorage(t, secret)

	keys, err := s.ListKeys(context.Background())
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Empty(t, keys[0].KeyID)
	require.NotNil(t, keys[0].ExpiresAt)
	assert.True(t, future.Equal(*keys[0].ExpiresAt))
}

func TestAPIKeyProvider_ScopedKey(t *testing.T) {
	entry := &APIKeyEntry{Name: "ci", UserID: "uid", Username: "ci-bot", Role: "admin", Namespaces: []string{"tenant-a"}}
	secret := keySecret(t, entry, "minato_scoped", nil)
	s := newTestStorage(t, secret)
	p := NewAPIKeyProvider(s)

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "minato_scoped")
	user, err := p.Authenticate(req)
	require.NoError(t, err)
	assert.Equal(t, "apikey", user.Source)
	assert.Equal(t, []string{"tenant-a"}, user.Namespaces)
	assert.False(t, user.ClusterWide())
}

func TestAPIKeyProvider_ExpiredKeyRejected(t *testing.T) {
	entry := &APIKeyEntry{Name: "old", UserID: "uid", Username: "ci-bot", Role: "admin"}
	past := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)
	secret := keySecret(t, entry, "minato_expired", map[string]string{"expiresAt": past})
	s := newTestStorage(t, secret)
	p := NewAPIKeyProvider(s)

	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "minato_expired")
	_, err := p.Authenticate(req)
	assert.ErrorIs(t, err, ErrUnauthorized)
}

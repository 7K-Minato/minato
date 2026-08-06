package controllers

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controllers/builder"
)

func sftpCapableProfile() *operatorv1.GameProfile {
	p := newTestProfile()
	p.Spec.Capabilities = &operatorv1.CapabilitiesSpec{SFTP: true}
	return p
}

func TestGenerateSFTPPassword(t *testing.T) {
	p1, err := generateSFTPPassword()
	require.NoError(t, err)
	assert.Len(t, p1, sftpPasswordLength)
	for _, c := range p1 {
		assert.True(t, strings.ContainsRune(sftpPasswordAlphabet, c), "non-alphanumeric char %q", c)
	}
	p2, err := generateSFTPPassword()
	require.NoError(t, err)
	assert.NotEqual(t, p1, p2, "passwords must be random")
}

func TestEnsureSFTPSecretCreatesSecret(t *testing.T) {
	scheme := newTestScheme()
	server := newTestGameServer()
	profile := sftpCapableProfile()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	require.NoError(t, reconciler.ensureSFTPSecret(context.Background(), server, profile))

	secret := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Name: builder.SFTPSecretName(server.Name), Namespace: server.Namespace}, secret))

	assert.Equal(t, builder.SFTPUsername, string(secret.Data[builder.SFTPUsernameKey]))
	assert.Len(t, secret.Data[builder.SFTPPasswordKey], sftpPasswordLength)
	assert.Contains(t, string(secret.Data[builder.SFTPUsersKey]), profile.Spec.Storage.MountPath)

	require.Len(t, secret.OwnerReferences, 1)
	assert.Equal(t, "GameServer", secret.OwnerReferences[0].Kind)
	assert.Equal(t, server.Name, secret.OwnerReferences[0].Name)
}

func TestEnsureSFTPSecretIsIdempotent(t *testing.T) {
	scheme := newTestScheme()
	server := newTestGameServer()
	profile := sftpCapableProfile()
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(server).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}

	require.NoError(t, reconciler.ensureSFTPSecret(context.Background(), server, profile))
	first := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Name: builder.SFTPSecretName(server.Name), Namespace: server.Namespace}, first))

	// Second reconcile must reuse the existing password, never rotate.
	require.NoError(t, reconciler.ensureSFTPSecret(context.Background(), server, profile))
	second := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Name: builder.SFTPSecretName(server.Name), Namespace: server.Namespace}, second))
	assert.Equal(t, first.Data[builder.SFTPPasswordKey], second.Data[builder.SFTPPasswordKey])
	assert.Equal(t, first.ResourceVersion, second.ResourceVersion)

	// Deleting the secret (manual rotation) yields a new password on the next
	// reconcile.
	require.NoError(t, cl.Delete(context.Background(), second))
	require.NoError(t, reconciler.ensureSFTPSecret(context.Background(), server, profile))
	third := &corev1.Secret{}
	require.NoError(t, cl.Get(context.Background(),
		types.NamespacedName{Name: builder.SFTPSecretName(server.Name), Namespace: server.Namespace}, third))
	assert.NotEqual(t, first.Data[builder.SFTPPasswordKey], third.Data[builder.SFTPPasswordKey])
}

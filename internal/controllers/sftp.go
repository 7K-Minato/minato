package controllers

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"golang.org/x/crypto/bcrypt"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controllers/builder"
)

const sftpPasswordLength = 32

// sftpPasswordAlphabet is alphanumeric-only for safe display/copy in UIs.
const sftpPasswordAlphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// generateSFTPPassword returns a cryptographically random alphanumeric
// password.
func generateSFTPPassword() (string, error) {
	out := make([]byte, sftpPasswordLength)
	max := big.NewInt(int64(len(sftpPasswordAlphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("generate sftp password: %w", err)
		}
		out[i] = sftpPasswordAlphabet[n.Int64()]
	}
	return string(out), nil
}

// ensureSFTPSecret creates the per-server SFTP credentials secret if it does
// not exist. The generated password is never rotated by the reconciler: if
// the secret already exists it is left untouched. Manual rotation is done by
// deleting the secret; the next reconcile recreates it with a fresh password.
func (r *GameServerReconciler) ensureSFTPSecret(ctx context.Context, server *operatorv1.GameServer, profile *operatorv1.GameProfile) error {
	name := builder.SFTPSecretName(server.Name)
	existing := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: server.Namespace}, existing); err == nil {
		return nil
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	password, err := generateSFTPPassword()
	if err != nil {
		return err
	}
	// SFTPGo stores dump passwords as-is; pre-hash with bcrypt so the
	// plaintext password (served to users via the API) never sits in the
	// users dump, and so dump restore works at all (plaintext is not hashed
	// on restore).
	passwordHash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash sftp password: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: server.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name": "minato",
				"minato.io/gameserver":   server.Name,
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			builder.SFTPUsernameKey: []byte(builder.SFTPUsername),
			builder.SFTPPasswordKey: []byte(password),
			builder.SFTPUsersKey:    []byte(builder.SFTPUsersJSON(profile.Spec.Storage.MountPath, string(passwordHash))),
		},
	}
	if err := controllerutil.SetControllerReference(server, secret, r.Scheme); err != nil {
		return err
	}
	if err := r.Create(ctx, secret); err != nil && !apierrors.IsAlreadyExists(err) {
		return fmt.Errorf("create sftp secret %s/%s: %w", server.Namespace, name, err)
	}
	return nil
}

package webhook

import (
	"context"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
)

func TestObserveValidation_Allowed(t *testing.T) {
	before := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues("gameprofile", "create", "allowed"))

	v := &GameProfileValidator{}
	profile := &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-allowed"},
		Spec: operatorv1.GameProfileSpec{
			Image:       "img",
			DisplayName: "Test",
			Storage: operatorv1.StorageSpec{
				MountPath:   "/data",
				SizeDefault: "1Gi",
			},
			Agent: operatorv1.AgentSpec{Image: "agent"},
		},
	}
	_, err := v.ValidateCreate(context.Background(), profile)
	require.NoError(t, err)

	after := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues("gameprofile", "create", "allowed"))
	assert.Equal(t, before+1, after)
	assert.Greater(t, testutil.CollectAndCount(webhookRequestDuration), 0)
}

func TestObserveValidation_Denied(t *testing.T) {
	before := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues("gameprofile", "create", "denied"))

	v := &GameProfileValidator{}
	_, err := v.ValidateCreate(context.Background(), &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-denied"},
	})
	require.Error(t, err)

	after := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues("gameprofile", "create", "denied"))
	assert.Equal(t, before+1, after)
}

func TestObserveValidation_GameServerDelete(t *testing.T) {
	before := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues("gameserver", "delete", "allowed"))

	v := &GameServerValidator{}
	_, err := v.ValidateDelete(context.Background(), &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "metrics-delete", Namespace: "default"},
	})
	require.NoError(t, err)

	after := testutil.ToFloat64(webhookRequestsTotal.WithLabelValues("gameserver", "delete", "allowed"))
	assert.Equal(t, before+1, after)
}

package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestDetectPrometheusOperator(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)
	ctx := context.Background()

	t.Run("CRD present", func(t *testing.T) {
		ResetPrometheusDetection()
		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "servicemonitors.monitoring.coreos.com"},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()
		detected := DetectPrometheusOperator(ctx, cl)
		assert.True(t, detected)
		assert.True(t, IsPrometheusOperatorDetected())
	})

	t.Run("CRD not found", func(t *testing.T) {
		ResetPrometheusDetection()
		cl := fake.NewClientBuilder().WithScheme(scheme).Build()
		detected := DetectPrometheusOperator(ctx, cl)
		assert.False(t, detected)
		assert.False(t, IsPrometheusOperatorDetected())
	})

	t.Run("cached result", func(t *testing.T) {
		ResetPrometheusDetection()
		crd := &apiextensionsv1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{Name: "servicemonitors.monitoring.coreos.com"},
		}
		cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()
		_ = DetectPrometheusOperator(ctx, cl)

		// Second call should return cached result even with different client
		cl2 := fake.NewClientBuilder().WithScheme(scheme).Build()
		detected := DetectPrometheusOperator(ctx, cl2)
		assert.True(t, detected)
	})
}

func TestResetPrometheusDetection(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = apiextensionsv1.AddToScheme(scheme)
	ctx := context.Background()

	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "servicemonitors.monitoring.coreos.com"},
	}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(crd).Build()
	_ = DetectPrometheusOperator(ctx, cl)
	require.True(t, IsPrometheusOperatorDetected())

	ResetPrometheusDetection()
	assert.False(t, IsPrometheusOperatorDetected())
}

func TestMustRegisterPrometheusCRDs(t *testing.T) {
	err := MustRegisterPrometheusCRDs()
	assert.NoError(t, err)
}

package controllers

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

func TestGameServerReconcilerSmoke(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := operatorv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("add scheme: %v", err)
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).Build()
	reconciler := &GameServerReconciler{Client: cl, Scheme: scheme}
	if reconciler.Client == nil || reconciler.Scheme == nil {
		t.Fatalf("expected reconciler to be initialized")
	}
}

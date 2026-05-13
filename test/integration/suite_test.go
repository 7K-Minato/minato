package integration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	operatorv1 "github.com/7k-group/minami/api/operator/v1"
	"github.com/7k-group/minami/internal/controllers"
)

var (
	testEnv    *envtest.Environment
	k8sManager manager.Manager
	ctx        context.Context
	cancel     context.CancelFunc
)

func TestMain(m *testing.M) {
	ctx, cancel = context.WithCancel(context.Background())

	scheme := runtime.NewScheme()
	if err := operatorv1.AddToScheme(scheme); err != nil {
		os.Exit(1)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		os.Exit(1)
	}

	root := filepath.Join("..", "..")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{filepath.Join(root, "config", "crd", "bases")},
	}

	config, err := testEnv.Start()
	if err != nil {
		os.Exit(1)
	}

	k8sManager, err = manager.New(config, manager.Options{Scheme: scheme})
	if err != nil {
		os.Exit(1)
	}

	reconciler := &controllers.GameServerReconciler{Client: k8sManager.GetClient(), Scheme: scheme}
	if err := reconciler.SetupWithManager(k8sManager); err != nil {
		os.Exit(1)
	}

	go func() {
		_ = k8sManager.Start(ctrl.SetupSignalHandler())
	}()

	code := m.Run()
	cancel()
	_ = testEnv.Stop()
	os.Exit(code)
}

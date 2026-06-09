package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controllers"
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
		fmt.Fprintf(os.Stderr, "failed to add operatorv1 scheme: %v\n", err)
		os.Exit(1)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add appsv1 scheme: %v\n", err)
		os.Exit(1)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		fmt.Fprintf(os.Stderr, "failed to add corev1 scheme: %v\n", err)
		os.Exit(1)
	}

	root := filepath.Join("..", "..")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join(root, "config", "crd", "bases")},
		BinaryAssetsDirectory: filepath.Join(root, "bin", "k8s", "1.35.0-linux-amd64"),
	}

	config, err := testEnv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start envtest: %v\n", err)
		os.Exit(1)
	}

	k8sManager, err = manager.New(config, manager.Options{Scheme: scheme})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create manager: %v\n", err)
		os.Exit(1)
	}

	reconciler := &controllers.GameServerReconciler{Client: k8sManager.GetClient(), Scheme: scheme}
	if err := reconciler.SetupWithManager(k8sManager); err != nil {
		fmt.Fprintf(os.Stderr, "failed to setup reconciler: %v\n", err)
		os.Exit(1)
	}

	go func() {
		_ = k8sManager.Start(ctx)
	}()

	code := m.Run()
	cancel()
	_ = testEnv.Stop()
	os.Exit(code)
}

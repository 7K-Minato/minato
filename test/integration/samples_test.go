package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestSamplesApplyCleanly guards against config/samples drifting from the CRDs:
// every sample must pass strict field validation against the apiserver schemas
// installed from config/crd/bases.
func TestSamplesApplyCleanly(t *testing.T) {
	samplesDir := filepath.Join("..", "..", "config", "samples")
	entries, err := os.ReadDir(samplesDir)
	if err != nil {
		t.Fatalf("read samples dir: %v", err)
	}

	c := k8sManager.GetClient()
	decoder := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "minato"}}
	if err := c.Create(ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		t.Fatalf("create minato namespace: %v", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		t.Run(entry.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(samplesDir, entry.Name()))
			if err != nil {
				t.Fatalf("read sample: %v", err)
			}
			obj := &unstructured.Unstructured{}
			if _, _, err := decoder.Decode(raw, nil, obj); err != nil {
				t.Fatalf("decode sample: %v", err)
			}
			if obj.GetNamespace() == "" && obj.GetObjectKind().GroupVersionKind().Kind != "GameProfile" {
				obj.SetNamespace("default")
			}
			err = c.Create(ctx, obj, &client.CreateOptions{
				DryRun:          []string{"All"},
				FieldValidation: "Strict",
			})
			if err != nil {
				t.Fatalf("sample does not validate against CRD schema: %v", err)
			}
		})
	}
}

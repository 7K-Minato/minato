package actions

import "testing"

func TestLoadCatalogFromBytesJSON(t *testing.T) {
	content := []byte(`{"actions": [{"name": "save", "params": {"world": {"type": "string", "required": true}}}]}`)
	catalog, err := LoadCatalogFromBytes(content)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(catalog.Actions))
	}
}

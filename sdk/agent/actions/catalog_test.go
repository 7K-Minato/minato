package actions

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCatalogFromFileValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	content := []byte(`{"actions": [{"name": "restart", "steps": [{"type": "exec", "inputs": {"command": "echo hello"}}]}]}`)
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	catalog, err := LoadCatalogFromFile(path)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(catalog.Actions))
	}
	if catalog.Actions[0].Name != "restart" {
		t.Fatalf("expected action name restart, got %s", catalog.Actions[0].Name)
	}
}

func TestLoadCatalogFromFileEmptyPath(t *testing.T) {
	_, err := LoadCatalogFromFile("")
	if err == nil {
		t.Fatalf("expected error for empty path")
	}
}

func TestLoadCatalogFromFileInvalidPath(t *testing.T) {
	_, err := LoadCatalogFromFile("/nonexistent/path/catalog.json")
	if err == nil {
		t.Fatalf("expected error for invalid path")
	}
}

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

func TestLoadCatalogFromBytesYAML(t *testing.T) {
	content := []byte(`
actions:
  - name: backup
    description: Backup world
    steps:
      - type: exec
        inputs:
          command: echo backup
`)
	catalog, err := LoadCatalogFromBytes(content)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	if len(catalog.Actions) != 1 {
		t.Fatalf("expected 1 action, got %d", len(catalog.Actions))
	}
	if catalog.Actions[0].Name != "backup" {
		t.Fatalf("expected action name backup, got %s", catalog.Actions[0].Name)
	}
}

func TestLoadCatalogFromBytesEmpty(t *testing.T) {
	_, err := LoadCatalogFromBytes([]byte(""))
	if err == nil {
		t.Fatalf("expected error for empty content")
	}
}

func TestLoadCatalogFromBytesWhitespace(t *testing.T) {
	_, err := LoadCatalogFromBytes([]byte("   \n\t  "))
	if err == nil {
		t.Fatalf("expected error for whitespace-only content")
	}
}

func TestLoadCatalogFromBytesInvalidJSON(t *testing.T) {
	content := []byte(`{invalid json}`)
	_, err := LoadCatalogFromBytes(content)
	if err == nil {
		t.Fatalf("expected error for invalid JSON")
	}
}

func TestLoadCatalogFromBytesInvalidYAML(t *testing.T) {
	content := []byte(`actions: [bad: yaml: structure`)
	_, err := LoadCatalogFromBytes(content)
	if err == nil {
		t.Fatalf("expected error for invalid YAML")
	}
}

func TestLoadCatalogFromBytesUnknownFieldsJSON(t *testing.T) {
	content := []byte(`{"actions": [{"name": "save"}], "unknownField": "value"}`)
	_, err := LoadCatalogFromBytes(content)
	if err == nil {
		t.Fatalf("expected error for unknown fields in JSON")
	}
}

func TestLoadCatalogFromBytesUnknownFieldsYAML(t *testing.T) {
	content := []byte(`
actions:
  - name: save
unknownField: value
`)
	_, err := LoadCatalogFromBytes(content)
	if err == nil {
		t.Fatalf("expected error for unknown fields in YAML")
	}
}

func TestCatalogFindActionFound(t *testing.T) {
	catalog := Catalog{
		Actions: []ActionDefinition{
			{Name: "save"},
			{Name: "restart"},
		},
	}
	action, ok := catalog.FindAction("restart")
	if !ok {
		t.Fatalf("expected to find action restart")
	}
	if action.Name != "restart" {
		t.Fatalf("expected action name restart, got %s", action.Name)
	}
}

func TestCatalogFindActionNotFound(t *testing.T) {
	catalog := Catalog{
		Actions: []ActionDefinition{
			{Name: "save"},
		},
	}
	_, ok := catalog.FindAction("restart")
	if ok {
		t.Fatalf("expected not to find action restart")
	}
}

func TestCatalogFindActionEmptyCatalog(t *testing.T) {
	catalog := Catalog{}
	_, ok := catalog.FindAction("save")
	if ok {
		t.Fatalf("expected not to find action in empty catalog")
	}
}

func TestDecodeJSONEOF(t *testing.T) {
	// Empty JSON object should decode without error (EOF after valid decode is ignored)
	content := []byte(`{}`)
	catalog, err := decodeJSON(content)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(catalog.Actions) != 0 {
		t.Fatalf("expected 0 actions, got %d", len(catalog.Actions))
	}
}

func TestDecodeYAMLEOF(t *testing.T) {
	content := []byte(`actions:`)
	catalog, err := decodeYAML(content)
	if err != nil {
		t.Fatalf("expected no error for YAML with just actions key: %v", err)
	}
	if len(catalog.Actions) != 0 {
		t.Fatalf("expected 0 actions, got %d", len(catalog.Actions))
	}
}

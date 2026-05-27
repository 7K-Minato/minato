package actions

import (
	"os"
	"testing"
)

func TestCatalogFromEnv(t *testing.T) {
	_ = os.Setenv("minato_ACTION_SAVE_STEP_1", "rcon")
	_ = os.Setenv("minato_ACTION_SAVE_INPUT_1_COMMAND", "save-all")
	_ = os.Setenv("minato_ACTION_BACKUP_STEP_1", "exec")
	_ = os.Setenv("minato_ACTION_BACKUP_INPUT_1_COMMAND", "tar")
	_ = os.Setenv("minato_ACTION_BACKUP_INPUT_1_ARGS", "-czf backup.tar.gz")
	defer func() {
		_ = os.Unsetenv("minato_ACTION_SAVE_STEP_1")
		_ = os.Unsetenv("minato_ACTION_SAVE_INPUT_1_COMMAND")
		_ = os.Unsetenv("minato_ACTION_BACKUP_STEP_1")
		_ = os.Unsetenv("minato_ACTION_BACKUP_INPUT_1_COMMAND")
		_ = os.Unsetenv("minato_ACTION_BACKUP_INPUT_1_ARGS")
	}()

	catalog := CatalogFromEnv()
	if len(catalog.Actions) != 2 {
		t.Fatalf("expected 2 actions, got %d", len(catalog.Actions))
	}

	save, ok := catalog.FindAction("save")
	if !ok {
		t.Fatalf("expected save action")
	}
	if len(save.Steps) != 1 || save.Steps[0].Type != "rcon" {
		t.Fatalf("expected rcon step, got %+v", save.Steps)
	}
	if save.Steps[0].Inputs["command"] != "save-all" {
		t.Fatalf("expected command save-all, got %q", save.Steps[0].Inputs["command"])
	}

	backup, ok := catalog.FindAction("backup")
	if !ok {
		t.Fatalf("expected backup action")
	}
	if len(backup.Steps) != 1 || backup.Steps[0].Type != "exec" {
		t.Fatalf("expected exec step, got %+v", backup.Steps)
	}
	if backup.Steps[0].Inputs["command"] != "tar" {
		t.Fatalf("expected command tar, got %q", backup.Steps[0].Inputs["command"])
	}
	if backup.Steps[0].Inputs["args"] != "-czf backup.tar.gz" {
		t.Fatalf("expected args -czf backup.tar.gz, got %q", backup.Steps[0].Inputs["args"])
	}
}

func TestCatalogFromEnvNoMatch(t *testing.T) {
	catalog := CatalogFromEnv()
	if len(catalog.Actions) != 0 {
		t.Fatalf("expected 0 actions, got %d", len(catalog.Actions))
	}
}

func TestCatalogFromEnvInvalidEntry(t *testing.T) {
	_ = os.Setenv("minato_ACTION_NOSEGMENT", "value")
	defer func() { _ = os.Unsetenv("minato_ACTION_NOSEGMENT") }()

	catalog := CatalogFromEnv()
	if len(catalog.Actions) != 0 {
		t.Fatalf("expected 0 actions for invalid entry, got %d", len(catalog.Actions))
	}
}

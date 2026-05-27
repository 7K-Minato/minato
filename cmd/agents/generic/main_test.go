package main

import (
	"context"
	"testing"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
	"github.com/7k-group/minato/sdk/agent/actions"
)

func TestGenericAgent_Info(t *testing.T) {
	agent := &genericAgent{
		name:    "test-agent",
		version: "1.0.0",
		catalog: actions.Catalog{
			Actions: []actions.ActionDefinition{
				{Name: "test-action", Description: "Test"},
			},
		},
		runtime: &noopRuntime{},
	}

	resp, err := agent.Info(context.Background(), &agentv1.InfoRequest{})
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}

	if resp.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got %q", resp.Name)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got %q", resp.Version)
	}
	if len(resp.Actions) != 1 {
		t.Errorf("expected 1 action, got %d", len(resp.Actions))
	}
}

func TestGenericAgent_Info_EmptyCatalog(t *testing.T) {
	agent := &genericAgent{
		name:    "test-agent",
		version: "1.0.0",
		catalog: actions.Catalog{},
		runtime: &noopRuntime{},
	}

	resp, err := agent.Info(context.Background(), &agentv1.InfoRequest{})
	if err != nil {
		t.Fatalf("Info failed: %v", err)
	}

	if len(resp.Actions) != 0 {
		t.Errorf("expected 0 actions, got %d", len(resp.Actions))
	}
}

func TestGenericAgent_HealthCheck(t *testing.T) {
	agent := &genericAgent{runtime: &noopRuntime{}}

	resp, err := agent.HealthCheck(context.Background(), &agentv1.HealthRequest{})
	if err != nil {
		t.Fatalf("HealthCheck failed: %v", err)
	}

	if !resp.Ready {
		t.Error("expected Ready=true")
	}
}

func TestGenericAgent_PrepareShutdown(t *testing.T) {
	agent := &genericAgent{runtime: &noopRuntime{}}

	resp, err := agent.PrepareShutdown(context.Background(), &agentv1.ShutdownRequest{})
	if err != nil {
		t.Fatalf("PrepareShutdown failed: %v", err)
	}

	if !resp.Success {
		t.Error("expected Success=true")
	}
}

func TestGenericAgent_GetPlayers(t *testing.T) {
	agent := &genericAgent{runtime: &noopRuntime{}}

	resp, err := agent.GetPlayers(context.Background(), &agentv1.PlayersRequest{})
	if err != nil {
		t.Fatalf("GetPlayers failed: %v", err)
	}

	if resp.Online != 0 {
		t.Errorf("expected Online=0, got %d", resp.Online)
	}
}

func TestGenericAgent_ExecuteAction_EmptyName(t *testing.T) {
	agent := &genericAgent{
		catalog: actions.Catalog{},
		runtime: &noopRuntime{},
	}

	resp, err := agent.ExecuteAction(context.Background(), &agentv1.ExecuteActionRequest{
		ActionName: "",
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if resp.State != agentv1.ActionState_ACTION_STATE_REJECTED {
		t.Errorf("expected REJECTED, got %v", resp.State)
	}
}

func TestGenericAgent_ExecuteAction_Unknown(t *testing.T) {
	agent := &genericAgent{
		catalog: actions.Catalog{},
		runtime: &noopRuntime{},
	}

	resp, err := agent.ExecuteAction(context.Background(), &agentv1.ExecuteActionRequest{
		ActionName: "unknown",
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if resp.State != agentv1.ActionState_ACTION_STATE_REJECTED {
		t.Errorf("expected REJECTED, got %v", resp.State)
	}
}

func TestGenericAgent_ExecuteAction_NilRuntime(t *testing.T) {
	agent := &genericAgent{
		catalog: actions.Catalog{
			Actions: []actions.ActionDefinition{
				{Name: "test"},
			},
		},
		runtime: nil,
	}

	resp, err := agent.ExecuteAction(context.Background(), &agentv1.ExecuteActionRequest{
		ActionName: "test",
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if resp.State != agentv1.ActionState_ACTION_STATE_FAILED {
		t.Errorf("expected FAILED, got %v", resp.State)
	}
}

func TestGenericAgent_ExecuteAction_Success(t *testing.T) {
	agent := &genericAgent{
		catalog: actions.Catalog{
			Actions: []actions.ActionDefinition{
				{Name: "test"},
			},
		},
		runtime: &noopRuntime{},
	}

	resp, err := agent.ExecuteAction(context.Background(), &agentv1.ExecuteActionRequest{
		ActionName: "test",
	})
	if err != nil {
		t.Fatalf("ExecuteAction failed: %v", err)
	}

	if resp.State != agentv1.ActionState_ACTION_STATE_SUCCEEDED {
		t.Errorf("expected SUCCEEDED, got %v", resp.State)
	}
}

func TestNoopRuntime(t *testing.T) {
	r := &noopRuntime{}
	ctx := context.Background()

	if _, err := r.RCON(ctx, "test"); err != nil {
		t.Errorf("RCON error: %v", err)
	}
	if _, err := r.Exec(ctx, "test", nil); err != nil {
		t.Errorf("Exec error: %v", err)
	}
	if _, err := r.HTTP(ctx, "GET", "http://test", ""); err != nil {
		t.Errorf("HTTP error: %v", err)
	}
	if err := r.Signal(ctx, "test", "SIGTERM"); err != nil {
		t.Errorf("Signal error: %v", err)
	}
	if err := r.Sleep(ctx, 0); err != nil {
		t.Errorf("Sleep error: %v", err)
	}
}

func TestMapKeys(t *testing.T) {
	input := map[string]string{"c": "3", "a": "1", "b": "2"}
	keys := mapKeys(input)

	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}

	expected := []string{"a", "b", "c"}
	for i, k := range expected {
		if keys[i] != k {
			t.Errorf("expected key %q at index %d, got %q", k, i, keys[i])
		}
	}
}

func TestMapKeys_Empty(t *testing.T) {
	keys := mapKeys(map[string]string{})
	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

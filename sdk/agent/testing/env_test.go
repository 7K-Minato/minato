package testing

import (
	"context"
	"testing"

	"github.com/7k-minato/minato/sdk/agent/rcon"
)

func TestNewFakeAgentEnv(t *testing.T) {
	env := NewFakeAgentEnv()
	if env == nil {
		t.Fatalf("expected non-nil FakeAgentEnv")
	}
	if env.RCON == nil {
		t.Fatalf("expected non-nil RCON client")
	}
}

func TestFakeAgentEnvRCONInterface(t *testing.T) {
	env := NewFakeAgentEnv()
	var _ rcon.Client = env.RCON
}

func TestFakeAgentEnvRCONCommand(t *testing.T) {
	env := NewFakeAgentEnv()
	env.RCON.Response = "test response"

	resp, err := env.RCON.Command(context.TODO(), "test command")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "test response" {
		t.Fatalf("expected 'test response', got %q", resp)
	}
	if len(env.RCON.Commands) != 1 || env.RCON.Commands[0] != "test command" {
		t.Fatalf("expected command recorded, got %v", env.RCON.Commands)
	}
}

func TestFakeAgentEnvRCONClose(t *testing.T) {
	env := NewFakeAgentEnv()
	if err := env.RCON.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFakeAgentEnvMultipleInstances(t *testing.T) {
	env1 := NewFakeAgentEnv()
	env2 := NewFakeAgentEnv()

	env1.RCON.Response = "env1"
	env2.RCON.Response = "env2"

	resp1, _ := env1.RCON.Command(context.TODO(), "cmd")
	resp2, _ := env2.RCON.Command(context.TODO(), "cmd")

	if resp1 != "env1" {
		t.Fatalf("expected env1 response, got %q", resp1)
	}
	if resp2 != "env2" {
		t.Fatalf("expected env2 response, got %q", resp2)
	}
}

func TestFakeAgentEnvRCONError(t *testing.T) {
	env := NewFakeAgentEnv()
	env.RCON.Err = errTest

	_, err := env.RCON.Command(context.TODO(), "fail")
	if err == nil {
		t.Fatalf("expected error")
	}
}

var errTest = errTestError{}

type errTestError struct{}

func (e errTestError) Error() string { return "test error" }

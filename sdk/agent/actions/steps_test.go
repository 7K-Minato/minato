package actions

import (
	"context"
	"testing"
	"time"
)

type fakeRuntime struct {
	lastCommand string
}

func (f *fakeRuntime) RCON(ctx context.Context, command string) (string, error) {
	f.lastCommand = command
	return "ok", nil
}

func (f *fakeRuntime) Exec(ctx context.Context, command string, args []string) (string, error) {
	f.lastCommand = command
	return "", nil
}

func (f *fakeRuntime) HTTP(ctx context.Context, method string, url string, body string) (string, error) {
	f.lastCommand = method + " " + url
	return "", nil
}

func (f *fakeRuntime) Signal(ctx context.Context, target string, signal string) error {
	f.lastCommand = target + ":" + signal
	return nil
}

func (f *fakeRuntime) Sleep(ctx context.Context, duration time.Duration) error {
	return nil
}

func TestExecuteRCONStep(t *testing.T) {
	action := ActionDefinition{
		Name: "save",
		Params: map[string]ParamSchema{
			"world": {Type: "string", Required: true},
		},
		Steps: []Step{
			{Type: "rcon", Inputs: map[string]string{"command": "save {{.world}}"}},
		},
	}

	runtime := &fakeRuntime{}
	result, err := Execute(context.Background(), action, map[string]string{"world": "main"}, runtime)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if runtime.lastCommand != "save main" {
		t.Fatalf("expected rendered command, got %q", runtime.lastCommand)
	}
	if len(result.Outputs) != 0 {
		t.Fatalf("expected no outputs")
	}
}

func TestExecuteNilRuntime(t *testing.T) {
	action := ActionDefinition{Name: "noop"}
	_, err := Execute(context.Background(), action, map[string]string{}, nil)
	if err == nil {
		t.Fatalf("expected error for nil runtime")
	}
}

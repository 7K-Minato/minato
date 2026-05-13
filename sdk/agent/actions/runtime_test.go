package actions

import (
	"context"
	"testing"
	"time"
)

type noopRuntime struct{}

func (n *noopRuntime) RCON(ctx context.Context, command string) (string, error) { return "", nil }
func (n *noopRuntime) Exec(ctx context.Context, command string, args []string) (string, error) {
	return "", nil
}
func (n *noopRuntime) HTTP(ctx context.Context, method string, url string, body string) (string, error) {
	return "", nil
}
func (n *noopRuntime) Signal(ctx context.Context, target string, signal string) error { return nil }
func (n *noopRuntime) Sleep(ctx context.Context, duration time.Duration) error        { return nil }

func TestRuntimeInterface(t *testing.T) {
	var runtime Runtime = &noopRuntime{}
	_, _ = runtime.RCON(context.Background(), "test")
}

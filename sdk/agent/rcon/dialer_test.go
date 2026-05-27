package rcon

import (
	"context"
	"testing"
)

func TestDialerFunc(t *testing.T) {
	called := false
	dialer := DialerFunc(func(ctx context.Context, addr, password string) (Client, error) {
		called = true
		if addr != "localhost:25575" {
			t.Errorf("expected addr localhost:25575, got %s", addr)
		}
		if password != "testpass" {
			t.Errorf("expected password testpass, got %s", password)
		}
		return nil, nil
	})

	_, _ = dialer.Dial(context.Background(), "localhost:25575", "testpass")
	if !called {
		t.Error("expected DialerFunc to be called")
	}
}

package rcon

import (
	"context"
	"errors"
	"testing"
)

func TestMockClientImplementsInterface(t *testing.T) {
	var _ Client = (*MockClient)(nil)
}

func TestMockClientCommand(t *testing.T) {
	mock := &MockClient{Response: "hello"}
	resp, err := mock.Command(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "hello" {
		t.Fatalf("expected response hello, got %q", resp)
	}
	if len(mock.Commands) != 1 || mock.Commands[0] != "test" {
		t.Fatalf("expected command recorded, got %v", mock.Commands)
	}
}

func TestMockClientCommandError(t *testing.T) {
	mock := &MockClient{Err: errors.New("connection refused")}
	_, err := mock.Command(context.Background(), "test")
	if err == nil {
		t.Fatalf("expected error")
	}
	if len(mock.Commands) != 1 || mock.Commands[0] != "test" {
		t.Fatalf("expected command recorded even on error, got %v", mock.Commands)
	}
}

func TestMockClientClose(t *testing.T) {
	mock := &MockClient{}
	if err := mock.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockDialerImplementsInterface(t *testing.T) {
	var _ Dialer = (*MockDialer)(nil)
}

func TestMockDialerDial(t *testing.T) {
	client := &MockClient{Response: "pong"}
	mock := &MockDialer{Client: client}
	c, err := mock.Dial(context.Background(), "localhost:25575", "password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != client {
		t.Fatalf("expected same client")
	}
}

func TestMockDialerDialError(t *testing.T) {
	mock := &MockDialer{Err: errors.New("dial failed")}
	_, err := mock.Dial(context.Background(), "localhost:25575", "password")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestDialerFuncImplementsInterface(t *testing.T) {
	var _ Dialer = DialerFunc(func(ctx context.Context, addr string, password string) (Client, error) {
		return &MockClient{}, nil
	})
}

func TestDialerFuncDial(t *testing.T) {
	called := false
	dialer := DialerFunc(func(ctx context.Context, addr string, password string) (Client, error) {
		called = true
		return &MockClient{Response: "ok"}, nil
	})
	client, err := dialer.Dial(context.Background(), "localhost:25575", "pass")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatalf("expected dialer func to be called")
	}
	resp, err := client.Command(context.Background(), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected ok, got %q", resp)
	}
}

func TestSourceClientImplementsInterface(t *testing.T) {
	var _ Client = (*SourceClient)(nil)
}

func TestSourceClientCommand(t *testing.T) {
	mock := &MockClient{Response: "source response"}
	client := NewSourceClient(mock)
	resp, err := client.Command(context.Background(), "status")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "source response" {
		t.Fatalf("expected source response, got %q", resp)
	}
	if len(mock.Commands) != 1 || mock.Commands[0] != "status" {
		t.Fatalf("expected command delegated, got %v", mock.Commands)
	}
}

func TestSourceClientClose(t *testing.T) {
	mock := &MockClient{}
	client := NewSourceClient(mock)
	if err := client.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMinecraftClientImplementsInterface(t *testing.T) {
	var _ Client = (*MinecraftClient)(nil)
}

func TestMinecraftClientCommand(t *testing.T) {
	mock := &MockClient{Response: "minecraft response"}
	client := NewMinecraftClient(mock)
	resp, err := client.Command(context.Background(), "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "minecraft response" {
		t.Fatalf("expected minecraft response, got %q", resp)
	}
	if len(mock.Commands) != 1 || mock.Commands[0] != "list" {
		t.Fatalf("expected command delegated, got %v", mock.Commands)
	}
}

func TestMinecraftClientClose(t *testing.T) {
	mock := &MockClient{}
	client := NewMinecraftClient(mock)
	if err := client.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPalworldClientImplementsInterface(t *testing.T) {
	var _ Client = (*PalworldClient)(nil)
}

func TestPalworldClientCommand(t *testing.T) {
	mock := &MockClient{Response: "palworld response"}
	client := NewPalworldClient(mock)
	resp, err := client.Command(context.Background(), "ShowPlayers")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp != "palworld response" {
		t.Fatalf("expected palworld response, got %q", resp)
	}
	if len(mock.Commands) != 1 || mock.Commands[0] != "ShowPlayers" {
		t.Fatalf("expected command delegated, got %v", mock.Commands)
	}
}

func TestPalworldClientClose(t *testing.T) {
	mock := &MockClient{}
	client := NewPalworldClient(mock)
	if err := client.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMockClientMultipleCommands(t *testing.T) {
	mock := &MockClient{Response: "ok"}
	for range 3 {
		_, err := mock.Command(context.Background(), "cmd")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if len(mock.Commands) != 3 {
		t.Fatalf("expected 3 commands, got %d", len(mock.Commands))
	}
}

func TestMockClientCommandWithContextCancellation(t *testing.T) {
	mock := &MockClient{Response: "ok"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resp, err := mock.Command(ctx, "test")
	if err != nil {
		t.Fatalf("mock client should not check context: %v", err)
	}
	if resp != "ok" {
		t.Fatalf("expected ok, got %q", resp)
	}
}

func TestMockDialerWithNilClient(t *testing.T) {
	mock := &MockDialer{Client: nil}
	c, err := mock.Dial(context.Background(), "localhost:25575", "password")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c != nil {
		t.Fatalf("expected nil client")
	}
}

func TestDialerFuncError(t *testing.T) {
	dialer := DialerFunc(func(ctx context.Context, addr string, password string) (Client, error) {
		return nil, errors.New("dial error")
	})
	_, err := dialer.Dial(context.Background(), "localhost:25575", "pass")
	if err == nil {
		t.Fatalf("expected error")
	}
}

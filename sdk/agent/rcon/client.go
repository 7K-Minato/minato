package rcon

import "context"

type Client interface {
	Command(ctx context.Context, command string) (string, error)
	Close() error
}

type Dialer interface {
	Dial(ctx context.Context, addr string, password string) (Client, error)
}

package actions

import (
	"context"
	"time"
)

type Runtime interface {
	RCON(ctx context.Context, command string) (string, error)
	Exec(ctx context.Context, command string, args []string) (string, error)
	HTTP(ctx context.Context, method string, url string, body string) (string, error)
	Signal(ctx context.Context, target string, signal string) error
	Sleep(ctx context.Context, duration time.Duration) error
}

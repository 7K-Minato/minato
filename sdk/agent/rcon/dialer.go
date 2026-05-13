package rcon

import "context"

type DialerFunc func(ctx context.Context, addr string, password string) (Client, error)

func (d DialerFunc) Dial(ctx context.Context, addr string, password string) (Client, error) {
	return d(ctx, addr, password)
}

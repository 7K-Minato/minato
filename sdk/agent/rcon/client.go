package rcon

import "context"

type Client interface {
	Command(ctx context.Context, command string) (string, error)
	Close() error
}

type Dialer interface {
	Dial(ctx context.Context, addr string, password string) (Client, error)
}

// MinecraftDialer implements RCON dialer for Minecraft servers.
type MinecraftDialer struct{}

func (d *MinecraftDialer) Dial(ctx context.Context, addr, password string) (Client, error) {
	return NewMinecraftRCONClient(ctx, addr, password)
}

// SourceDialer implements RCON dialer for Source engine servers (CS2, etc.).
type SourceDialer struct{}

func (d *SourceDialer) Dial(ctx context.Context, addr, password string) (Client, error) {
	return NewSourceRCONClient(ctx, addr, password)
}

// PalworldDialer implements RCON dialer for Palworld servers.
type PalworldDialer struct{}

func (d *PalworldDialer) Dial(ctx context.Context, addr, password string) (Client, error) {
	return NewSourceRCONClient(ctx, addr, password)
}

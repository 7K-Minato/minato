package rcon

import "context"

type MinecraftClient struct {
	client Client
}

func NewMinecraftClient(client Client) *MinecraftClient {
	return &MinecraftClient{client: client}
}

func (m *MinecraftClient) Command(ctx context.Context, command string) (string, error) {
	return m.client.Command(ctx, command)
}

func (m *MinecraftClient) Close() error {
	return m.client.Close()
}

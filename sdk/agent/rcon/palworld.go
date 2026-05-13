package rcon

import "context"

type PalworldClient struct {
	client Client
}

func NewPalworldClient(client Client) *PalworldClient {
	return &PalworldClient{client: client}
}

func (p *PalworldClient) Command(ctx context.Context, command string) (string, error) {
	return p.client.Command(ctx, command)
}

func (p *PalworldClient) Close() error {
	return p.client.Close()
}

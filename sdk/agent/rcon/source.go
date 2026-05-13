package rcon

import "context"

type SourceClient struct {
	client Client
}

func NewSourceClient(client Client) *SourceClient {
	return &SourceClient{client: client}
}

func (s *SourceClient) Command(ctx context.Context, command string) (string, error) {
	return s.client.Command(ctx, command)
}

func (s *SourceClient) Close() error {
	return s.client.Close()
}

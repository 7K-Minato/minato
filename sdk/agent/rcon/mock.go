package rcon

import "context"

type MockClient struct {
	Commands []string
	Response string
	Err      error
}

func (m *MockClient) Command(ctx context.Context, command string) (string, error) {
	m.Commands = append(m.Commands, command)
	if m.Err != nil {
		return "", m.Err
	}
	return m.Response, nil
}

func (m *MockClient) Close() error {
	return nil
}

type MockDialer struct {
	Client Client
	Err    error
}

func (m *MockDialer) Dial(ctx context.Context, addr string, password string) (Client, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	return m.Client, nil
}

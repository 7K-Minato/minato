package server

import (
	"context"
	"testing"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
)

type noopAgent struct{}

func (n *noopAgent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	return &agentv1.InfoResponse{}, nil
}

func (n *noopAgent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	return &agentv1.HealthResponse{}, nil
}

func (n *noopAgent) PrepareShutdown(ctx context.Context, req *agentv1.ShutdownRequest) (*agentv1.ShutdownResponse, error) {
	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (n *noopAgent) GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error) {
	return &agentv1.PlayersResponse{}, nil
}

func (n *noopAgent) ExecuteAction(ctx context.Context, req *agentv1.ExecuteActionRequest) (*agentv1.ExecuteActionResponse, error) {
	return &agentv1.ExecuteActionResponse{}, nil
}

func (n *noopAgent) Console(stream agentv1.Agent_ConsoleServer) error {
	return nil
}

func TestServeDefaults(t *testing.T) {
	server, err := Serve(&noopAgent{}, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if server == nil {
		t.Fatalf("expected server")
	}
	_ = server.Shutdown(context.Background())
}

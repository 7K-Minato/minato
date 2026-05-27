package main

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/protobuf/types/known/anypb"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
	"github.com/7k-group/minato/sdk/agent/rcon"
	"github.com/7k-group/minato/sdk/agent/server"
)

// cs2Agent implements the Minato agent interface for Counter-Strike 2.
// This is a stub implementation - full implementation requires a working
// CS2 dedicated server with RCON support.
type cs2Agent struct {
	name       string
	version    string
	rconClient rcon.Client
}

func main() {
	password := os.Getenv("MINATO_RCON_PASSWORD")

	var client rcon.Client
	if password != "" {
		// TODO: Use proper dialer to create Source RCON client
		// client = rcon.NewSourceClient(client)
		_ = password
	}

	agent := &cs2Agent{
		name:       "minato-cs2",
		version:    "0.1.0",
		rconClient: client,
	}

	_, err := server.Serve(agent, server.Options{})
	if err != nil {
		panic(err)
	}

	select {}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func getEnvIntOrDefault(key string, defaultVal int) int {
	// Simplified - would need proper parsing
	return defaultVal
}

func (a *cs2Agent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	actions := []*agentv1.ActionSchema{
		{Name: "restart", Description: "Restart the server", Params: map[string]*agentv1.ParamSchema{}},
		{Name: "change-map", Description: "Change the current map", Params: map[string]*agentv1.ParamSchema{
			"map": {Type: "string", Required: true},
		}},
		{Name: "send-message", Description: "Send a message to all players", Params: map[string]*agentv1.ParamSchema{
			"message": {Type: "string", Required: true},
		}},
		{Name: "kick-player", Description: "Kick a player", Params: map[string]*agentv1.ParamSchema{
			"player": {Type: "string", Required: true},
			"reason": {Type: "string", Required: false},
		}},
	}

	return &agentv1.InfoResponse{
		Name:    a.name,
		Version: a.version,
		Actions: actions,
		Metrics: []*agentv1.MetricDescriptor{
			{Name: "cs2_tickrate", Description: "Server tickrate", Unit: "hz"},
		},
	}, nil
}

func (a *cs2Agent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	if a.rconClient == nil {
		return &agentv1.HealthResponse{Ready: true, Message: "no rcon configured"}, nil
	}
	return &agentv1.HealthResponse{Ready: true, Message: "healthy"}, nil
}

func (a *cs2Agent) PrepareShutdown(ctx context.Context, req *agentv1.ShutdownRequest) (*agentv1.ShutdownResponse, error) {
	if a.rconClient != nil {
		_, _ = a.rconClient.Command(ctx, "say Server shutting down...")
		_, _ = a.rconClient.Command(ctx, "quit")
	}
	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (a *cs2Agent) GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error) {
	return &agentv1.PlayersResponse{Online: 0, Capacity: 64}, nil
}

func (a *cs2Agent) ExecuteAction(ctx context.Context, req *agentv1.ExecuteActionRequest) (*agentv1.ExecuteActionResponse, error) {
	if a.rconClient == nil {
		return &agentv1.ExecuteActionResponse{State: agentv1.ActionState_ACTION_STATE_FAILED, Error: "rcon not configured"}, nil
	}

	var cmd string
	switch req.ActionName {
	case "restart":
		cmd = "quit"
	case "change-map":
		cmd = fmt.Sprintf("changelevel %s", req.Params["map"])
	case "send-message":
		cmd = fmt.Sprintf("say %s", req.Params["message"])
	case "kick-player":
		cmd = fmt.Sprintf("kickid %s", req.Params["player"])
	default:
		return &agentv1.ExecuteActionResponse{State: agentv1.ActionState_ACTION_STATE_REJECTED, Error: "unknown action"}, nil
	}

	output, err := a.rconClient.Command(ctx, cmd)
	if err != nil {
		return &agentv1.ExecuteActionResponse{State: agentv1.ActionState_ACTION_STATE_FAILED, Error: err.Error()}, nil
	}

	result, _ := anypb.New(&agentv1.ConsoleResponse{Response: output})
	return &agentv1.ExecuteActionResponse{State: agentv1.ActionState_ACTION_STATE_SUCCEEDED, Result: result}, nil
}

func (a *cs2Agent) Console(stream agentv1.Agent_ConsoleServer) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		_ = msg
		_ = stream.Send(&agentv1.ConsoleServerMessage{
			Payload: &agentv1.ConsoleServerMessage_Response{Response: &agentv1.ConsoleResponse{Response: "ok"}},
		})
	}
}

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

// palworldAgent implements the Minato agent interface for Palworld.
// This is a stub implementation - full implementation requires a working
// Palworld dedicated server with RCON support.
type palworldAgent struct {
	name       string
	version    string
	rconClient rcon.Client
}

func main() {
	password := os.Getenv("MINATO_RCON_PASSWORD")

	var client rcon.Client
	if password != "" {
		// TODO: Use proper dialer to create Palworld RCON client
		// client = rcon.NewPalworldClient(client)
		_ = password
	}

	agent := &palworldAgent{
		name:       "minato-palworld",
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
	return defaultVal
}

func (a *palworldAgent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	actions := []*agentv1.ActionSchema{
		{Name: "restart", Description: "Restart the server", Params: map[string]*agentv1.ParamSchema{}},
		{Name: "save-world", Description: "Save the world", Params: map[string]*agentv1.ParamSchema{}},
		{Name: "send-message", Description: "Send a message to all players", Params: map[string]*agentv1.ParamSchema{
			"message": {Type: "string", Required: true},
		}},
		{Name: "kick-player", Description: "Kick a player", Params: map[string]*agentv1.ParamSchema{
			"player": {Type: "string", Required: true},
		}},
		{Name: "ban-player", Description: "Ban a player", Params: map[string]*agentv1.ParamSchema{
			"player": {Type: "string", Required: true},
		}},
	}

	return &agentv1.InfoResponse{
		Name:    a.name,
		Version: a.version,
		Actions: actions,
		Metrics: []*agentv1.MetricDescriptor{
			{Name: "palworld_world_time", Description: "World time", Unit: "seconds"},
		},
	}, nil
}

func (a *palworldAgent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	if a.rconClient == nil {
		return &agentv1.HealthResponse{Ready: true, Message: "no rcon configured"}, nil
	}
	return &agentv1.HealthResponse{Ready: true, Message: "healthy"}, nil
}

func (a *palworldAgent) PrepareShutdown(ctx context.Context, req *agentv1.ShutdownRequest) (*agentv1.ShutdownResponse, error) {
	if a.rconClient != nil {
		_, _ = a.rconClient.Command(ctx, "Broadcast Server_shutting_down...")
		_, _ = a.rconClient.Command(ctx, "Save")
		_, _ = a.rconClient.Command(ctx, "Shutdown")
	}
	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (a *palworldAgent) GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error) {
	return &agentv1.PlayersResponse{Online: 0, Capacity: 32}, nil
}

func (a *palworldAgent) ExecuteAction(ctx context.Context, req *agentv1.ExecuteActionRequest) (*agentv1.ExecuteActionResponse, error) {
	if a.rconClient == nil {
		return &agentv1.ExecuteActionResponse{State: agentv1.ActionState_ACTION_STATE_FAILED, Error: "rcon not configured"}, nil
	}

	var cmd string
	switch req.ActionName {
	case "restart":
		cmd = "Shutdown"
	case "save-world":
		cmd = "Save"
	case "send-message":
		cmd = fmt.Sprintf("Broadcast %s", req.Params["message"])
	case "kick-player":
		cmd = fmt.Sprintf("KickPlayer %s", req.Params["player"])
	case "ban-player":
		cmd = fmt.Sprintf("BanPlayer %s", req.Params["player"])
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

func (a *palworldAgent) Console(stream agentv1.Agent_ConsoleServer) error {
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

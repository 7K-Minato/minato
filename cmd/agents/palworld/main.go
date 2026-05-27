package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"google.golang.org/protobuf/types/known/anypb"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
	"github.com/7k-group/minato/sdk/agent/rcon"
	"github.com/7k-group/minato/sdk/agent/server"
)

// palworldAgent implements the Minato agent interface for Palworld.
type palworldAgent struct {
	name       string
	version    string
	rconClient rcon.Client
}

func main() {
	host := os.Getenv("RCON_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("RCON_PORT")
	if port == "" {
		port = "25575"
	}
	password := os.Getenv("RCON_PASSWORD")

	var client rcon.Client
	if password != "" {
		addr := fmt.Sprintf("%s:%s", host, port)
		var err error
		client, err = rcon.NewSourceRCONClient(context.Background(), addr, password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to Palworld RCON: %v\n", err)
			os.Exit(1)
		}
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
	_, err := a.rconClient.Command(ctx, "Info")
	if err != nil {
		return &agentv1.HealthResponse{Ready: false, Message: err.Error()}, nil
	}
	return &agentv1.HealthResponse{Ready: true, Message: "healthy"}, nil
}

func (a *palworldAgent) PrepareShutdown(
	ctx context.Context,
	req *agentv1.ShutdownRequest,
) (*agentv1.ShutdownResponse, error) {
	if a.rconClient != nil {
		_, _ = a.rconClient.Command(ctx, "Broadcast Server_shutting_down...")
		_, _ = a.rconClient.Command(ctx, "Save")
		_, _ = a.rconClient.Command(ctx, "Shutdown")
	}
	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (a *palworldAgent) GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error) {
	if a.rconClient == nil {
		return &agentv1.PlayersResponse{Online: 0, Capacity: 32}, nil
	}

	output, err := a.rconClient.Command(ctx, "ShowPlayers")
	if err != nil {
		return &agentv1.PlayersResponse{Online: 0, Capacity: 32}, nil
	}

	online := parsePalworldPlayerCount(output)
	players := parsePalworldPlayerList(output)

	return &agentv1.PlayersResponse{
		Online:   int32(online),
		Capacity: 32,
		Players:  players,
	}, nil
}

// parsePalworldPlayerCount parses the output of Palworld's "ShowPlayers" command.
func parsePalworldPlayerCount(output string) int {
	count := 0
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "name") {
			continue
		}
		count++
	}
	return count
}

// parsePalworldPlayerList parses player names from Palworld ShowPlayers output.
func parsePalworldPlayerList(output string) []*agentv1.Player {
	var players []*agentv1.Player
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, "name") {
			continue
		}
		fields := strings.Split(line, ",")
		if len(fields) > 0 && fields[0] != "" {
			players = append(players, &agentv1.Player{Name: fields[0]})
		}
	}
	return players
}

func (a *palworldAgent) ExecuteAction(
	ctx context.Context,
	req *agentv1.ExecuteActionRequest,
) (*agentv1.ExecuteActionResponse, error) {
	if a.rconClient == nil {
		return &agentv1.ExecuteActionResponse{
			State: agentv1.ActionState_ACTION_STATE_FAILED,
			Error: "rcon not configured",
		}, nil
	}

	var cmd string
	switch req.ActionName {
	case "restart":
		cmd = "Shutdown"
	case "save-world":
		cmd = "Save"
	case "send-message":
		msg := strings.ReplaceAll(req.Params["message"], " ", "_")
		cmd = fmt.Sprintf("Broadcast %s", msg)
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
		if a.rconClient != nil && msg.GetCommand() != nil {
			output, err := a.rconClient.Command(stream.Context(), msg.GetCommand().RconCommand)
			if err != nil {
				_ = stream.Send(&agentv1.ConsoleServerMessage{
					Payload: &agentv1.ConsoleServerMessage_Error{Error: &agentv1.ConsoleError{Message: err.Error()}},
				})
				continue
			}
			_ = stream.Send(&agentv1.ConsoleServerMessage{
				Payload: &agentv1.ConsoleServerMessage_Response{Response: &agentv1.ConsoleResponse{Response: output}},
			})
		} else {
			_ = stream.Send(&agentv1.ConsoleServerMessage{
				Payload: &agentv1.ConsoleServerMessage_Response{Response: &agentv1.ConsoleResponse{Response: "ok"}},
			})
		}
	}
}

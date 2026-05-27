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

// cs2Agent implements the Minato agent interface for Counter-Strike 2.
type cs2Agent struct {
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
		port = "27015"
	}
	password := os.Getenv("RCON_PASSWORD")

	var client rcon.Client
	if password != "" {
		addr := fmt.Sprintf("%s:%s", host, port)
		var err error
		client, err = rcon.NewSourceRCONClient(context.Background(), addr, password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to CS2 RCON: %v\n", err)
			os.Exit(1)
		}
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
		{Name: "ban-player", Description: "Ban a player", Params: map[string]*agentv1.ParamSchema{
			"player":   {Type: "string", Required: true},
			"duration": {Type: "string", Required: false},
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
	_, err := a.rconClient.Command(ctx, "status")
	if err != nil {
		return &agentv1.HealthResponse{Ready: false, Message: err.Error()}, nil
	}
	return &agentv1.HealthResponse{Ready: true, Message: "healthy"}, nil
}

func (a *cs2Agent) PrepareShutdown(
	ctx context.Context,
	req *agentv1.ShutdownRequest,
) (*agentv1.ShutdownResponse, error) {
	if a.rconClient != nil {
		_, _ = a.rconClient.Command(ctx, "say Server shutting down...")
		_, _ = a.rconClient.Command(ctx, "quit")
	}
	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (a *cs2Agent) GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error) {
	if a.rconClient == nil {
		return &agentv1.PlayersResponse{Online: 0, Capacity: 64}, nil
	}

	output, err := a.rconClient.Command(ctx, "status")
	if err != nil {
		return &agentv1.PlayersResponse{Online: 0, Capacity: 64}, nil
	}

	online, capacity := parseCS2PlayerCount(output)
	players := parseCS2PlayerList(output)

	return &agentv1.PlayersResponse{
		Online:   int32(online),
		Capacity: int32(capacity),
		Players:  players,
	}, nil
}

func (a *cs2Agent) ExecuteAction(
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
		cmd = "quit"
	case "change-map":
		cmd = fmt.Sprintf("changelevel %s", req.Params["map"])
	case "send-message":
		cmd = fmt.Sprintf("say %s", req.Params["message"])
	case "kick-player":
		player := req.Params["player"]
		reason := req.Params["reason"]
		if reason != "" {
			cmd = fmt.Sprintf("kickid %s %s", player, reason)
		} else {
			cmd = fmt.Sprintf("kickid %s", player)
		}
	case "ban-player":
		player := req.Params["player"]
		duration := req.Params["duration"]
		if duration != "" {
			cmd = fmt.Sprintf("banid %s %s", duration, player)
		} else {
			cmd = fmt.Sprintf("banid %s", player)
		}
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

// parseCS2PlayerCount parses the output of CS2's "status" command.
func parseCS2PlayerCount(output string) (online, capacity int) {
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "players") {
			var o, c int
			if _, err := fmt.Sscanf(line, "players : %d (%d max)", &o, &c); err == nil {
				return o, c
			}
		}
		if strings.HasPrefix(line, "maxplayers") {
			var c int
			if _, err := fmt.Sscanf(line, "maxplayers : %d", &c); err == nil {
				capacity = c
			}
		}
	}
	return online, capacity
}

// parseCS2PlayerList parses player names from CS2 status output.
func parseCS2PlayerList(output string) []*agentv1.Player {
	var players []*agentv1.Player
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") && !strings.Contains(line, "userid") {
			fields := strings.Fields(line)
			if len(fields) >= 3 {
				players = append(players, &agentv1.Player{
					Name: fields[2],
					Id:   fields[1],
				})
			}
		}
	}
	return players
}

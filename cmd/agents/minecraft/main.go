package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"google.golang.org/protobuf/types/known/anypb"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
	"github.com/7k-group/minato/sdk/agent/rcon"
	"github.com/7k-group/minato/sdk/agent/server"
)

type minecraftAgent struct {
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
		client, err = rcon.NewMinecraftRCONClient(context.Background(), addr, password)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to connect to Minecraft RCON: %v\n", err)
			os.Exit(1)
		}
	}

	agent := &minecraftAgent{
		name:       "minato-minecraft",
		version:    "0.1.0",
		rconClient: client,
	}

	_, err := server.Serve(agent, server.Options{})
	if err != nil {
		panic(err)
	}

	select {}
}

func (a *minecraftAgent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	actions := []*agentv1.ActionSchema{
		{
			Name:        "restart",
			Description: "Gracefully restart the server",
			Params:      map[string]*agentv1.ParamSchema{},
		},
		{
			Name:        "save-world",
			Description: "Save the game world",
			Params:      map[string]*agentv1.ParamSchema{},
		},
		{
			Name:        "send-message",
			Description: "Broadcast a message to all players",
			Params: map[string]*agentv1.ParamSchema{
				"message": {Type: "string", Required: true, Description: "Message to send"},
			},
		},
		{
			Name:        "kick-player",
			Description: "Kick a player from the server",
			Params: map[string]*agentv1.ParamSchema{
				"player": {Type: "string", Required: true, Description: "Player name"},
				"reason": {Type: "string", Required: false, Description: "Kick reason"},
			},
		},
		{
			Name:        "op-player",
			Description: "Give operator status to a player",
			Params: map[string]*agentv1.ParamSchema{
				"player": {Type: "string", Required: true, Description: "Player name"},
			},
		},
		{
			Name:        "deop-player",
			Description: "Remove operator status from a player",
			Params: map[string]*agentv1.ParamSchema{
				"player": {Type: "string", Required: true, Description: "Player name"},
			},
		},
		{
			Name:        "whitelist-add",
			Description: "Add a player to the whitelist",
			Params: map[string]*agentv1.ParamSchema{
				"player": {Type: "string", Required: true, Description: "Player name"},
			},
		},
		{
			Name:        "whitelist-remove",
			Description: "Remove a player from the whitelist",
			Params: map[string]*agentv1.ParamSchema{
				"player": {Type: "string", Required: true, Description: "Player name"},
			},
		},
	}

	return &agentv1.InfoResponse{
		Name:    a.name,
		Version: a.version,
		Actions: actions,
		Metrics: []*agentv1.MetricDescriptor{
			{Name: "minecraft_tps", Description: "Server TPS", Unit: "tps"},
		},
	}, nil
}

func (a *minecraftAgent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	if a.rconClient == nil {
		return &agentv1.HealthResponse{Ready: true, Message: "no rcon configured"}, nil
	}

	_, err := a.rconClient.Command(ctx, "list")
	if err != nil {
		return &agentv1.HealthResponse{Ready: false, Message: err.Error()}, nil
	}

	return &agentv1.HealthResponse{Ready: true, Message: "healthy"}, nil
}

func (a *minecraftAgent) PrepareShutdown(
	ctx context.Context,
	req *agentv1.ShutdownRequest,
) (*agentv1.ShutdownResponse, error) {
	if a.rconClient == nil {
		return &agentv1.ShutdownResponse{Success: true}, nil
	}

	// Broadcast shutdown warning
	if req.DrainReason != "" {
		_, _ = a.rconClient.Command(ctx, fmt.Sprintf("say Server shutting down: %s", req.DrainReason))
	} else {
		_, _ = a.rconClient.Command(ctx, "say Server shutting down...")
	}

	// Save the world
	_, _ = a.rconClient.Command(ctx, "save-all")

	// Stop the server
	_, _ = a.rconClient.Command(ctx, "stop")

	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (a *minecraftAgent) GetPlayers(
	ctx context.Context,
	req *agentv1.PlayersRequest,
) (*agentv1.PlayersResponse, error) {
	if a.rconClient == nil {
		return &agentv1.PlayersResponse{Online: 0, Capacity: 20}, nil
	}

	output, err := a.rconClient.Command(ctx, "list")
	if err != nil {
		return &agentv1.PlayersResponse{Online: 0, Capacity: 20}, nil
	}

	online, capacity := parseMinecraftList(output)

	return &agentv1.PlayersResponse{
		Online:   int32(online),
		Capacity: int32(capacity),
	}, nil
}

func (a *minecraftAgent) ExecuteAction(
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
		cmd = "restart"
	case "save-world":
		cmd = "save-all"
	case "send-message":
		msg := req.Params["message"]
		cmd = fmt.Sprintf("say %s", msg)
	case "kick-player":
		player := req.Params["player"]
		reason := req.Params["reason"]
		if reason != "" {
			cmd = fmt.Sprintf("kick %s %s", player, reason)
		} else {
			cmd = fmt.Sprintf("kick %s", player)
		}
	case "op-player":
		cmd = fmt.Sprintf("op %s", req.Params["player"])
	case "deop-player":
		cmd = fmt.Sprintf("deop %s", req.Params["player"])
	case "whitelist-add":
		cmd = fmt.Sprintf("whitelist add %s", req.Params["player"])
	case "whitelist-remove":
		cmd = fmt.Sprintf("whitelist remove %s", req.Params["player"])
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

func (a *minecraftAgent) Console(stream agentv1.Agent_ConsoleServer) error {
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

// parseMinecraftList parses the output of the Minecraft "list" command.
// Expected format: "There are X of a max Y players online: player1, player2"
func parseMinecraftList(output string) (online, capacity int) {
	parts := strings.Split(output, ":")
	if len(parts) < 1 {
		return 0, 20
	}

	header := parts[0]
	// Try to extract numbers from "There are X of a max Y players online"
	var x, y int
	if _, err := fmt.Sscanf(header, "There are %d of a max %d players online", &x, &y); err == nil {
		return x, y
	}

	// Fallback: try to find numbers in the string
	fields := strings.Fields(header)
	for i, p := range fields {
		if p == "are" && i+1 < len(fields) {
			if v, err := strconv.Atoi(fields[i+1]); err == nil {
				x = v
			}
		}
		if p == "max" && i+1 < len(fields) {
			if v, err := strconv.Atoi(fields[i+1]); err == nil {
				y = v
			}
		}
	}
	if x > 0 || y > 0 {
		return x, y
	}

	return 0, 20
}

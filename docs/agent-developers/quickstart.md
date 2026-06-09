# Agent SDK Quickstart

This guide shows how to build a minimal agent in ~50 lines.

```go
package main

import (
  "context"

  agentv1 "github.com/7k-minato/minato/api/agent/v1/minato/agent/v1"
  "github.com/7k-minato/minato/sdk/agent/server"
)

type agent struct{}

func main() {
  _, _ = server.Serve(&agent{}, server.Options{})
  select {}
}

func (a *agent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
  return &agentv1.InfoResponse{Name: "my-agent", Version: "0.1.0"}, nil
}

func (a *agent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
  return &agentv1.HealthResponse{Ready: true, Message: "ok"}, nil
}

func (a *agent) PrepareShutdown(ctx context.Context, req *agentv1.ShutdownRequest) (*agentv1.ShutdownResponse, error) {
  return &agentv1.ShutdownResponse{Success: true}, nil
}

func (a *agent) GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error) {
  return &agentv1.PlayersResponse{Online: 0, Capacity: 0}, nil
}

func (a *agent) ExecuteAction(ctx context.Context, req *agentv1.ExecuteActionRequest) (*agentv1.ExecuteActionResponse, error) {
  return &agentv1.ExecuteActionResponse{State: agentv1.ActionState_ACTION_STATE_SUCCEEDED}, nil
}

func (a *agent) Console(stream agentv1.Agent_ConsoleServer) error {
  return nil
}
```

package main

import (
	"context"
	"os"
	"slices"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/anypb"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
	"github.com/7k-group/minato/sdk/agent/actions"
	"github.com/7k-group/minato/sdk/agent/server"
)

type genericAgent struct {
	name    string
	version string
	catalog actions.Catalog
	runtime actions.Runtime
}

type noopRuntime struct{}

func (n *noopRuntime) RCON(ctx context.Context, command string) (string, error) { return "", nil }
func (n *noopRuntime) Exec(ctx context.Context, command string, args []string) (string, error) {
	return "", nil
}
func (n *noopRuntime) HTTP(ctx context.Context, method string, url string, body string) (string, error) {
	return "", nil
}
func (n *noopRuntime) Signal(ctx context.Context, target string, signal string) error { return nil }
func (n *noopRuntime) Sleep(ctx context.Context, duration time.Duration) error        { return nil }

func main() {
	configPath := os.Getenv("minato_AGENT_CONFIG_PATH")
	inline := os.Getenv("minato_AGENT_CONFIG_INLINE")

	var catalog actions.Catalog
	var err error
	if configPath != "" {
		catalog, err = actions.LoadCatalogFromFile(configPath)
	} else if inline != "" {
		catalog, err = actions.LoadCatalogFromBytes([]byte(inline))
	} else {
		catalog = actions.Catalog{}
	}
	if err != nil {
		panic(err)
	}

	if len(catalog.Actions) == 0 {
		catalog = actions.CatalogFromEnv()
	}

	agent := &genericAgent{name: "minato-generic", version: "0.1.0", catalog: catalog, runtime: &noopRuntime{}}
	_, err = server.Serve(agent, server.Options{})
	if err != nil {
		panic(err)
	}

	select {}
}

func (g *genericAgent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	actionList := make([]*agentv1.ActionSchema, 0, len(g.catalog.Actions))
	for _, item := range g.catalog.Actions {
		params := map[string]*agentv1.ParamSchema{}
		for key, schema := range item.Params {
			params[key] = &agentv1.ParamSchema{
				Type:        schema.Type,
				Required:    schema.Required,
				Description: schema.Description,
				Default:     schema.Default,
			}
		}
		actionList = append(actionList, &agentv1.ActionSchema{
			Name:        item.Name,
			Description: item.Description,
			Params:      params,
		})
	}

	return &agentv1.InfoResponse{
		Name:    g.name,
		Version: g.version,
		Actions: actionList,
		Metrics: []*agentv1.MetricDescriptor{},
	}, nil
}

func (g *genericAgent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	return &agentv1.HealthResponse{Ready: true, Message: "ok", Details: map[string]string{}}, nil
}

func (g *genericAgent) PrepareShutdown(
	ctx context.Context,
	req *agentv1.ShutdownRequest,
) (*agentv1.ShutdownResponse, error) {
	return &agentv1.ShutdownResponse{Success: true, Error: ""}, nil
}

func (g *genericAgent) GetPlayers(
	ctx context.Context,
	req *agentv1.PlayersRequest,
) (*agentv1.PlayersResponse, error) {
	return &agentv1.PlayersResponse{Online: 0, Capacity: 0, Players: nil}, nil
}

func (g *genericAgent) ExecuteAction(
	ctx context.Context,
	req *agentv1.ExecuteActionRequest,
) (*agentv1.ExecuteActionResponse, error) {
	if strings.TrimSpace(req.ActionName) == "" {
		return &agentv1.ExecuteActionResponse{
			State: agentv1.ActionState_ACTION_STATE_REJECTED,
			Error: "action_name required",
		}, nil
	}
	action, ok := g.catalog.FindAction(req.ActionName)
	if !ok {
		return &agentv1.ExecuteActionResponse{
			State: agentv1.ActionState_ACTION_STATE_REJECTED,
			Error: "unknown action",
		}, nil
	}
	if g.runtime == nil {
		return &agentv1.ExecuteActionResponse{
			State: agentv1.ActionState_ACTION_STATE_FAILED,
			Error: "runtime not configured",
		}, nil
	}
	result, err := g.executeAction(ctx, action, req.Params)
	if err != nil {
		return &agentv1.ExecuteActionResponse{
			State: agentv1.ActionState_ACTION_STATE_FAILED,
			Error: err.Error(),
		}, nil
	}
	return &agentv1.ExecuteActionResponse{
		State:  agentv1.ActionState_ACTION_STATE_SUCCEEDED,
		Result: result,
	}, nil
}

func (g *genericAgent) Console(stream agentv1.Agent_ConsoleServer) error {
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		payload := &agentv1.ConsoleServerMessage{
			Payload: &agentv1.ConsoleServerMessage_Response{
				Response: &agentv1.ConsoleResponse{Response: "ok"},
			},
		}
		_ = msg
		if err := stream.Send(payload); err != nil {
			return err
		}
	}
}

func (g *genericAgent) executeAction(
	ctx context.Context,
	action actions.ActionDefinition,
	params map[string]string,
) (*anypb.Any, error) {
	result, err := actions.Execute(ctx, action, params, g.runtime)
	if err != nil {
		return nil, err
	}
	return anypb.New(&agentv1.ConsoleResponse{
		Response: strings.Join(mapKeys(result.Outputs), ","),
	})
}

func mapKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

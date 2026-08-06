// Package controlplane is the official Go client (sdk-go) for the minato
// control plane REST API. HTTP calls are generated from api/openapi.yaml
// (see gen/); this package exposes ergonomic value types on top.
package controlplane

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/7k-minato/minato/sdk/controlplane/gen"
)

// Client talks to a single minato control plane.
type Client struct {
	baseURL string
	inner   *gen.ClientWithResponses
}

// NewClient creates a control plane client. apiKey may be empty when the
// control plane runs with AUTH_MODE=none.
func NewClient(baseURL, apiKey string, timeout time.Duration) (*Client, error) {
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("controlplane: invalid base URL %q: %w", baseURL, err)
	}
	inner, err := gen.NewClientWithResponses(
		strings.TrimSuffix(baseURL, "/"),
		gen.WithHTTPClient(&http.Client{Timeout: timeout}),
		gen.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Accept", "application/json")
			if apiKey != "" {
				req.Header.Set("Authorization", "Bearer "+apiKey)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: baseURL, inner: inner}, nil
}

// BaseURL returns the control plane base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// APIError is a non-2xx response from the control plane.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("controlplane: status %d: %s", e.Status, e.Message)
}

func apiError(status int, body []byte) error {
	msg := string(body)
	var env struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &env) == nil && env.Error != "" {
		msg = env.Error
	}
	return &APIError{Status: status, Message: msg}
}

// ObjectMeta is a subset of Kubernetes object metadata.
type ObjectMeta struct {
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// GameServerSpec is the desired state of a GameServer.
type GameServerSpec struct {
	Profile string            `json:"profile"`
	Env     map[string]string `json:"env,omitempty"`
	Storage struct {
		Size         string `json:"size,omitempty"`
		StorageClass string `json:"storageClass,omitempty"`
		// SnapshotRef restores the server from a GameSnapshot.
		SnapshotRef *SnapshotRef `json:"snapshotRef,omitempty"`
	} `json:"storage,omitempty"`
	Lifecycle struct {
		IdleTimeoutSeconds int32 `json:"idleTimeoutSeconds,omitempty"`
		AutoStart          bool  `json:"autoStart,omitempty"`
	} `json:"lifecycle,omitempty"`
	// LoadBalancerIP pins the external IP of the game service.
	LoadBalancerIP string `json:"loadBalancerIP,omitempty"`
}

// SnapshotRef references a snapshot to restore a GameServer from.
type SnapshotRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// Endpoint is a published network endpoint of a GameServer.
type Endpoint struct {
	Name    string `json:"name"`
	Address string `json:"address"`
	Port    int32  `json:"port"`
}

// GameServerStatus is the observed state of a GameServer.
type GameServerStatus struct {
	State          string     `json:"state,omitempty"`
	AgentVersion   string     `json:"agentVersion,omitempty"`
	Players        int32      `json:"players,omitempty"`
	PlayerCapacity int32      `json:"playerCapacity,omitempty"`
	Endpoints      []Endpoint `json:"endpoints,omitempty"`
}

// GameServer is a managed game server instance.
type GameServer struct {
	APIVersion string           `json:"apiVersion,omitempty"`
	Kind       string           `json:"kind,omitempty"`
	Metadata   ObjectMeta       `json:"metadata"`
	Spec       GameServerSpec   `json:"spec"`
	Status     GameServerStatus `json:"status,omitempty"`
}

// SFTPInfo is the SFTP connection info for a game server.
type SFTPInfo struct {
	Host     string `json:"host"`
	Port     int32  `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

// ActionParam describes one parameter of an action.
type ActionParam struct {
	Type     string `json:"type,omitempty"`
	Required bool   `json:"required,omitempty"`
	Default  string `json:"default,omitempty"`
}

// Action is an executable action declared by a GameProfile.
type Action struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Concurrency string                 `json:"concurrency,omitempty"`
	Timeout     string                 `json:"timeout,omitempty"`
	Params      map[string]ActionParam `json:"params,omitempty"`
}

// GameProfile is a game template.
type GameProfile struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		DisplayName string   `json:"displayName"`
		Image       string   `json:"image,omitempty"`
		Actions     []Action `json:"actions,omitempty"`
	} `json:"spec"`
}

// GameServerFleet manages a set of identical GameServers.
type GameServerFleet struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		Profile  string `json:"profile,omitempty"`
		Replicas int32  `json:"replicas,omitempty"`
	} `json:"spec"`
	Status struct {
		Replicas          int32 `json:"replicas,omitempty"`
		ReadyReplicas     int32 `json:"readyReplicas,omitempty"`
		UpdatedReplicas   int32 `json:"updatedReplicas,omitempty"`
		AvailableReplicas int32 `json:"availableReplicas,omitempty"`
	} `json:"status"`
}

// ActionExecutionRef identifies a started action execution.
type ActionExecutionRef struct {
	Name string `json:"name"`
}

// ActionExecution is the status of a dispatched action.
type ActionExecution struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		ActionName string            `json:"actionName"`
		Caller     string            `json:"caller"`
		Params     map[string]string `json:"params,omitempty"`
	} `json:"spec"`
	Status struct {
		State       string     `json:"state,omitempty"`
		Result      string     `json:"result,omitempty"`
		Error       string     `json:"error,omitempty"`
		StartedAt   *time.Time `json:"startedAt,omitempty"`
		CompletedAt *time.Time `json:"completedAt,omitempty"`
	} `json:"status"`
}

// GameSnapshot is a point-in-time copy of a GameServer's data.
type GameSnapshot struct {
	Metadata ObjectMeta `json:"metadata"`
	Spec     struct {
		GameServerRef string `json:"gameServerRef"`
		Schedule      string `json:"schedule,omitempty"`
	} `json:"spec"`
	Status struct {
		State   string     `json:"state,omitempty"`
		ReadyAt *time.Time `json:"readyAt,omitempty"`
		Size    string     `json:"size,omitempty"`
	} `json:"status,omitempty"`
}

// Conversions between SDK types and generated models.

func toGenGameServer(gs *GameServer) gen.GameServer {
	out := gen.GameServer{
		Metadata: &gen.ObjectMeta{
			Name:      &gs.Metadata.Name,
			Namespace: ptrOrNil(gs.Metadata.Namespace),
		},
		Spec: &gen.GameServerSpec{
			Profile: &gs.Spec.Profile,
			Env:     mapOrNil(gs.Spec.Env),
		},
	}
	if gs.APIVersion != "" {
		out.ApiVersion = &gs.APIVersion
	}
	if gs.Kind != "" {
		out.Kind = &gs.Kind
	}
	if gs.Metadata.Labels != nil {
		out.Metadata.Labels = &gs.Metadata.Labels
	}
	if gs.Spec.Storage.Size != "" || gs.Spec.Storage.StorageClass != "" || gs.Spec.Storage.SnapshotRef != nil {
		out.Spec.Storage = &gen.StorageSpec{
			Size:         ptrOrNil(gs.Spec.Storage.Size),
			StorageClass: ptrOrNil(gs.Spec.Storage.StorageClass),
		}
		if gs.Spec.Storage.SnapshotRef != nil {
			out.Spec.Storage.SnapshotRef = &struct {
				Name      *string `json:"name,omitempty"`
				Namespace *string `json:"namespace,omitempty"`
			}{
				Name:      &gs.Spec.Storage.SnapshotRef.Name,
				Namespace: ptrOrNil(gs.Spec.Storage.SnapshotRef.Namespace),
			}
		}
	}
	if gs.Spec.Lifecycle.AutoStart || gs.Spec.Lifecycle.IdleTimeoutSeconds != 0 {
		idle := int(gs.Spec.Lifecycle.IdleTimeoutSeconds)
		out.Spec.Lifecycle = &struct {
			AutoStart          *bool `json:"autoStart,omitempty"`
			IdleTimeoutSeconds *int  `json:"idleTimeoutSeconds,omitempty"`
		}{
			AutoStart:          &gs.Spec.Lifecycle.AutoStart,
			IdleTimeoutSeconds: &idle,
		}
	}
	out.Spec.LoadBalancerIP = ptrOrNil(gs.Spec.LoadBalancerIP)
	return out
}

func fromGenGameServer(in *gen.GameServer) GameServer {
	out := GameServer{}
	if in == nil {
		return out
	}
	out.APIVersion = deref(in.ApiVersion)
	out.Kind = deref(in.Kind)
	if in.Metadata != nil {
		out.Metadata.Name = deref(in.Metadata.Name)
		out.Metadata.Namespace = deref(in.Metadata.Namespace)
		if in.Metadata.Labels != nil {
			out.Metadata.Labels = *in.Metadata.Labels
		}
	}
	if in.Spec != nil {
		out.Spec.Profile = deref(in.Spec.Profile)
		if in.Spec.Env != nil {
			out.Spec.Env = *in.Spec.Env
		}
		if in.Spec.Storage != nil {
			out.Spec.Storage.Size = deref(in.Spec.Storage.Size)
			out.Spec.Storage.StorageClass = deref(in.Spec.Storage.StorageClass)
			if in.Spec.Storage.SnapshotRef != nil {
				out.Spec.Storage.SnapshotRef = &SnapshotRef{
					Name:      deref(in.Spec.Storage.SnapshotRef.Name),
					Namespace: deref(in.Spec.Storage.SnapshotRef.Namespace),
				}
			}
		}
		if in.Spec.Lifecycle != nil {
			if in.Spec.Lifecycle.AutoStart != nil {
				out.Spec.Lifecycle.AutoStart = *in.Spec.Lifecycle.AutoStart
			}
			if in.Spec.Lifecycle.IdleTimeoutSeconds != nil {
				out.Spec.Lifecycle.IdleTimeoutSeconds = int32(*in.Spec.Lifecycle.IdleTimeoutSeconds)
			}
		}
		out.Spec.LoadBalancerIP = deref(in.Spec.LoadBalancerIP)
	}
	if in.Status != nil {
		out.Status.State = deref((*string)(in.Status.State))
		out.Status.AgentVersion = deref(in.Status.AgentVersion)
		if in.Status.Players != nil {
			out.Status.Players = int32(*in.Status.Players)
		}
		if in.Status.PlayerCapacity != nil {
			out.Status.PlayerCapacity = int32(*in.Status.PlayerCapacity)
		}
		if in.Status.Endpoints != nil {
			for _, e := range *in.Status.Endpoints {
				ep := Endpoint{
					Name:    deref(e.Name),
					Address: deref(e.Address),
				}
				if e.Port != nil {
					ep.Port = int32(*e.Port)
				}
				out.Status.Endpoints = append(out.Status.Endpoints, ep)
			}
		}
	}
	return out
}

func fromGenAction(in gen.Action) Action {
	out := Action{
		Name:        deref(in.Name),
		Description: deref(in.Description),
		Concurrency: deref((*string)(in.Concurrency)),
		Timeout:     deref(in.Timeout),
	}
	if in.Params != nil {
		out.Params = map[string]ActionParam{}
		for k, p := range *in.Params {
			out.Params[k] = ActionParam{
				Type:     deref((*string)(p.Type)),
				Required: p.Required != nil && *p.Required,
				Default:  deref(p.Default),
			}
		}
	}
	return out
}

func fromGenProfile(in gen.GameProfile) GameProfile {
	out := GameProfile{}
	if in.Metadata != nil {
		out.Metadata.Name = deref(in.Metadata.Name)
		out.Metadata.Namespace = deref(in.Metadata.Namespace)
		if in.Metadata.Labels != nil {
			out.Metadata.Labels = *in.Metadata.Labels
		}
	}
	if in.Spec != nil {
		out.Spec.DisplayName = deref(in.Spec.DisplayName)
		out.Spec.Image = deref(in.Spec.Image)
		if in.Spec.Actions != nil {
			for _, a := range *in.Spec.Actions {
				out.Spec.Actions = append(out.Spec.Actions, fromGenAction(a))
			}
		}
	}
	return out
}

func fromGenSnapshot(in gen.GameSnapshot) GameSnapshot {
	out := GameSnapshot{}
	if in.Metadata != nil {
		out.Metadata.Name = deref(in.Metadata.Name)
		out.Metadata.Namespace = deref(in.Metadata.Namespace)
		if in.Metadata.Labels != nil {
			out.Metadata.Labels = *in.Metadata.Labels
		}
	}
	if in.Spec != nil {
		out.Spec.GameServerRef = deref(in.Spec.GameServerRef)
		out.Spec.Schedule = deref(in.Spec.Schedule)
	}
	if in.Status != nil {
		out.Status.State = deref((*string)(in.Status.State))
		out.Status.Size = deref(in.Status.Size)
		out.Status.ReadyAt = in.Status.ReadyAt
	}
	return out
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func mapOrNil(m map[string]string) *map[string]string {
	if m == nil {
		return nil
	}
	return &m
}

// GameServers

func (c *Client) ListGameServers(ctx context.Context) ([]GameServer, error) {
	resp, err := c.inner.ListGameServersWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := []GameServer{}
	for _, gs := range *resp.JSON200 {
		out = append(out, fromGenGameServer(&gs))
	}
	return out, nil
}

func (c *Client) GetGameServer(ctx context.Context, namespace, name string) (*GameServer, error) {
	resp, err := c.inner.GetGameServerWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenGameServer(resp.JSON200)
	return &out, nil
}

func (c *Client) CreateGameServer(ctx context.Context, namespace string, gs *GameServer) (*GameServer, error) {
	gs.APIVersion = "operator.minato.io/v1"
	gs.Kind = "GameServer"
	body := toGenGameServer(gs)
	resp, err := c.inner.CreateGameServerWithResponse(ctx, namespace, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenGameServer(resp.JSON201)
	return &out, nil
}

func (c *Client) DeleteGameServer(ctx context.Context, namespace, name string) error {
	resp, err := c.inner.DeleteGameServerWithResponse(ctx, namespace, name)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// GetSFTPInfo returns the SFTP connection info (host, port, username,
// password) for a game server whose profile enables capabilities.sftp
// (operator+ role required).
func (c *Client) GetSFTPInfo(ctx context.Context, namespace, name string) (*SFTPInfo, error) {
	resp, err := c.inner.GetSFTPInfoWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := &SFTPInfo{
		Host:     resp.JSON200.Host,
		Port:     int32(resp.JSON200.Port),
		Username: resp.JSON200.Username,
		Password: resp.JSON200.Password,
	}
	return out, nil
}

// UpdateGameServerLifecycle patches a GameServer's lifecycle. Pass nil for
// fields that should be left unchanged. Setting autoStart=false on a running
// server gracefully stops it (state Stopped); autoStart=true starts a
// stopped server again.
func (c *Client) UpdateGameServerLifecycle(ctx context.Context, namespace, name string, autoStart *bool, idleTimeout *int32) (*GameServer, error) {
	var idle *int
	if idleTimeout != nil {
		v := int(*idleTimeout)
		idle = &v
	}
	body := gen.GameServerLifecyclePatch{
		Spec: &struct {
			Lifecycle *gen.GameServerLifecycle `json:"lifecycle,omitempty"`
		}{
			Lifecycle: &gen.GameServerLifecycle{
				AutoStart:          autoStart,
				IdleTimeoutSeconds: idle,
			},
		},
	}
	resp, err := c.inner.UpdateGameServerLifecycleWithResponse(ctx, namespace, name, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenGameServer(resp.JSON200)
	return &out, nil
}

// Actions

func (c *Client) ListActions(ctx context.Context, namespace, name string) ([]Action, error) {
	resp, err := c.inner.ListActionsWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := []Action{}
	for _, a := range *resp.JSON200 {
		out = append(out, fromGenAction(a))
	}
	return out, nil
}

func (c *Client) ExecuteAction(ctx context.Context, namespace, name, action string, params map[string]string) (*ActionExecutionRef, error) {
	resp, err := c.inner.ExecuteActionWithResponse(ctx, namespace, name, action, params)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return &ActionExecutionRef{Name: resp.JSON201.Name}, nil
}

func (c *Client) GetActionExecution(ctx context.Context, namespace, name, executionID string) (*ActionExecution, error) {
	resp, err := c.inner.GetActionExecutionWithResponse(ctx, namespace, name, executionID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := &ActionExecution{}
	in := resp.JSON200
	if in.Metadata != nil {
		out.Metadata.Name = deref(in.Metadata.Name)
		out.Metadata.Namespace = deref(in.Metadata.Namespace)
		if in.Metadata.Labels != nil {
			out.Metadata.Labels = *in.Metadata.Labels
		}
	}
	if in.Spec != nil {
		out.Spec.ActionName = deref(in.Spec.ActionName)
		out.Spec.Caller = deref(in.Spec.Caller)
		if in.Spec.Params != nil {
			out.Spec.Params = *in.Spec.Params
		}
	}
	if in.Status != nil {
		out.Status.State = deref((*string)(in.Status.State))
		out.Status.Result = deref(in.Status.Result)
		out.Status.Error = deref(in.Status.Error)
		out.Status.StartedAt = in.Status.StartedAt
		out.Status.CompletedAt = in.Status.CompletedAt
	}
	return out, nil
}

// Snapshots

func (c *Client) ListSnapshots(ctx context.Context, namespace, name string) ([]GameSnapshot, error) {
	resp, err := c.inner.ListSnapshotsWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := []GameSnapshot{}
	for _, s := range *resp.JSON200 {
		out = append(out, fromGenSnapshot(s))
	}
	return out, nil
}

func (c *Client) CreateSnapshot(ctx context.Context, namespace, name string) (*GameSnapshot, error) {
	resp, err := c.inner.CreateSnapshotWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenSnapshot(*resp.JSON201)
	return &out, nil
}

// Profiles

func (c *Client) ListProfiles(ctx context.Context) ([]GameProfile, error) {
	resp, err := c.inner.ListProfilesWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := []GameProfile{}
	for _, p := range *resp.JSON200 {
		out = append(out, fromGenProfile(p))
	}
	return out, nil
}

func (c *Client) GetProfile(ctx context.Context, name string) (*GameProfile, error) {
	resp, err := c.inner.GetProfileWithResponse(ctx, name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenProfile(*resp.JSON200)
	return &out, nil
}

// Fleets

func fromGenFleet(in gen.GameServerFleet) GameServerFleet {
	out := GameServerFleet{}
	if in.Metadata != nil {
		out.Metadata.Name = deref(in.Metadata.Name)
		out.Metadata.Namespace = deref(in.Metadata.Namespace)
		if in.Metadata.Labels != nil {
			out.Metadata.Labels = *in.Metadata.Labels
		}
	}
	if in.Spec != nil {
		out.Spec.Profile = deref(in.Spec.Profile)
		if in.Spec.Replicas != nil {
			out.Spec.Replicas = int32(*in.Spec.Replicas)
		}
	}
	if in.Status != nil {
		if in.Status.Replicas != nil {
			out.Status.Replicas = int32(*in.Status.Replicas)
		}
		if in.Status.ReadyReplicas != nil {
			out.Status.ReadyReplicas = int32(*in.Status.ReadyReplicas)
		}
		if in.Status.UpdatedReplicas != nil {
			out.Status.UpdatedReplicas = int32(*in.Status.UpdatedReplicas)
		}
		if in.Status.AvailableReplicas != nil {
			out.Status.AvailableReplicas = int32(*in.Status.AvailableReplicas)
		}
	}
	return out
}

func (c *Client) ListGameServerFleets(ctx context.Context) ([]GameServerFleet, error) {
	resp, err := c.inner.ListGameServerFleetsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := []GameServerFleet{}
	for _, f := range *resp.JSON200 {
		out = append(out, fromGenFleet(f))
	}
	return out, nil
}

func (c *Client) GetGameServerFleet(ctx context.Context, namespace, name string) (*GameServerFleet, error) {
	resp, err := c.inner.GetGameServerFleetWithResponse(ctx, namespace, name)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenFleet(*resp.JSON200)
	return &out, nil
}

// CreateGameServerFleet creates a GameServerFleet in the namespace (admin
// only). The namespace is auto-created if missing; the profile must exist.
func (c *Client) CreateGameServerFleet(ctx context.Context, namespace string, fleet *GameServerFleet) (*GameServerFleet, error) {
	raw, err := json.Marshal(map[string]any{
		"apiVersion": "operator.minato.io/v1",
		"kind":       "GameServerFleet",
		"metadata":   map[string]any{"name": fleet.Metadata.Name},
		"spec": map[string]any{
			"profile":  fleet.Spec.Profile,
			"replicas": fleet.Spec.Replicas,
		},
	})
	if err != nil {
		return nil, err
	}
	var body gen.GameServerFleet
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, err
	}
	resp, err := c.inner.CreateGameServerFleetWithResponse(ctx, namespace, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenFleet(*resp.JSON201)
	return &out, nil
}

// ScaleGameServerFleet sets a fleet's replica count (operator+).
func (c *Client) ScaleGameServerFleet(ctx context.Context, namespace, name string, replicas int32) (*GameServerFleet, error) {
	r := int(replicas)
	body := gen.GameServerFleetPatch{
		Spec: &struct {
			Replicas *int `json:"replicas,omitempty"`
		}{
			Replicas: &r,
		},
	}
	resp, err := c.inner.UpdateGameServerFleetWithResponse(ctx, namespace, name, body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	out := fromGenFleet(*resp.JSON200)
	return &out, nil
}

// DeleteGameServerFleet deletes a fleet and its GameServers (admin only).
func (c *Client) DeleteGameServerFleet(ctx context.Context, namespace, name string) error {
	resp, err := c.inner.DeleteGameServerFleetWithResponse(ctx, namespace, name)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

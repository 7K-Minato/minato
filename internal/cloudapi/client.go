// Package cloudapi is the Go client for the minato-cloud SaaS API. HTTP
// calls are generated from api/minato-cloud.openapi.yaml (see gen/;
// regenerate with `make oapi-generate`). This file adds an ergonomic
// wrapper: auth header injection, error mapping and tenant resolution.
package cloudapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/7k-minato/minato/internal/cloudapi/gen"
)

// Re-exported generated models, so callers only import this package.
type (
	Tenant                    = gen.Tenant
	Server                    = gen.Server
	ServerWithLive            = gen.ServerWithLive
	CreateServerRequest       = gen.CreateServerRequest
	GameSnapshot              = gen.GameSnapshot
	Action                    = gen.Action
	ActionExecutionRef        = gen.ActionExecutionRef
	CatalogEntry              = gen.CatalogEntry
	Plan                      = gen.Plan
	Subscription              = gen.Subscription
	APIKey                    = gen.APIKey
	CreateAPIKeyRequest       = gen.CreateAPIKeyRequest
	CreateAPIKeyRequestScopes = gen.CreateAPIKeyRequestScopes
	APIKeyCreated             = gen.APIKeyCreated
)

// Client talks to a single minato-cloud deployment.
type Client struct {
	baseURL string
	inner   *gen.ClientWithResponses
}

// NewClient creates a cloud API client. token is a Keycloak OIDC ID token or
// a tenant API key (mk_...); it is sent as a Bearer credential.
func NewClient(baseURL, token string, timeout time.Duration) (*Client, error) {
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		return nil, fmt.Errorf("cloudapi: invalid base URL %q: %w", baseURL, err)
	}
	inner, err := gen.NewClientWithResponses(
		strings.TrimSuffix(baseURL, "/"),
		gen.WithHTTPClient(&http.Client{Timeout: timeout}),
		gen.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Accept", "application/json")
			if token != "" {
				req.Header.Set("Authorization", "Bearer "+token)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return &Client{baseURL: baseURL, inner: inner}, nil
}

// BaseURL returns the cloud API base URL.
func (c *Client) BaseURL() string { return c.baseURL }

// APIError is a non-2xx response from the cloud API.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("cloudapi: status %d: %s", e.Status, e.Message)
}

func apiError(status int, body []byte) error {
	msg := string(body)
	var env gen.Error
	if json.Unmarshal(body, &env) == nil && env.Error != "" {
		msg = env.Error
	}
	return &APIError{Status: status, Message: msg}
}

// Tenants

func (c *Client) ListMyTenants(ctx context.Context) ([]Tenant, error) {
	resp, err := c.inner.ListMyTenantsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON200 == nil {
		return []Tenant{}, nil
	}
	return *resp.JSON200, nil
}

// ResolveTenant resolves a tenant reference (id, slug or name) to a tenant.
// An empty ref defaults to the caller's only tenant; it is an error when the
// caller belongs to zero or several tenants.
func (c *Client) ResolveTenant(ctx context.Context, ref string) (*Tenant, error) {
	tenants, err := c.ListMyTenants(ctx)
	if err != nil {
		return nil, err
	}
	if ref == "" {
		switch len(tenants) {
		case 0:
			return nil, fmt.Errorf("cloudapi: you are not a member of any tenant")
		case 1:
			return &tenants[0], nil
		default:
			names := make([]string, 0, len(tenants))
			for _, t := range tenants {
				names = append(names, fmt.Sprintf("%s (%s)", t.Slug, t.Id))
			}
			return nil, fmt.Errorf("cloudapi: multiple tenants — pass --tenant with one of: %s", strings.Join(names, ", "))
		}
	}
	for i := range tenants {
		t := &tenants[i]
		if t.Id == ref || t.Slug == ref || strings.EqualFold(t.Name, ref) {
			return t, nil
		}
	}
	return nil, fmt.Errorf("cloudapi: no tenant matching %q (id, slug or name)", ref)
}

// Servers

func (c *Client) ListServers(ctx context.Context, tenantID string) ([]Server, error) {
	resp, err := c.inner.ListServersWithResponse(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON200 == nil {
		return []Server{}, nil
	}
	return *resp.JSON200, nil
}

func (c *Client) GetServer(ctx context.Context, tenantID, serverID string) (*ServerWithLive, error) {
	resp, err := c.inner.GetServerWithResponse(ctx, tenantID, serverID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

func (c *Client) CreateServer(ctx context.Context, tenantID string, req CreateServerRequest) (*Server, error) {
	resp, err := c.inner.CreateServerWithResponse(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

func (c *Client) DeleteServer(ctx context.Context, tenantID, serverID string) error {
	resp, err := c.inner.DeleteServerWithResponse(ctx, tenantID, serverID)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

// Snapshots

func (c *Client) ListSnapshots(ctx context.Context, tenantID, serverID string) ([]GameSnapshot, error) {
	resp, err := c.inner.ListSnapshotsWithResponse(ctx, tenantID, serverID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON200 == nil {
		return []GameSnapshot{}, nil
	}
	return *resp.JSON200, nil
}

func (c *Client) CreateSnapshot(ctx context.Context, tenantID, serverID string) (*GameSnapshot, error) {
	resp, err := c.inner.CreateSnapshotWithResponse(ctx, tenantID, serverID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

// Actions

func (c *Client) ListActions(ctx context.Context, tenantID, serverID string) ([]Action, error) {
	resp, err := c.inner.ListActionsWithResponse(ctx, tenantID, serverID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON200 == nil {
		return []Action{}, nil
	}
	return *resp.JSON200, nil
}

func (c *Client) ExecuteAction(ctx context.Context, tenantID, serverID, action string, params map[string]string) (*ActionExecutionRef, error) {
	resp, err := c.inner.ExecuteActionWithResponse(ctx, tenantID, serverID, action, params)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusAccepted {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON202, nil
}

// Catalog, plans, billing

func (c *Client) GetCatalog(ctx context.Context, tenantID string) ([]CatalogEntry, error) {
	resp, err := c.inner.GetCatalogWithResponse(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON200 == nil {
		return []CatalogEntry{}, nil
	}
	return *resp.JSON200, nil
}

func (c *Client) ListPlans(ctx context.Context) ([]Plan, error) {
	resp, err := c.inner.ListPlansWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON200 == nil {
		return []Plan{}, nil
	}
	return *resp.JSON200, nil
}

func (c *Client) GetSubscription(ctx context.Context, tenantID string) (*Subscription, error) {
	resp, err := c.inner.GetSubscriptionWithResponse(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON200, nil
}

// API keys

func (c *Client) ListAPIKeys(ctx context.Context, tenantID string) ([]APIKey, error) {
	resp, err := c.inner.ListAPIKeysWithResponse(ctx, tenantID)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	if resp.JSON200 == nil {
		return []APIKey{}, nil
	}
	return *resp.JSON200, nil
}

func (c *Client) CreateAPIKey(ctx context.Context, tenantID string, req CreateAPIKeyRequest) (*APIKeyCreated, error) {
	resp, err := c.inner.CreateAPIKeyWithResponse(ctx, tenantID, req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != http.StatusCreated {
		return nil, apiError(resp.StatusCode(), resp.Body)
	}
	return resp.JSON201, nil
}

func (c *Client) DeleteAPIKey(ctx context.Context, tenantID, keyID string) error {
	resp, err := c.inner.DeleteAPIKeyWithResponse(ctx, tenantID, keyID)
	if err != nil {
		return err
	}
	if resp.StatusCode() != http.StatusNoContent {
		return apiError(resp.StatusCode(), resp.Body)
	}
	return nil
}

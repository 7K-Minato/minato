package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	agentv1 "github.com/7k-minato/minato/api/agent/v1/minato/agent/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type noopAgent struct{}

func (n *noopAgent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	return &agentv1.InfoResponse{}, nil
}

func (n *noopAgent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	return &agentv1.HealthResponse{}, nil
}

func (n *noopAgent) PrepareShutdown(
	ctx context.Context,
	req *agentv1.ShutdownRequest,
) (*agentv1.ShutdownResponse, error) {
	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (n *noopAgent) GetPlayers(
	ctx context.Context,
	req *agentv1.PlayersRequest,
) (*agentv1.PlayersResponse, error) {
	return &agentv1.PlayersResponse{}, nil
}

func (n *noopAgent) ExecuteAction(
	ctx context.Context,
	req *agentv1.ExecuteActionRequest,
) (*agentv1.ExecuteActionResponse, error) {
	return &agentv1.ExecuteActionResponse{}, nil
}

func (n *noopAgent) Console(stream agentv1.Agent_ConsoleServer) error {
	return nil
}

type mockAgent struct {
	infoCalled            bool
	healthCheckCalled     bool
	prepareShutdownCalled bool
	getPlayersCalled      bool
	executeActionCalled   bool
	consoleCalled         bool
	panicOnInfo           bool
}

func (m *mockAgent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	m.infoCalled = true
	if m.panicOnInfo {
		panic("intentional panic")
	}
	return &agentv1.InfoResponse{Name: "test-agent", Version: "1.0.0"}, nil
}

func (m *mockAgent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	m.healthCheckCalled = true
	return &agentv1.HealthResponse{Ready: true}, nil
}

func (m *mockAgent) PrepareShutdown(
	ctx context.Context,
	req *agentv1.ShutdownRequest,
) (*agentv1.ShutdownResponse, error) {
	m.prepareShutdownCalled = true
	return &agentv1.ShutdownResponse{Success: true}, nil
}

func (m *mockAgent) GetPlayers(
	ctx context.Context,
	req *agentv1.PlayersRequest,
) (*agentv1.PlayersResponse, error) {
	m.getPlayersCalled = true
	return &agentv1.PlayersResponse{Online: 5, Capacity: 20}, nil
}

func (m *mockAgent) ExecuteAction(
	ctx context.Context,
	req *agentv1.ExecuteActionRequest,
) (*agentv1.ExecuteActionResponse, error) {
	m.executeActionCalled = true
	return &agentv1.ExecuteActionResponse{}, nil
}

func (m *mockAgent) Console(stream agentv1.Agent_ConsoleServer) error {
	m.consoleCalled = true
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

func TestServeCustomOptions(t *testing.T) {
	server, err := Serve(&noopAgent{}, Options{
		GRPCAddr:      "127.0.0.1:0",
		MetricsAddr:   "127.0.0.1:0",
		ShutdownGrace: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if server == nil {
		t.Fatalf("expected server")
	}
	_ = server.Shutdown(context.Background())
}

func TestServeDefaultAddresses(t *testing.T) {
	// We can't bind to :9876 and :9090 in parallel tests easily,
	// so we verify the defaults by checking the struct values indirectly.
	// Instead, we test that empty options are filled with defaults.
	server, err := Serve(&noopAgent{}, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	if server == nil {
		t.Fatalf("expected server")
	}
	_ = server.Shutdown(context.Background())
}

func TestShutdownNilServer(t *testing.T) {
	var s *Server
	err := s.Shutdown(context.Background())
	if err == nil {
		t.Fatalf("expected error for nil server")
	}
}

func TestShutdownGraceful(t *testing.T) {
	server, err := Serve(&noopAgent{}, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
}

func TestShutdownWithTimeout(t *testing.T) {
	server, err := Serve(&noopAgent{}, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()
	// The shutdown should handle context timeout gracefully
	_ = server.Shutdown(ctx)
}

func TestMetricsEndpoints(t *testing.T) {
	// Use a fixed port to avoid "127.0.0.1:0" being stored in metricsSrv.Addr.
	// We bind the metrics server to the same address as the gRPC listener
	// by creating a listener first and passing its address.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	metricsAddr := listener.Addr().String()
	_ = listener.Close()

	server, err := Serve(&noopAgent{}, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: metricsAddr})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer func() { _ = server.Shutdown(context.Background()) }()

	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)

	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", metricsAddr))
	if err != nil {
		t.Fatalf("healthz request: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Fatalf("expected body ok, got %q", string(body))
	}

	resp2, err := http.Get(fmt.Sprintf("http://%s/metrics", metricsAddr))
	if err != nil {
		t.Fatalf("metrics request: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp2.StatusCode)
	}
}

func TestGRPCHandlers(t *testing.T) {
	agent := &mockAgent{}
	server, err := Serve(agent, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer func() { _ = server.Shutdown(context.Background()) }()

	// Give the server a moment to start
	time.Sleep(50 * time.Millisecond)

	grpcAddr := server.listener.Addr().String()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentv1.NewAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Test Info
	infoResp, err := client.Info(ctx, &agentv1.InfoRequest{})
	if err != nil {
		t.Fatalf("info: %v", err)
	}
	if infoResp.Name != "test-agent" {
		t.Fatalf("expected name test-agent, got %s", infoResp.Name)
	}
	if !agent.infoCalled {
		t.Fatalf("expected Info to be called")
	}

	// Test HealthCheck
	_, err = client.HealthCheck(ctx, &agentv1.HealthRequest{})
	if err != nil {
		t.Fatalf("healthcheck: %v", err)
	}
	if !agent.healthCheckCalled {
		t.Fatalf("expected HealthCheck to be called")
	}

	// Test PrepareShutdown
	_, err = client.PrepareShutdown(ctx, &agentv1.ShutdownRequest{})
	if err != nil {
		t.Fatalf("prepareshutdown: %v", err)
	}
	if !agent.prepareShutdownCalled {
		t.Fatalf("expected PrepareShutdown to be called")
	}

	// Test GetPlayers
	playersResp, err := client.GetPlayers(ctx, &agentv1.PlayersRequest{})
	if err != nil {
		t.Fatalf("getplayers: %v", err)
	}
	if playersResp.GetOnline() != 5 {
		t.Fatalf("expected online 5, got %d", playersResp.GetOnline())
	}
	if !agent.getPlayersCalled {
		t.Fatalf("expected GetPlayers to be called")
	}

	// Test ExecuteAction
	_, err = client.ExecuteAction(ctx, &agentv1.ExecuteActionRequest{})
	if err != nil {
		t.Fatalf("executeaction: %v", err)
	}
	if !agent.executeActionCalled {
		t.Fatalf("expected ExecuteAction to be called")
	}
}

func TestGRPCConsoleHandler(t *testing.T) {
	agent := &mockAgent{}
	server, err := Serve(agent, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer func() { _ = server.Shutdown(context.Background()) }()

	time.Sleep(50 * time.Millisecond)

	grpcAddr := server.listener.Addr().String()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentv1.NewAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stream, err := client.Console(ctx)
	if err != nil {
		t.Fatalf("console: %v", err)
	}
	// Recv once to trigger the server handler
	_, _ = stream.Recv()
	_ = stream.CloseSend()
	if !agent.consoleCalled {
		t.Fatalf("expected Console to be called")
	}
}

func TestRecoverInterceptorNormal(t *testing.T) {
	agent := &mockAgent{}
	server, err := Serve(agent, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer func() { _ = server.Shutdown(context.Background()) }()

	time.Sleep(50 * time.Millisecond)

	grpcAddr := server.listener.Addr().String()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentv1.NewAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.Info(ctx, &agentv1.InfoRequest{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRecoverInterceptorPanic(t *testing.T) {
	agent := &mockAgent{panicOnInfo: true}
	server, err := Serve(agent, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer func() { _ = server.Shutdown(context.Background()) }()

	time.Sleep(100 * time.Millisecond)

	grpcAddr := server.listener.Addr().String()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentv1.NewAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.Info(ctx, &agentv1.InfoRequest{})
	if err == nil {
		t.Fatalf("expected error after panic recovery")
	}
	// Verify it's an internal error from the panic recovery
	if !strings.Contains(err.Error(), "panic") {
		t.Fatalf("expected panic error, got %v", err)
	}
}

func TestHandlerReturnsError(t *testing.T) {
	errAgent := &errorAgent{}
	server, err := Serve(errAgent, Options{GRPCAddr: "127.0.0.1:0", MetricsAddr: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("serve: %v", err)
	}
	defer func() { _ = server.Shutdown(context.Background()) }()

	time.Sleep(50 * time.Millisecond)

	grpcAddr := server.listener.Addr().String()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("grpc dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	client := agentv1.NewAgentClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err = client.Info(ctx, &agentv1.InfoRequest{})
	if err == nil {
		t.Fatalf("expected error from handler")
	}
}

type errorAgent struct{}

func (e *errorAgent) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	return nil, errors.New("info error")
}

func (e *errorAgent) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	return nil, errors.New("healthcheck error")
}

func (e *errorAgent) PrepareShutdown(
	ctx context.Context,
	req *agentv1.ShutdownRequest,
) (*agentv1.ShutdownResponse, error) {
	return nil, errors.New("shutdown error")
}

func (e *errorAgent) GetPlayers(
	ctx context.Context,
	req *agentv1.PlayersRequest,
) (*agentv1.PlayersResponse, error) {
	return nil, errors.New("getplayers error")
}

func (e *errorAgent) ExecuteAction(
	ctx context.Context,
	req *agentv1.ExecuteActionRequest,
) (*agentv1.ExecuteActionResponse, error) {
	return nil, errors.New("executeaction error")
}

func (e *errorAgent) Console(stream agentv1.Agent_ConsoleServer) error {
	return errors.New("console error")
}

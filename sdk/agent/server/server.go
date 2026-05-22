package server

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"google.golang.org/grpc"

	agentv1 "github.com/7k-group/minato/api/agent/v1/minato/agent/v1"
)

type Agent interface {
	Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error)
	HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error)
	PrepareShutdown(ctx context.Context, req *agentv1.ShutdownRequest) (*agentv1.ShutdownResponse, error)
	GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error)
	ExecuteAction(ctx context.Context, req *agentv1.ExecuteActionRequest) (*agentv1.ExecuteActionResponse, error)
	Console(agentv1.Agent_ConsoleServer) error
}

type Server struct {
	grpcServer *grpc.Server
	listener   net.Listener
	metricsSrv *http.Server
}

type Options struct {
	GRPCAddr      string
	MetricsAddr   string
	ShutdownGrace time.Duration
}

func Serve(agent Agent, opts Options) (*Server, error) {
	if opts.GRPCAddr == "" {
		opts.GRPCAddr = ":9876"
	}
	if opts.MetricsAddr == "" {
		opts.MetricsAddr = ":9090"
	}
	if opts.ShutdownGrace == 0 {
		opts.ShutdownGrace = 5 * time.Second
	}

	listener, err := net.Listen("tcp", opts.GRPCAddr)
	if err != nil {
		return nil, err
	}

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(recoverInterceptor()))
	agentv1.RegisterAgentServer(grpcServer, &handler{agent: agent})

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(""))
	})
	metricsSrv := &http.Server{Addr: opts.MetricsAddr, Handler: mux}

	server := &Server{grpcServer: grpcServer, listener: listener, metricsSrv: metricsSrv}

	go func() {
		_ = metricsSrv.ListenAndServe()
	}()
	go func() {
		_ = grpcServer.Serve(listener)
	}()

	return server, nil
}

func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil {
		return errors.New("server is nil")
	}

	stopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	select {
	case <-ctx.Done():
		s.grpcServer.Stop()
	case <-stopped:
	}

	_ = s.metricsSrv.Shutdown(ctx)
	return nil
}

type handler struct {
	agentv1.UnimplementedAgentServer
	agent Agent
}

func (h *handler) Info(ctx context.Context, req *agentv1.InfoRequest) (*agentv1.InfoResponse, error) {
	return h.agent.Info(ctx, req)
}

func (h *handler) HealthCheck(ctx context.Context, req *agentv1.HealthRequest) (*agentv1.HealthResponse, error) {
	return h.agent.HealthCheck(ctx, req)
}

func (h *handler) PrepareShutdown(ctx context.Context, req *agentv1.ShutdownRequest) (*agentv1.ShutdownResponse, error) {
	return h.agent.PrepareShutdown(ctx, req)
}

func (h *handler) GetPlayers(ctx context.Context, req *agentv1.PlayersRequest) (*agentv1.PlayersResponse, error) {
	return h.agent.GetPlayers(ctx, req)
}

func (h *handler) ExecuteAction(ctx context.Context, req *agentv1.ExecuteActionRequest) (*agentv1.ExecuteActionResponse, error) {
	return h.agent.ExecuteAction(ctx, req)
}

func (h *handler) Console(stream agentv1.Agent_ConsoleServer) error {
	return h.agent.Console(stream)
}

func recoverInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic in %s: %v", info.FullMethod, r)
			}
		}()

		resp, err := handler(ctx, req)
		if err != nil {
			return nil, err
		}
		return resp, nil
	}
}

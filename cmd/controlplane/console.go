package main

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	agentv1 "github.com/7k-minato/minato/api/agent/v1/minato/agent/v1"
	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/sdk/controlplane"
)

const defaultAgentGRPCPort = 9876

var allowedOrigins = []string{}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if len(allowedOrigins) == 0 {
			return true // Allow all if no restrictions configured
		}
		return slices.Contains(allowedOrigins, origin)
	},
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// ConsoleMessage is one frame of the console WebSocket protocol.
// Canonical definition lives in the SDK (contract: api/console.schema.json).
type ConsoleMessage = controlplane.ConsoleMessage

func (api *controlPlaneAPI) serveConsole(w http.ResponseWriter, r *http.Request, ns, name string) {
	// Verify GameServer exists
	server := &operatorv1.GameServer{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: ns}, server); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = conn.Close() }()

	// Connect to agent via gRPC
	if err := api.proxyConsole(r.Context(), conn, server); err != nil {
		// Send error to client
		msg := ConsoleMessage{Type: "error", Data: err.Error()}
		_ = conn.WriteJSON(msg)
	}
}

func (api *controlPlaneAPI) proxyConsole(
	ctx context.Context,
	wsConn *websocket.Conn,
	server *operatorv1.GameServer,
) error {
	// Get service to resolve agent endpoint
	svc := &corev1.Service{}
	if err := api.client.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, svc); err != nil {
		return fmt.Errorf("failed to get service: %w", err)
	}

	addr := fmt.Sprintf("%s.%s.svc.cluster.local:%d", svc.Name, svc.Namespace, defaultAgentGRPCPort)

	grpcConn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("failed to connect to agent: %w", err)
	}
	defer func() { _ = grpcConn.Close() }()

	client := agentv1.NewAgentClient(grpcConn)
	stream, err := client.Console(ctx)
	if err != nil {
		return fmt.Errorf("failed to start console stream: %w", err)
	}

	// Handle incoming WebSocket messages from client
	errCh := make(chan error, 2)

	// Client → Agent
	go func() {
		for {
			var msg ConsoleMessage
			if err := wsConn.ReadJSON(&msg); err != nil {
				errCh <- err
				return
			}

			switch msg.Type {
			case "rcon":
				if err := stream.Send(&agentv1.ConsoleClientMessage{
					Payload: &agentv1.ConsoleClientMessage_Command{
						Command: &agentv1.ConsoleCommand{RconCommand: msg.Data},
					},
				}); err != nil {
					errCh <- err
					return
				}
			case "ping":
				if err := stream.Send(&agentv1.ConsoleClientMessage{
					Payload: &agentv1.ConsoleClientMessage_Ping{
						Ping: &agentv1.ConsolePing{Message: msg.Data},
					},
				}); err != nil {
					errCh <- err
					return
				}
			}
		}
	}()

	// Agent → Client
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				errCh <- err
				return
			}

			var msg ConsoleMessage
			switch p := resp.Payload.(type) {
			case *agentv1.ConsoleServerMessage_Log:
				msg = ConsoleMessage{Type: "log", TS: time.Now().Unix(), Line: p.Log.Line}
			case *agentv1.ConsoleServerMessage_Response:
				msg = ConsoleMessage{Type: "rcon-response", Data: p.Response.Response}
			case *agentv1.ConsoleServerMessage_Status:
				msg = ConsoleMessage{Type: "status", Data: p.Status.State}
			case *agentv1.ConsoleServerMessage_Error:
				msg = ConsoleMessage{Type: "error", Data: p.Error.Message}
			}

			if err := wsConn.WriteJSON(msg); err != nil {
				errCh <- err
				return
			}
		}
	}()

	return <-errCh
}

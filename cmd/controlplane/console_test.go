package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

func setupConsoleTestAPI(objs ...client.Object) *controlPlaneAPI {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	api := &controlPlaneAPI{client: c}
	return api
}

func TestHandleConsole_MissingParams(t *testing.T) {
	api := setupConsoleTestAPI()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gameservers/default/gs1/console", nil)
	api.handleConsole(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "namespace and name required") {
		t.Fatalf("unexpected error message: %s", rec.Body.String())
	}
}

func TestHandleConsole_GameServerNotFound(t *testing.T) {
	api := setupConsoleTestAPI()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gameservers/default/gs1/console?namespace=default&name=gs1", nil)
	api.handleConsole(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleConsole_UpgradeFails(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api := setupConsoleTestAPI(gs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/gameservers/default/gs1/console?namespace=default&name=gs1", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-Websocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	// httptest.ResponseRecorder is not a valid hijacker, so Upgrade will fail with 500
	api.handleConsole(rec, req)

	// When upgrade fails, handleConsole writes an HTTP error response
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProxyConsole_ServiceNotFound(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api := setupConsoleTestAPI(gs)

	// Create a fake WebSocket connection using a local pipe
	serverConn, clientConn := newFakeWSConn(t)
	defer func() { _ = serverConn.Close() }()
	defer func() { _ = clientConn.Close() }()

	ctx := context.Background()
	err := api.proxyConsole(ctx, serverConn, gs)
	if err == nil {
		t.Fatal("expected error when service is not found")
	}
	if !strings.Contains(err.Error(), "failed to get service") {
		t.Fatalf("expected 'failed to get service' error, got: %v", err)
	}
}

func TestProxyConsole_ServiceFound_GrpcFails(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
	}
	api := setupConsoleTestAPI(gs, svc)

	serverConn, clientConn := newFakeWSConn(t)
	defer func() { _ = serverConn.Close() }()
	defer func() { _ = clientConn.Close() }()

	ctx := context.Background()
	err := api.proxyConsole(ctx, serverConn, gs)
	if err == nil {
		t.Fatal("expected error when gRPC connection fails")
	}
	if !strings.Contains(err.Error(), "failed to connect to agent") && !strings.Contains(err.Error(), "failed to start console stream") {
		t.Fatalf("expected 'failed to connect to agent' or 'failed to start console stream' error, got: %v", err)
	}
}

// newFakeWSConn creates a pair of websocket connections over an in-memory net.Pipe.
// It returns the server-side connection and a client-side connection.
func newFakeWSConn(t *testing.T) (*websocket.Conn, *websocket.Conn) {
	t.Helper()

	// Use a local HTTP server to perform the WebSocket upgrade
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}

	var serverConn *websocket.Conn
	var upgradeErr error
	done := make(chan struct{})
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(done)
		serverConn, upgradeErr = upgrader.Upgrade(w, r, nil)
	}))
	defer httpServer.Close()

	// Dial the test server
	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("failed to dial test websocket server: %v", err)
	}

	<-done // wait for handler to finish so serverConn is assigned

	if upgradeErr != nil {
		t.Fatalf("server upgrade failed: %v", upgradeErr)
	}

	return serverConn, clientConn
}

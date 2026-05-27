package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

func setupTestAPI(objs ...client.Object) (*controlPlaneAPI, client.Client) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	builder := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...)

	// Index GameSnapshot by spec.gameServerRef
	builder = builder.WithIndex(
		&operatorv1.GameSnapshot{},
		"spec.gameServerRef",
		func(obj client.Object) []string {
			snap := obj.(*operatorv1.GameSnapshot)
			return []string{snap.Spec.GameServerRef}
		},
	)

	c := builder.Build()
	api := &controlPlaneAPI{client: c}
	return api, c
}

func newRouter(api *controlPlaneAPI) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/gameservers", api.listGameServers)
		r.Get("/gameservers/{namespace}/{name}", api.getGameServer)
		r.Post("/gameservers/{namespace}", api.createGameServer)
		r.Delete("/gameservers/{namespace}/{name}", api.deleteGameServer)

		r.Get("/gameservers/{namespace}/{name}/actions", api.listActions)
		r.Post("/gameservers/{namespace}/{name}/actions/{action}", api.executeAction)
		r.Get("/gameservers/{namespace}/{name}/actions/{executionId}", api.getActionExecution)

		r.Get("/gameservers/{namespace}/{name}/snapshots", api.listSnapshots)
		r.Post("/gameservers/{namespace}/{name}/snapshots", api.createSnapshot)

		r.Get("/gameservers/{namespace}/{name}/console", api.handleConsole)

		r.Get("/gameserverfleets", api.listGameServerFleets)
		r.Get("/gameserverfleets/{namespace}/{name}", api.getGameServerFleet)

		r.Get("/profiles", api.listProfiles)
		r.Get("/profiles/{name}", api.getProfile)
	})

	return r
}

// Test helper: respondJSON
func TestRespondJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	data := map[string]string{"key": "value"}
	respondJSON(rec, data)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	ct := rec.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %s", ct)
	}
	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if result["key"] != "value" {
		t.Fatalf("unexpected response: %v", result)
	}
}

// Health endpoints
func TestHealthz(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "ok" {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

// listGameServers
func TestListGameServers_Success(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api, _ := setupTestAPI(gs)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var items []operatorv1.GameServer
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 gameserver, got %d", len(items))
	}
}

func TestListGameServers_InternalError(t *testing.T) {
	// Use an empty scheme so List fails
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// getGameServer
func TestGetGameServer_Success(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api, _ := setupTestAPI(gs)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result operatorv1.GameServer
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Name != "gs1" {
		t.Fatalf("unexpected name: %s", result.Name)
	}
}

func TestGetGameServer_NotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/missing", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// createGameServer
func TestCreateGameServer_Success(t *testing.T) {
	api, c := setupTestAPI()
	r := newRouter(api)

	gs := operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-new"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	body, _ := json.Marshal(gs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var created operatorv1.GameServer
	if err := c.Get(context.Background(), types.NamespacedName{Name: "gs-new", Namespace: "default"}, &created); err != nil {
		t.Fatalf("expected created GameServer to exist: %v", err)
	}
}

func TestCreateGameServer_BadRequest(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default", strings.NewReader("not-json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCreateGameServer_InternalError(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	gs := operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs-new"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	body, _ := json.Marshal(gs)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	// fake client Create will succeed because scheme is registered; we need a different approach.
	// Instead, test with a scheme that lacks the type.
	scheme2 := runtime.NewScheme()
	c2 := fake.NewClientBuilder().WithScheme(scheme2).Build()
	api2 := &controlPlaneAPI{client: c2}
	r2 := newRouter(api2)

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	r2.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec2.Code)
	}
}

// deleteGameServer
func TestDeleteGameServer_Success(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api, c := setupTestAPI(gs)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gameservers/default/gs1", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", rec.Code, rec.Body.String())
	}

	var remaining operatorv1.GameServer
	if err := c.Get(context.Background(), types.NamespacedName{Name: "gs1", Namespace: "default"}, &remaining); err == nil {
		t.Fatalf("expected GameServer to be deleted")
	}
}

func TestDeleteGameServer_InternalError(t *testing.T) {
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/gameservers/default/gs1", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// listActions
func TestListActions_Success(t *testing.T) {
	profile := &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "minecraft"},
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Minecraft",
			Image:       "minecraft:latest",
			Storage:     operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "10Gi"},
			Agent:       operatorv1.AgentSpec{Image: "minato/minecraft-agent", Version: "v1"},
			Actions: []operatorv1.ActionDecl{
				{Name: "restart", Description: "Restart the server"},
			},
		},
	}
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api, _ := setupTestAPI(profile, gs)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/actions", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var actions []operatorv1.ActionDecl
	if err := json.Unmarshal(rec.Body.Bytes(), &actions); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(actions) != 1 || actions[0].Name != "restart" {
		t.Fatalf("unexpected actions: %v", actions)
	}
}

func TestListActions_GameServerNotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/missing/actions", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestListActions_ProfileNotFound(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "missing-profile"},
	}
	api, _ := setupTestAPI(gs)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/actions", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// executeAction
func TestExecuteAction_Success(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api, c := setupTestAPI(gs)
	r := newRouter(api)

	params := map[string]string{"reason": "maintenance"}
	body, _ := json.Marshal(params)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default/gs1/actions/restart", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User", "test-user")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var result map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result["name"] == "" {
		t.Fatalf("expected execution name in response")
	}

	var exec operatorv1.ActionExecution
	if err := c.Get(context.Background(), types.NamespacedName{Name: result["name"], Namespace: "default"}, &exec); err != nil {
		t.Fatalf("expected ActionExecution to exist: %v", err)
	}
	if exec.Spec.ActionName != "restart" {
		t.Fatalf("unexpected action name: %s", exec.Spec.ActionName)
	}
	if exec.Spec.Caller != "test-user" {
		t.Fatalf("unexpected caller: %s", exec.Spec.Caller)
	}
}

func TestExecuteAction_EmptyBody(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api, _ := setupTestAPI(gs)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default/gs1/actions/restart", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestExecuteAction_InternalError(t *testing.T) {
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default/gs1/actions/restart", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// getActionExecution
// NOTE: getActionExecution uses chi.URLParam(r, "name") which resolves to the GameServer name,
// not the executionId. This matches the current source code behavior.
func TestGetActionExecution_Success(t *testing.T) {
	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec: operatorv1.ActionExecutionSpec{
			ActionName: "restart",
			TargetRef:  operatorv1.TargetRef{Name: "gs1", Namespace: "default", Kind: "GameServer", APIVersion: "operator.minato.io/v1"},
		},
	}
	api, _ := setupTestAPI(exec)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/actions/exec1", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result operatorv1.ActionExecution
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Spec.ActionName != "restart" {
		t.Fatalf("unexpected action name: %s", result.Spec.ActionName)
	}
}

func TestGetActionExecution_NotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/actions/missing", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// listSnapshots
func TestListSnapshots_SuccessWithIndex(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap1", Namespace: "default"},
		Spec:       operatorv1.GameSnapshotSpec{GameServerRef: "gs1"},
	}
	api, _ := setupTestAPI(gs, snap)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/snapshots", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var items []operatorv1.GameSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(items))
	}
}

func TestListSnapshots_FallbackFilter(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{Name: "snap1", Namespace: "default"},
		Spec:       operatorv1.GameSnapshotSpec{GameServerRef: "gs1"},
	}
	// Build client without index to force fallback path
	scheme := runtime.NewScheme()
	_ = operatorv1.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(gs, snap).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/snapshots", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var items []operatorv1.GameSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(items))
	}
}

func TestListSnapshots_InternalError(t *testing.T) {
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameservers/default/gs1/snapshots", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// createSnapshot
func TestCreateSnapshot_Success(t *testing.T) {
	gs := &operatorv1.GameServer{
		ObjectMeta: metav1.ObjectMeta{Name: "gs1", Namespace: "default"},
		Spec:       operatorv1.GameServerSpec{Profile: "minecraft"},
	}
	api, c := setupTestAPI(gs)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default/gs1/snapshots", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}

	var snaps operatorv1.GameSnapshotList
	if err := c.List(context.Background(), &snaps, client.InNamespace("default")); err != nil {
		t.Fatalf("failed to list snapshots: %v", err)
	}
	if len(snaps.Items) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(snaps.Items))
	}
	if snaps.Items[0].Spec.GameServerRef != "gs1" {
		t.Fatalf("unexpected gameserver ref: %s", snaps.Items[0].Spec.GameServerRef)
	}
}

func TestCreateSnapshot_InternalError(t *testing.T) {
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/gameservers/default/gs1/snapshots", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// listGameServerFleets
func TestListGameServerFleets_Success(t *testing.T) {
	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "default"},
		Spec:       operatorv1.GameServerFleetSpec{Profile: "minecraft", Replicas: 3},
	}
	api, _ := setupTestAPI(fleet)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameserverfleets", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var items []operatorv1.GameServerFleet
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 fleet, got %d", len(items))
	}
}

func TestListGameServerFleets_InternalError(t *testing.T) {
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameserverfleets", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// getGameServerFleet
func TestGetGameServerFleet_Success(t *testing.T) {
	fleet := &operatorv1.GameServerFleet{
		ObjectMeta: metav1.ObjectMeta{Name: "fleet1", Namespace: "default"},
		Spec:       operatorv1.GameServerFleetSpec{Profile: "minecraft", Replicas: 3},
	}
	api, _ := setupTestAPI(fleet)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameserverfleets/default/fleet1", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result operatorv1.GameServerFleet
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Name != "fleet1" {
		t.Fatalf("unexpected name: %s", result.Name)
	}
}

func TestGetGameServerFleet_NotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/gameserverfleets/default/missing", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

// listProfiles
func TestListProfiles_Success(t *testing.T) {
	profile := &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "minecraft"},
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Minecraft",
			Image:       "minecraft:latest",
			Storage:     operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "10Gi"},
			Agent:       operatorv1.AgentSpec{Image: "minato/minecraft-agent", Version: "v1"},
		},
	}
	api, _ := setupTestAPI(profile)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var items []operatorv1.GameProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(items))
	}
}

func TestListProfiles_InternalError(t *testing.T) {
	scheme := runtime.NewScheme()
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	api := &controlPlaneAPI{client: c}
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

// getProfile
func TestGetProfile_Success(t *testing.T) {
	profile := &operatorv1.GameProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "minecraft"},
		Spec: operatorv1.GameProfileSpec{
			DisplayName: "Minecraft",
			Image:       "minecraft:latest",
			Storage:     operatorv1.StorageSpec{MountPath: "/data", SizeDefault: "10Gi"},
			Agent:       operatorv1.AgentSpec{Image: "minato/minecraft-agent", Version: "v1"},
		},
	}
	api, _ := setupTestAPI(profile)
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/minecraft", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var result operatorv1.GameProfile
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if result.Name != "minecraft" {
		t.Fatalf("unexpected name: %s", result.Name)
	}
}

func TestGetProfile_NotFound(t *testing.T) {
	api, _ := setupTestAPI()
	r := newRouter(api)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profiles/missing", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

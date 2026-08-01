package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controlplane/audit"
	"github.com/7k-minato/minato/internal/controlplane/auth"
	"github.com/7k-minato/minato/internal/controlplane/oapi"
	"github.com/7k-minato/minato/internal/controlplane/rbac"
)

func main() {
	cfg := config.GetConfigOrDie()
	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(operatorv1.AddToScheme(scheme))
	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		log.Fatalf("failed to create k8s client: %v", err)
	}

	// Load auth configuration
	authCfg := auth.LoadConfig()

	// API key storage (namespace where control plane runs)
	keyStorage := auth.NewAPIKeyStorage(c, os.Getenv("POD_NAMESPACE"))

	authChain, err := auth.BuildChainWithStorage(authCfg, keyStorage)
	if err != nil {
		log.Fatalf("failed to build auth chain: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// A global timeout breaks long-lived WebSocket connections and strips
	// Hijacker support; skip it for the console route.
	timeout := middleware.Timeout(30 * time.Second)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/console") {
				next.ServeHTTP(w, r)
				return
			}
			timeout(next).ServeHTTP(w, r)
		})
	})
	r.Use(securityHeadersMiddleware)
	r.Use(middleware.RequestSize(10 * 1024 * 1024)) // 10MB max request size
	r.Use(audit.Middleware())
	r.Use(auth.Middleware(authChain))
	r.Use(rbac.RouteGuard(rbac.DefaultRules))

	api := &controlPlaneAPI{client: c, authCfg: authCfg, keyStorage: keyStorage}

	// Mount all routes from the OpenAPI spec (api/openapi.yaml).
	oapi.HandlerFromMux(api, r)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: r,
	}

	go func() {
		log.Printf("Control plane starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

type controlPlaneAPI struct {
	client     client.Client
	authCfg    *auth.Config
	keyStorage *auth.APIKeyStorage
}

var _ oapi.ServerInterface = (*controlPlaneAPI)(nil)

// Health endpoints (public)

func (api *controlPlaneAPI) GetHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (api *controlPlaneAPI) GetReadyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (api *controlPlaneAPI) GetAuthConfig(w http.ResponseWriter, _ *http.Request) {
	// Parse auth modes from config
	modes := []string{}
	for mode := range strings.SplitSeq(api.authCfg.Mode, ",") {
		mode = strings.TrimSpace(strings.ToLower(mode))
		if mode != "" {
			modes = append(modes, mode)
		}
	}

	// If no explicit modes, infer from enabled providers
	if len(modes) == 0 || (len(modes) == 1 && modes[0] == "none") {
		modes = []string{"none"}
		if api.authCfg.Basic.Enabled {
			modes = append(modes, "basic")
		}
		if api.authCfg.OIDC.Enabled {
			modes = append(modes, "oidc")
		}
		if api.authCfg.APIKey.Enabled {
			modes = append(modes, "apikey")
		}
	}

	responseConfig := map[string]any{
		"authModes":    modes,
		"basicEnabled": api.authCfg.Basic.Enabled,
	}

	if api.authCfg.OIDC.Enabled && api.authCfg.OIDC.IssuerURL != "" {
		responseConfig["oidcIssuer"] = api.authCfg.OIDC.IssuerURL
	}

	respondJSON(w, http.StatusOK, responseConfig)
}

// GameServers

func (api *controlPlaneAPI) ListGameServers(w http.ResponseWriter, r *http.Request) {
	var list operatorv1.GameServerList
	if err := api.client.List(r.Context(), &list); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, list.Items)
}

func (api *controlPlaneAPI) GetGameServer(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name) {
	server := &operatorv1.GameServer{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: string(name), Namespace: string(namespace)}, server); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, server)
}

func (api *controlPlaneAPI) CreateGameServer(w http.ResponseWriter, r *http.Request, namespace string) {
	var server operatorv1.GameServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Ensure the target namespace exists; tenants land in their own namespace
	// which may not have been provisioned ahead of time.
	ns := &corev1.Namespace{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: namespace}, ns); apierrors.IsNotFound(err) {
		ns = &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		if err := api.client.Create(r.Context(), ns); err != nil && !apierrors.IsAlreadyExists(err) {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	server.Namespace = namespace
	if err := api.client.Create(r.Context(), &server); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusCreated, &server)
}

func (api *controlPlaneAPI) DeleteGameServer(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name) {
	server := &operatorv1.GameServer{ObjectMeta: metav1.ObjectMeta{Name: string(name), Namespace: string(namespace)}}
	if err := api.client.Delete(r.Context(), server); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Console

func (api *controlPlaneAPI) GetConsole(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name) {
	api.serveConsole(w, r, string(namespace), string(name))
}

// Actions

func (api *controlPlaneAPI) ListActions(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name) {
	server := &operatorv1.GameServer{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: string(name), Namespace: string(namespace)}, server); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	profile := &operatorv1.GameProfile{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: server.Spec.Profile}, profile); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	respondJSON(w, http.StatusOK, profile.Spec.Actions)
}

func (api *controlPlaneAPI) ExecuteAction(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name, action string) {
	ns, serverName := string(namespace), string(name)

	var params map[string]string
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		params = map[string]string{}
	}

	user := auth.GetUser(r.Context())
	caller := r.Header.Get("X-User")
	if caller == "" {
		caller = "anonymous"
	}
	if user != nil && user.Source != "none" {
		caller = user.Username
	}

	exec := &operatorv1.ActionExecution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%s-%d", serverName, action, time.Now().Unix()),
			Namespace: ns,
		},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef: operatorv1.TargetRef{
				APIVersion: "operator.minato.io/v1",
				Kind:       "GameServer",
				Name:       serverName,
				Namespace:  ns,
			},
			ActionName: action,
			Params:     params,
			Caller:     caller,
		},
	}

	if err := api.client.Create(r.Context(), exec); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"name": exec.Name})
}

func (api *controlPlaneAPI) GetActionExecution(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, _ oapi.Name, executionId string) {
	exec := &operatorv1.ActionExecution{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: executionId, Namespace: string(namespace)}, exec); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, exec)
}

// Snapshots

func (api *controlPlaneAPI) ListSnapshots(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name) {
	ns, serverName := string(namespace), string(name)

	var list operatorv1.GameSnapshotList
	if err := api.client.List(r.Context(), &list,
		client.InNamespace(ns),
		client.MatchingFields{"spec.gameServerRef": serverName},
	); err != nil {
		// Fallback: list all and filter
		if err := api.client.List(r.Context(), &list, client.InNamespace(ns)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		filtered := []operatorv1.GameSnapshot{}
		for _, snap := range list.Items {
			if snap.Spec.GameServerRef == serverName {
				filtered = append(filtered, snap)
			}
		}
		respondJSON(w, http.StatusOK, filtered)
		return
	}
	respondJSON(w, http.StatusOK, list.Items)
}

func (api *controlPlaneAPI) CreateSnapshot(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name) {
	ns, serverName := string(namespace), string(name)

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-snap-%d", serverName, time.Now().Unix()),
			Namespace: ns,
		},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: serverName,
		},
	}

	if err := api.client.Create(r.Context(), snap); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusCreated, snap)
}

// Fleets

func (api *controlPlaneAPI) ListGameServerFleets(w http.ResponseWriter, r *http.Request) {
	var list operatorv1.GameServerFleetList
	if err := api.client.List(r.Context(), &list); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, list.Items)
}

func (api *controlPlaneAPI) GetGameServerFleet(w http.ResponseWriter, r *http.Request, namespace oapi.Namespace, name oapi.Name) {
	fleet := &operatorv1.GameServerFleet{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: string(name), Namespace: string(namespace)}, fleet); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, fleet)
}

// Profiles

func (api *controlPlaneAPI) ListProfiles(w http.ResponseWriter, r *http.Request) {
	var list operatorv1.GameProfileList
	if err := api.client.List(r.Context(), &list); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, list.Items)
}

func (api *controlPlaneAPI) GetProfile(w http.ResponseWriter, r *http.Request, name string) {
	profile := &operatorv1.GameProfile{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: name}, profile); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	respondJSON(w, http.StatusOK, profile)
}

// API Keys

func (api *controlPlaneAPI) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := api.keyStorage.ListKeys(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, keys)
}

func (api *controlPlaneAPI) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	// Only authenticated users can generate API keys
	user := auth.GetUser(r.Context())
	if user == nil {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("name is required"))
		return
	}

	// Default role to the user's role if not specified
	role := req.Role
	if role == "" {
		role = user.Role
	}

	// Generate the key
	entry, keyValue, err := api.keyStorage.GenerateKey(r.Context(), req.Name, user.ID, user.Username, role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Return the key value ONCE - it will never be shown again
	respondJSON(w, http.StatusCreated, map[string]any{
		"name":      entry.Name,
		"role":      entry.Role,
		"createdAt": entry.CreatedAt,
		"key":       keyValue, // One-time display
		"warning":   "This key will never be shown again. Store it securely.",
	})
}

func (api *controlPlaneAPI) DeleteAPIKey(w http.ResponseWriter, r *http.Request, keyId string) {
	if err := api.keyStorage.DeleteKey(r.Context(), keyId); err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helpers

func respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeError emits the standard error envelope: {"error": "..."}.
func writeError(w http.ResponseWriter, status int, err error) {
	respondJSON(w, status, map[string]string{"error": err.Error()})
}

// securityHeadersMiddleware adds security headers to all HTTP responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		next.ServeHTTP(w, r)
	})
}

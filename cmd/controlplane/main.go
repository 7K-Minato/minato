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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorv1 "github.com/7k-minato/minato/api/operator/v1"
	"github.com/7k-minato/minato/internal/controlplane/audit"
	"github.com/7k-minato/minato/internal/controlplane/auth"
	"github.com/7k-minato/minato/internal/controlplane/rbac"
)

func main() {
	cfg := config.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{})
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
	r.Use(middleware.Timeout(30 * time.Second))
	r.Use(securityHeadersMiddleware)
	r.Use(middleware.RequestSize(10 * 1024 * 1024)) // 10MB max request size
	r.Use(audit.Middleware())
	r.Use(auth.Middleware(authChain))

	api := &controlPlaneAPI{client: c, authCfg: authCfg, keyStorage: keyStorage}

	// Health endpoints (always public)
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Auth configuration endpoint (always public, used by UI for discovery)
	r.Get("/auth/config", api.getAuthConfig)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// GameServers - viewer+ can read
		r.Get("/gameservers", api.listGameServers)
		r.Get("/gameservers/{namespace}/{name}", api.getGameServer)

		// GameServers - admin only for write
		r.With(rbac.RequireRole("admin")).
			Post("/gameservers/{namespace}", api.createGameServer)
		r.With(rbac.RequireRole("admin")).
			Delete("/gameservers/{namespace}/{name}", api.deleteGameServer)

		// Actions - viewer can list, operator+ can execute
		r.Get("/gameservers/{namespace}/{name}/actions", api.listActions)
		r.With(rbac.RequireRole("operator", "admin")).
			Post("/gameservers/{namespace}/{name}/actions/{action}", api.executeAction)
		r.Get("/gameservers/{namespace}/{name}/actions/{executionId}", api.getActionExecution)

		// Snapshots - viewer can list, operator+ can create
		r.Get("/gameservers/{namespace}/{name}/snapshots", api.listSnapshots)
		r.With(rbac.RequireRole("operator", "admin")).
			Post("/gameservers/{namespace}/{name}/snapshots", api.createSnapshot)

		// Console (WebSocket) - operator+
		// TODO: Implement WebSocket proxy to agent gRPC console stream
		// r.With(rbac.RequireRole("operator", "admin")).
		// 	Get("/gameservers/{namespace}/{name}/console", api.handleConsole)

		// Fleets - viewer+ can read, admin can write
		r.Get("/gameserverfleets", api.listGameServerFleets)
		r.Get("/gameserverfleets/{namespace}/{name}", api.getGameServerFleet)

		// Profiles - viewer+ can read
		r.Get("/profiles", api.listProfiles)
		r.Get("/profiles/{name}", api.getProfile)

		// API Keys - admin only
		r.With(rbac.RequireRole("admin")).
			Get("/apikeys", api.listAPIKeys)
		r.With(rbac.RequireRole("admin")).
			Post("/apikeys", api.createAPIKey)
		r.With(rbac.RequireRole("admin")).
			Delete("/apikeys/{keyId}", api.deleteAPIKey)
	})

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

func (api *controlPlaneAPI) listGameServers(w http.ResponseWriter, r *http.Request) {
	var list operatorv1.GameServerList
	if err := api.client.List(r.Context(), &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, list.Items)
}

func (api *controlPlaneAPI) getGameServer(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	server := &operatorv1.GameServer{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: ns}, server); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondJSON(w, server)
}

func (api *controlPlaneAPI) createGameServer(w http.ResponseWriter, r *http.Request) {
	var server operatorv1.GameServer
	if err := json.NewDecoder(r.Body).Decode(&server); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	server.Namespace = chi.URLParam(r, "namespace")
	if err := api.client.Create(r.Context(), &server); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, server)
}

func (api *controlPlaneAPI) deleteGameServer(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	server := &operatorv1.GameServer{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
	if err := api.client.Delete(r.Context(), server); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *controlPlaneAPI) listActions(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	server := &operatorv1.GameServer{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: ns}, server); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	profile := &operatorv1.GameProfile{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: server.Spec.Profile}, profile); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	respondJSON(w, profile.Spec.Actions)
}

func (api *controlPlaneAPI) executeAction(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")
	actionName := chi.URLParam(r, "action")

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
			Name:      fmt.Sprintf("%s-%s-%d", name, actionName, time.Now().Unix()),
			Namespace: ns,
		},
		Spec: operatorv1.ActionExecutionSpec{
			TargetRef: operatorv1.TargetRef{
				APIVersion: "operator.minato.io/v1",
				Kind:       "GameServer",
				Name:       name,
				Namespace:  ns,
			},
			ActionName: actionName,
			Params:     params,
			Caller:     caller,
		},
	}

	if err := api.client.Create(r.Context(), exec); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, map[string]string{"name": exec.Name})
}

func (api *controlPlaneAPI) getActionExecution(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	exec := &operatorv1.ActionExecution{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: ns}, exec); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondJSON(w, exec)
}

func (api *controlPlaneAPI) listSnapshots(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	var list operatorv1.GameSnapshotList
	if err := api.client.List(r.Context(), &list,
		client.InNamespace(ns),
		client.MatchingFields{"spec.gameServerRef": name},
	); err != nil {
		// Fallback: list all and filter
		if err := api.client.List(r.Context(), &list, client.InNamespace(ns)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		var filtered []operatorv1.GameSnapshot
		for _, snap := range list.Items {
			if snap.Spec.GameServerRef == name {
				filtered = append(filtered, snap)
			}
		}
		respondJSON(w, filtered)
		return
	}
	respondJSON(w, list.Items)
}

func (api *controlPlaneAPI) createSnapshot(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	snap := &operatorv1.GameSnapshot{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-snap-%d", name, time.Now().Unix()),
			Namespace: ns,
		},
		Spec: operatorv1.GameSnapshotSpec{
			GameServerRef: name,
		},
	}

	if err := api.client.Create(r.Context(), snap); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	respondJSON(w, snap)
}

func (api *controlPlaneAPI) listGameServerFleets(w http.ResponseWriter, r *http.Request) {
	var list operatorv1.GameServerFleetList
	if err := api.client.List(r.Context(), &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, list.Items)
}

func (api *controlPlaneAPI) getGameServerFleet(w http.ResponseWriter, r *http.Request) {
	ns := chi.URLParam(r, "namespace")
	name := chi.URLParam(r, "name")

	fleet := &operatorv1.GameServerFleet{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: name, Namespace: ns}, fleet); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondJSON(w, fleet)
}

func (api *controlPlaneAPI) listProfiles(w http.ResponseWriter, r *http.Request) {
	var list operatorv1.GameProfileList
	if err := api.client.List(r.Context(), &list); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, list.Items)
}

func (api *controlPlaneAPI) getProfile(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")

	profile := &operatorv1.GameProfile{}
	if err := api.client.Get(r.Context(), types.NamespacedName{Name: name}, profile); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondJSON(w, profile)
}

// API Key management endpoints

func (api *controlPlaneAPI) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := api.keyStorage.ListKeys(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, keys)
}

func (api *controlPlaneAPI) createAPIKey(w http.ResponseWriter, r *http.Request) {
	// Only authenticated users can generate API keys
	user := auth.GetUser(r.Context())
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Return the key value ONCE - it will never be shown again
	respondJSON(w, map[string]any{
		"name":      entry.Name,
		"role":      entry.Role,
		"createdAt": entry.CreatedAt,
		"key":       keyValue, // One-time display
		"warning":   "This key will never be shown again. Store it securely.",
	})
}

func (api *controlPlaneAPI) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "keyId")
	if err := api.keyStorage.DeleteKey(r.Context(), name); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (api *controlPlaneAPI) getAuthConfig(w http.ResponseWriter, r *http.Request) {
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

	config := map[string]any{
		"authModes":    modes,
		"basicEnabled": api.authCfg.Basic.Enabled,
	}

	if api.authCfg.OIDC.Enabled && api.authCfg.OIDC.IssuerURL != "" {
		config["oidcIssuer"] = api.authCfg.OIDC.IssuerURL
	}

	respondJSON(w, config)
}

func respondJSON(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
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

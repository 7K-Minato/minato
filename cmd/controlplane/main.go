package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	operatorv1 "github.com/7k-group/minato/api/operator/v1"
)

func main() {
	cfg := config.GetConfigOrDie()
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		log.Fatalf("failed to create k8s client: %v", err)
	}

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	api := &controlPlaneAPI{client: c}

	// Health endpoints
	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// GameServers
		r.Get("/gameservers", api.listGameServers)
		r.Get("/gameservers/{namespace}/{name}", api.getGameServer)
		r.Post("/gameservers/{namespace}", api.createGameServer)
		r.Delete("/gameservers/{namespace}/{name}", api.deleteGameServer)

		// Actions
		r.Get("/gameservers/{namespace}/{name}/actions", api.listActions)
		r.Post("/gameservers/{namespace}/{name}/actions/{action}", api.executeAction)
		r.Get("/gameservers/{namespace}/{name}/actions/{executionId}", api.getActionExecution)

		// Snapshots
		r.Get("/gameservers/{namespace}/{name}/snapshots", api.listSnapshots)
		r.Post("/gameservers/{namespace}/{name}/snapshots", api.createSnapshot)

		// Console (WebSocket)
		r.Get("/gameservers/{namespace}/{name}/console", api.handleConsole)

		// Fleets
		r.Get("/gameserverfleets", api.listGameServerFleets)
		r.Get("/gameserverfleets/{namespace}/{name}", api.getGameServerFleet)

		// Profiles
		r.Get("/profiles", api.listProfiles)
		r.Get("/profiles/{name}", api.getProfile)
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
	client client.Client
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
			Caller:     r.Header.Get("X-User"),
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

func respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

// serverSummary is the per-GameServer snapshot pushed to minato cloud. It is
// intentionally decoupled from the operator CRD types: the registrar only
// needs these fields, and decoding into a plain struct keeps the push
// resilient to unrelated CRD schema changes.
type serverSummary struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Profile        string `json:"profile"`
	State          string `json:"state"`
	Players        int32  `json:"players"`
	PlayerCapacity int32  `json:"playerCapacity"`
}

// gameServerJSON mirrors the JSON shape of operatorv1.GameServer used by the
// control plane's GET /api/v1/gameservers response.
type gameServerJSON struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		Profile string `json:"profile"`
	} `json:"spec"`
	Status struct {
		State          string `json:"state"`
		Players        int32  `json:"players"`
		PlayerCapacity int32  `json:"playerCapacity"`
	} `json:"status"`
}

type metricsPushPayload struct {
	CollectedAt string          `json:"collectedAt"`
	Servers     []serverSummary `json:"servers"`
}

// fetchGameServerSummaries lists GameServers from the local control plane
// and maps them to push summaries.
func (r *registrar) fetchGameServerSummaries(ctx context.Context) ([]serverSummary, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.cfg.ControlPlaneURL+"/api/v1/gameservers", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.cfg.ControlPlaneKey)

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("list gameservers: status %d: %s", resp.StatusCode, body)
	}

	var servers []gameServerJSON
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return nil, fmt.Errorf("list gameservers: decode: %w", err)
	}

	summaries := make([]serverSummary, 0, len(servers))
	for _, s := range servers {
		summaries = append(summaries, serverSummary{
			Name:           s.Metadata.Name,
			Namespace:      s.Metadata.Namespace,
			Profile:        s.Spec.Profile,
			State:          s.Status.State,
			Players:        s.Status.Players,
			PlayerCapacity: s.Status.PlayerCapacity,
		})
	}
	return summaries, nil
}

// buildMetricsPushPayload builds the POST body for the cloud metrics
// endpoint.
func buildMetricsPushPayload(now time.Time, servers []serverSummary) metricsPushPayload {
	if servers == nil {
		servers = []serverSummary{}
	}
	return metricsPushPayload{
		CollectedAt: now.UTC().Format(time.RFC3339),
		Servers:     servers,
	}
}

// pushMetrics collects GameServer summaries and pushes them to minato cloud.
// A 404 means the cloud deployment does not have the metrics endpoint yet;
// that is tolerated and reported as result "unsupported".
func (r *registrar) pushMetrics(ctx context.Context) error {
	servers, err := r.fetchGameServerSummaries(ctx)
	if err != nil {
		metricsPushTotal.WithLabelValues("error").Inc()
		return err
	}

	status, body, err := r.call(ctx, "/api/v1/clusters/"+r.cfg.Name+"/metrics",
		buildMetricsPushPayload(time.Now(), servers))
	switch {
	case err != nil:
		metricsPushTotal.WithLabelValues("error").Inc()
		return err
	case status == http.StatusNotFound:
		metricsPushTotal.WithLabelValues("unsupported").Inc()
		log.Printf("cloud %s does not support the metrics endpoint (404); skipping", r.cfg.CloudURL)
		return nil
	case status != http.StatusNoContent:
		metricsPushTotal.WithLabelValues("error").Inc()
		return fmt.Errorf("metrics push: status %d: %s", status, body)
	}
	metricsPushTotal.WithLabelValues("ok").Inc()
	return nil
}

// runMetricsPush runs the push loop on its own ticker. Failures are logged
// and counted but never affect the heartbeat loop.
func (r *registrar) runMetricsPush(ctx context.Context) {
	if r.cfg.ControlPlaneURL == "" || r.cfg.ControlPlaneKey == "" {
		log.Printf("CONTROLPLANE_URL or CONTROLPLANE_API_KEY empty, metrics push disabled")
		return
	}
	ticker := time.NewTicker(r.cfg.MetricsPushInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		pushCtx, cancel := context.WithTimeout(ctx, r.cfg.RequestTimeout)
		if err := r.pushMetrics(pushCtx); err != nil {
			log.Printf("metrics push failed: %v", err)
		}
		cancel()
	}
}

// Command registrar runs alongside the minato control plane in a cluster and
// registers the cluster with minato cloud, then keeps it alive with periodic
// heartbeats. If the cloud forgets the cluster (404 on heartbeat), it
// re-registers automatically.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

type config struct {
	CloudURL           string
	RegisterToken      string
	Name               string
	Region             string
	ControlPlaneURL    string
	ControlPlaneKey    string
	CapacityMax        int
	DedicatedTenantID  string
	ClusterFingerprint string
	HeartbeatInterval  time.Duration
	RequestTimeout     time.Duration
}

func loadConfig() (config, error) {
	var c config
	c.CloudURL = os.Getenv("CLOUD_URL")
	c.RegisterToken = os.Getenv("REGISTER_TOKEN")
	c.Name = os.Getenv("CLUSTER_NAME")
	c.Region = os.Getenv("CLUSTER_REGION")
	c.ControlPlaneURL = envStr("CONTROLPLANE_URL", "http://minato-controlplane:8080")
	c.ControlPlaneKey = os.Getenv("CONTROLPLANE_API_KEY")
	c.DedicatedTenantID = os.Getenv("DEDICATED_TENANT_ID")
	c.ClusterFingerprint = os.Getenv("CLUSTER_FINGERPRINT")
	c.CapacityMax = envInt("CAPACITY_MAX", 100)
	c.HeartbeatInterval = envDuration("HEARTBEAT_INTERVAL", 30*time.Second)
	c.RequestTimeout = envDuration("REQUEST_TIMEOUT", 10*time.Second)

	if c.CloudURL == "" || c.RegisterToken == "" || c.Name == "" || c.ControlPlaneKey == "" {
		return c, fmt.Errorf("CLOUD_URL, REGISTER_TOKEN, CLUSTER_NAME and CONTROLPLANE_API_KEY are required")
	}
	return c, nil
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if v, err := time.ParseDuration(os.Getenv(key)); err == nil && v > 0 {
		return v
	}
	return def
}

type registrar struct {
	cfg         config
	fingerprint string
	client      *http.Client
}

type registerPayload struct {
	Name               string `json:"name"`
	Region             string `json:"region"`
	ControlPlaneURL    string `json:"controlplaneUrl"`
	APIKey             string `json:"apiKey"`
	CapacityMax        int    `json:"capacityMax"`
	DedicatedTenantID  string `json:"dedicatedTenantId"`
	ClusterFingerprint string `json:"clusterFingerprint,omitempty"`
}

type heartbeatPayload struct {
	CapacityMax        int    `json:"capacityMax"`
	ClusterFingerprint string `json:"clusterFingerprint,omitempty"`
}

func buildRegisterPayload(cfg config, fingerprint string) registerPayload {
	return registerPayload{
		Name:               cfg.Name,
		Region:             cfg.Region,
		ControlPlaneURL:    cfg.ControlPlaneURL,
		APIKey:             cfg.ControlPlaneKey,
		CapacityMax:        cfg.CapacityMax,
		DedicatedTenantID:  cfg.DedicatedTenantID,
		ClusterFingerprint: fingerprint,
	}
}

func buildHeartbeatPayload(cfg config, fingerprint string) heartbeatPayload {
	return heartbeatPayload{
		CapacityMax:        cfg.CapacityMax,
		ClusterFingerprint: fingerprint,
	}
}

func (r *registrar) call(ctx context.Context, path string, body any) (int, []byte, error) {
	buf, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.cfg.CloudURL+path, bytes.NewReader(buf))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+r.cfg.RegisterToken)

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, respBody, nil
}

func (r *registrar) register(ctx context.Context) error {
	status, body, err := r.call(ctx, "/api/v1/clusters/register", buildRegisterPayload(r.cfg, r.fingerprint))
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("register: status %d: %s", status, body)
	}
	return nil
}

func (r *registrar) heartbeat(ctx context.Context) (bool, error) {
	status, body, err := r.call(ctx, "/api/v1/clusters/"+r.cfg.Name+"/heartbeat", buildHeartbeatPayload(r.cfg, r.fingerprint))
	if err != nil {
		return false, err
	}
	if status == http.StatusNotFound {
		return false, nil
	}
	if status != http.StatusNoContent {
		return false, fmt.Errorf("heartbeat: status %d: %s", status, body)
	}
	return true, nil
}

func (r *registrar) run(ctx context.Context) {
	// Initial registration with backoff until the cloud accepts us.
	backoff := time.Second
	for {
		if err := r.register(ctx); err != nil {
			log.Printf("register failed: %v (retrying in %s)", err, backoff)
		} else {
			log.Printf("cluster %q registered with %s", r.cfg.Name, r.cfg.CloudURL)
			break
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}

	ticker := time.NewTicker(r.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		beatCtx, cancel := context.WithTimeout(ctx, r.cfg.RequestTimeout)
		ok, err := r.heartbeat(beatCtx)
		cancel()
		switch {
		case err != nil:
			log.Printf("heartbeat failed: %v", err)
		case !ok:
			log.Printf("cluster unknown to cloud, re-registering")
			if err := r.register(ctx); err != nil {
				log.Printf("re-register failed: %v", err)
			} else {
				log.Printf("cluster %q re-registered", r.cfg.Name)
			}
		default:
			log.Printf("heartbeat ok")
		}
	}
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fingerprint := resolveClusterFingerprint(cfg.ClusterFingerprint, defaultTokenPath, defaultCAPath)
	if fingerprint != "" {
		log.Printf("cluster fingerprint: %s", fingerprint)
	}

	r := &registrar{cfg: cfg, fingerprint: fingerprint, client: &http.Client{Timeout: cfg.RequestTimeout}}
	r.run(ctx)
}

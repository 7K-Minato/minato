package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

const (
	defaultTokenPath = "/var/run/secrets/kubernetes.io/serviceaccount/token"
	defaultCAPath    = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
)

// resolveClusterFingerprint returns a stable identifier for this physical
// cluster. Precedence: explicit override (env CLUSTER_FINGERPRINT) >
// kube-system namespace UID > empty string. It never fails — the fingerprint
// is best-effort so registration must work out of cluster too.
func resolveClusterFingerprint(override, tokenPath, caPath string) string {
	if override != "" {
		return override
	}
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		log.Printf("cluster fingerprint unavailable (no service account token: %v); continuing without it", err)
		return ""
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if ca, err := os.ReadFile(caPath); err == nil {
		if pool := x509.NewCertPool(); pool.AppendCertsFromPEM(ca) {
			tlsCfg.RootCAs = pool
		}
	}
	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	uid, err := kubeSystemUID(ctx, client, inClusterBaseURL(), string(token))
	if err != nil {
		log.Printf("cluster fingerprint unavailable (%v); continuing without it", err)
		return ""
	}
	return uid
}

// inClusterBaseURL builds the API server URL from the standard in-cluster
// env, defaulting to the kubernetes service DNS name.
func inClusterBaseURL() string {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if host == "" {
		host = "kubernetes.default.svc"
	}
	port := os.Getenv("KUBERNETES_SERVICE_PORT")
	if port == "" {
		port = "443"
	}
	return "https://" + net.JoinHostPort(host, port)
}

// kubeSystemUID fetches the UID of the kube-system namespace via the
// Kubernetes API using a plain HTTP client — no client-go dependency.
func kubeSystemUID(ctx context.Context, client *http.Client, baseURL, token string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/api/v1/namespaces/kube-system", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get kube-system namespace: status %d", resp.StatusCode)
	}
	var ns struct {
		Metadata struct {
			UID string `json:"uid"`
		} `json:"metadata"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ns); err != nil {
		return "", err
	}
	if ns.Metadata.UID == "" {
		return "", fmt.Errorf("kube-system namespace has no uid")
	}
	return ns.Metadata.UID, nil
}

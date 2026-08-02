package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const defaultCloudURL = "http://localhost:8080"

// cloudConfig is the persisted state of `minato-ctl cloud`, stored in
// ~/.config/minato/config.json (mode 0600).
type cloudConfig struct {
	Cloud struct {
		URL          string `json:"url,omitempty"`
		APIKey       string `json:"api_key,omitempty"`
		SessionToken string `json:"session_token,omitempty"`
	} `json:"cloud"`
}

// cloudConfigPath is a variable so tests can point it at a temp file.
var cloudConfigPath = defaultCloudConfigPath()

func defaultCloudConfigPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = filepath.Join(os.Getenv("HOME"), ".config")
	}
	return filepath.Join(dir, "minato", "config.json")
}

func loadCloudConfig() (*cloudConfig, error) {
	cfg := &cloudConfig{}
	data, err := os.ReadFile(cloudConfigPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("read config %s: %w", cloudConfigPath, err)
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", cloudConfigPath, err)
	}
	return cfg, nil
}

func saveCloudConfig(cfg *cloudConfig) error {
	if err := os.MkdirAll(filepath.Dir(cloudConfigPath), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cloudConfigPath, data, 0o600)
}

// resolveCloudURL implements precedence: --url flag > MINATO_CLOUD_URL >
// stored config > default.
func resolveCloudURL(cfg *cloudConfig) string {
	if cloudURL != "" {
		return cloudURL
	}
	if v := os.Getenv("MINATO_CLOUD_URL"); v != "" {
		return v
	}
	if cfg.Cloud.URL != "" {
		return cfg.Cloud.URL
	}
	return defaultCloudURL
}

// resolveCloudToken returns the credential and how it was obtained:
// MINATO_CLOUD_API_KEY > stored API key > stored session token.
func resolveCloudToken(cfg *cloudConfig) (token, mode string) {
	if v := os.Getenv("MINATO_CLOUD_API_KEY"); v != "" {
		return v, "api-key (env)"
	}
	if cfg.Cloud.APIKey != "" {
		return cfg.Cloud.APIKey, "api-key"
	}
	if cfg.Cloud.SessionToken != "" {
		return cfg.Cloud.SessionToken, "session"
	}
	return "", "none"
}

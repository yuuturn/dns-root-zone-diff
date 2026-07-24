package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.ZoneURL != "https://www.internic.net/domain/root.zone" {
		t.Errorf("ZoneURL = %q, want internic URL", cfg.ZoneURL)
	}
	if cfg.FetchInterval != 6*time.Hour {
		t.Errorf("FetchInterval = %v, want 6h", cfg.FetchInterval)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, want ./data", cfg.DataDir)
	}
}

func TestLoadFromFile(t *testing.T) {
	content := `
zone_url: "https://example.com/root.zone"
fetch_interval: "1h"
data_dir: "/tmp/zones"
slack:
  enabled: true
  webhook_url: "https://hooks.slack.com/services/T00/B00/XXX"
twitter:
  enabled: true
  api_key: "key"
  api_secret: "secret"
  access_token: "token"
  access_secret: "tokensecret"
  oauth2_access_token: "oauth2token"
  oauth2_refresh_token: "refreshtoken"
  oauth2_client_id: "clientid"
  oauth2_client_secret: "clientsecret"
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ZoneURL != "https://example.com/root.zone" {
		t.Errorf("ZoneURL = %q", cfg.ZoneURL)
	}
	if cfg.FetchInterval != time.Hour {
		t.Errorf("FetchInterval = %v", cfg.FetchInterval)
	}
	if !cfg.Slack.Enabled {
		t.Error("Slack.Enabled = false, want true")
	}
	if cfg.Slack.WebhookURL != "https://hooks.slack.com/services/T00/B00/XXX" {
		t.Errorf("Slack.WebhookURL = %q", cfg.Slack.WebhookURL)
	}
	if !cfg.Twitter.Enabled {
		t.Error("Twitter.Enabled = false, want true")
	}
	if cfg.Twitter.OAuth2AccessToken != "oauth2token" {
		t.Errorf("Twitter.OAuth2AccessToken = %q", cfg.Twitter.OAuth2AccessToken)
	}
	if cfg.Twitter.OAuth2ClientID != "clientid" {
		t.Errorf("Twitter.OAuth2ClientID = %q", cfg.Twitter.OAuth2ClientID)
	}
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("zone_url: https://file.example.com\n"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("DNS_ROOT_DIFF_ZONE_URL", "https://env.example.com")
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.com/env")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.ZoneURL != "https://env.example.com" {
		t.Errorf("ZoneURL = %q, want env override", cfg.ZoneURL)
	}
	if cfg.Slack.WebhookURL != "https://hooks.slack.com/env" {
		t.Errorf("Slack.WebhookURL = %q, want env override", cfg.Slack.WebhookURL)
	}
}

func TestLoadNoFile(t *testing.T) {
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load(\"\") error = %v", err)
	}
	if cfg.ZoneURL != "https://www.internic.net/domain/root.zone" {
		t.Errorf("ZoneURL = %q, want default", cfg.ZoneURL)
	}
}

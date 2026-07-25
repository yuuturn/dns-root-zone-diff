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
	if cfg.FetchInterval != 2*time.Hour {
		t.Errorf("FetchInterval = %v, want 2h", cfg.FetchInterval)
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

func TestSaveOAuth2Tokens(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
zone_url: "https://example.com/root.zone"
fetch_interval: 1h0m0s
data_dir: "/tmp/zones"
twitter:
  enabled: true
  oauth2_access_token: "old-access"
  oauth2_refresh_token: "old-refresh"
  oauth2_client_id: "clientid"
  oauth2_client_secret: "clientsecret"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveOAuth2Tokens(path, "new-access", "new-refresh"); err != nil {
		t.Fatalf("SaveOAuth2Tokens() error = %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Twitter.OAuth2AccessToken != "new-access" {
		t.Errorf("OAuth2AccessToken = %q, want new-access", cfg.Twitter.OAuth2AccessToken)
	}
	if cfg.Twitter.OAuth2RefreshToken != "new-refresh" {
		t.Errorf("OAuth2RefreshToken = %q, want new-refresh", cfg.Twitter.OAuth2RefreshToken)
	}
	if cfg.Twitter.OAuth2ClientID != "clientid" {
		t.Errorf("OAuth2ClientID = %q, want clientid", cfg.Twitter.OAuth2ClientID)
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

func TestDefaultWeb(t *testing.T) {
	cfg := Default()
	if cfg.Web.Enabled {
		t.Error("Web.Enabled = true, want false by default")
	}
	if cfg.Web.Listen != "127.0.0.1:8080" {
		t.Errorf("Web.Listen = %q, want 127.0.0.1:8080", cfg.Web.Listen)
	}
}

func TestLoadWebSection(t *testing.T) {
	content := `
web:
  enabled: true
  listen: "0.0.0.0:9090"
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
	if !cfg.Web.Enabled {
		t.Error("Web.Enabled = false, want true")
	}
	if cfg.Web.Listen != "0.0.0.0:9090" {
		t.Errorf("Web.Listen = %q", cfg.Web.Listen)
	}
}

func TestLoadWebListenDefaulted(t *testing.T) {
	content := `
web:
  enabled: true
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
	if cfg.Web.Listen != "127.0.0.1:8080" {
		t.Errorf("Web.Listen = %q, want defaulted 127.0.0.1:8080", cfg.Web.Listen)
	}
}

func TestWebEnvOverride(t *testing.T) {
	t.Setenv("DNS_ROOT_DIFF_WEB_ENABLED", "true")
	t.Setenv("DNS_ROOT_DIFF_WEB_LISTEN", "127.0.0.1:3000")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Web.Enabled {
		t.Error("Web.Enabled = false, want true from env")
	}
	if cfg.Web.Listen != "127.0.0.1:3000" {
		t.Errorf("Web.Listen = %q", cfg.Web.Listen)
	}
}

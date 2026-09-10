package config

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSaveOAuth2TokensPreservesCommentsAndOmittedKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `# managed by ops: do not edit manually
zone_url: "https://example.com/root.zone"
# anchor monitoring intentionally disabled (key omitted)
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
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "managed by ops") {
		t.Errorf("comment lost after token save:\n%s", data)
	}
	// 未指定キーにデフォルト値が追記されないこと (anchor_url は省略されたまま)。
	if strings.Contains(string(data), "anchor_url") {
		t.Errorf("omitted anchor_url was written by token save:\n%s", data)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Twitter.OAuth2AccessToken != "new-access" {
		t.Errorf("OAuth2AccessToken = %q, want new-access", cfg.Twitter.OAuth2AccessToken)
	}
	if cfg.Twitter.OAuth2RefreshToken != "new-refresh" {
		t.Errorf("OAuth2RefreshToken = %q, want new-refresh", cfg.Twitter.OAuth2RefreshToken)
	}
	if cfg.Twitter.OAuth2ClientID != "clientid" {
		t.Errorf("OAuth2ClientID = %q, want clientid preserved", cfg.Twitter.OAuth2ClientID)
	}
}

func TestSaveOAuth2TokensKeepsExplicitEmptyAnchorURL(t *testing.T) {
	// anchor_url: "" を明示した構成 (root anchors 監視無効) は保存後も空のまま。
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `anchor_url: ""
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
		t.Fatal(err)
	}
	if cfg.AnchorURL != "" {
		t.Errorf("AnchorURL = %q, want empty (anchors monitoring must stay disabled)", cfg.AnchorURL)
	}
}

func TestSaveOAuth2TokensPreservesInlineComments(t *testing.T) {
	// トークン行の行末インラインコメントは yaml.v3 では値ノードの LineComment に
	// 保持される。setString がノードを置換する際にコメントを転写しないと失われる。
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `twitter:
  enabled: true
  oauth2_access_token: "old-access" # managed by ops
  oauth2_refresh_token: "old-refresh" # keep me
  oauth2_client_id: "clientid"
  oauth2_client_secret: "clientsecret"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveOAuth2Tokens(path, "new-access", "new-refresh"); err != nil {
		t.Fatalf("SaveOAuth2Tokens() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "# managed by ops") {
		t.Errorf("inline comment on access token line lost:\n%s", data)
	}
	if !strings.Contains(string(data), "# keep me") {
		t.Errorf("inline comment on refresh token line lost:\n%s", data)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Twitter.OAuth2AccessToken != "new-access" || cfg.Twitter.OAuth2RefreshToken != "new-refresh" {
		t.Errorf("tokens not updated: access=%q refresh=%q", cfg.Twitter.OAuth2AccessToken, cfg.Twitter.OAuth2RefreshToken)
	}
}

func TestSaveOAuth2TokensKeepsRefreshWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `twitter:
  enabled: true
  oauth2_access_token: "old-access"
  oauth2_refresh_token: "old-refresh"
  oauth2_client_id: "clientid"
  oauth2_client_secret: "clientsecret"
`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	if err := SaveOAuth2Tokens(path, "new-access", ""); err != nil {
		t.Fatalf("SaveOAuth2Tokens() error = %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Twitter.OAuth2AccessToken != "new-access" {
		t.Errorf("OAuth2AccessToken = %q, want new-access", cfg.Twitter.OAuth2AccessToken)
	}
	if cfg.Twitter.OAuth2RefreshToken != "old-refresh" {
		t.Errorf("OAuth2RefreshToken = %q, want old-refresh preserved", cfg.Twitter.OAuth2RefreshToken)
	}
}

func TestLoadFetchIntervalDefaulted(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"zero is defaulted", "fetch_interval: 0h\n"},
		{"negative is defaulted", "fetch_interval: -1h\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.FetchInterval != 2*time.Hour {
				t.Errorf("FetchInterval = %v, want default 2h", cfg.FetchInterval)
			}
		})
	}
}

func TestLoadFetchIntervalEnvZeroDefaulted(t *testing.T) {
	// DNS_ROOT_DIFF_INTERVAL=0s はパースに成功するため 0 が入り、
	// runLoop の time.NewTicker が panic する。Load でデフォルトに補正する。
	t.Setenv("DNS_ROOT_DIFF_INTERVAL", "0s")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.FetchInterval != 2*time.Hour {
		t.Errorf("FetchInterval = %v, want default 2h", cfg.FetchInterval)
	}
}

func TestSaveOAuth2TokensRejectsDuplicateKeys(t *testing.T) {
	// yaml.v3 は struct への unmarshal (Load) では重複マッピングキーをエラーにするが、
	// Node への unmarshal (SaveOAuth2Tokens) では検出しない (先頭のキーが勝つ)。
	// トークン更新が Load に反映されず無言で消えるのを防ぐため、
	// SaveOAuth2Tokens も Load と同じ判定 (重複キーはエラー) にする。
	tests := []struct {
		name    string
		content string
	}{
		{"top-level", "twitter:\n  enabled: true\ntwitter:\n  enabled: true\n"},
		{"nested", "twitter:\n  enabled: true\n  oauth2_access_token: \"a\"\n  oauth2_access_token: \"b\"\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0600); err != nil {
				t.Fatal(err)
			}
			if err := SaveOAuth2Tokens(path, "new-access", "new-refresh"); err == nil {
				t.Error("SaveOAuth2Tokens() = nil, want error on duplicate keys")
			}
			if _, err := Load(path); err == nil {
				t.Error("Load() = nil, want error on duplicate keys")
			}
		})
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

func TestTwitterMaxPosts(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"default when omitted", "twitter:\n  enabled: true\n", defaultTwitterMaxPosts},
		{"from file", "twitter:\n  enabled: true\n  max_posts: 6\n", 6},
		{"zero is defaulted", "twitter:\n  enabled: true\n  max_posts: 0\n", defaultTwitterMaxPosts},
		{"negative is defaulted", "twitter:\n  enabled: true\n  max_posts: -3\n", defaultTwitterMaxPosts},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(tt.content), 0644); err != nil {
				t.Fatal(err)
			}
			cfg, err := Load(path)
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if cfg.Twitter.MaxPosts != tt.want {
				t.Errorf("Twitter.MaxPosts = %d, want %d", cfg.Twitter.MaxPosts, tt.want)
			}
		})
	}
}

func TestTwitterMaxPostsEnvOverride(t *testing.T) {
	t.Setenv("TWITTER_MAX_POSTS", "7")
	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Twitter.MaxPosts != 7 {
		t.Errorf("Twitter.MaxPosts = %d, want 7", cfg.Twitter.MaxPosts)
	}

	t.Setenv("TWITTER_MAX_POSTS", "not-a-number")
	cfg, err = Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Twitter.MaxPosts != defaultTwitterMaxPosts {
		t.Errorf("invalid env value should be ignored, got %d", cfg.Twitter.MaxPosts)
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

func TestWebEnvDisable(t *testing.T) {
	content := `
web:
  enabled: true
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DNS_ROOT_DIFF_WEB_ENABLED", "false")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Web.Enabled {
		t.Error("Web.Enabled = true, want false from env override")
	}
}

func TestWebEnvInvalidValueIgnored(t *testing.T) {
	t.Setenv("DNS_ROOT_DIFF_WEB_ENABLED", "yes")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Web.Enabled {
		t.Error("Web.Enabled = true, want false (invalid env value ignored)")
	}
}

func TestBlueskyConfigDefaults(t *testing.T) {
	cfg := Default()
	if cfg.Bluesky.Enabled {
		t.Error("Bluesky.Enabled = true, want false by default")
	}
	if cfg.Bluesky.APIURL != "https://bsky.social/xrpc" {
		t.Errorf("Bluesky.APIURL = %q, want https://bsky.social/xrpc", cfg.Bluesky.APIURL)
	}
	if cfg.Bluesky.MaxPostChars != 300 {
		t.Errorf("Bluesky.MaxPostChars = %d, want 300", cfg.Bluesky.MaxPostChars)
	}
}

func TestBlueskyEnvOverride(t *testing.T) {
	t.Setenv("BLUESKY_HANDLE", "user.bsky.social")
	t.Setenv("BLUESKY_APP_PASSWORD", "app-pass")
	t.Setenv("BLUESKY_API_URL", "https://custom.bsky.social/xrpc")
	t.Setenv("BLUESKY_ENABLED", "true")
	t.Setenv("BLUESKY_MAX_POST_CHARS", "500")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Bluesky.Enabled {
		t.Error("Bluesky.Enabled = false, want true from env")
	}
	if cfg.Bluesky.Handle != "user.bsky.social" {
		t.Errorf("Bluesky.Handle = %q, want user.bsky.social", cfg.Bluesky.Handle)
	}
	if cfg.Bluesky.AppPassword != "app-pass" {
		t.Errorf("Bluesky.AppPassword = %q, want app-pass", cfg.Bluesky.AppPassword)
	}
	if cfg.Bluesky.APIURL != "https://custom.bsky.social/xrpc" {
		t.Errorf("Bluesky.APIURL = %q, want custom URL", cfg.Bluesky.APIURL)
	}
	if cfg.Bluesky.MaxPostChars != 500 {
		t.Errorf("Bluesky.MaxPostChars = %d, want 500", cfg.Bluesky.MaxPostChars)
	}
}

func TestBlueskyAutoEnable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
bluesky:
  handle: "dnsrootzonediff.bsky.social"
  app_password: "secret"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !cfg.Bluesky.Enabled {
		t.Error("Bluesky.Enabled = false, want true when handle+password set")
	}
}

func TestDefaultAnchorURL(t *testing.T) {
	cfg := Default()
	if cfg.AnchorURL != "https://data.iana.org/root-anchors/root-anchors.xml" {
		t.Errorf("AnchorURL = %q", cfg.AnchorURL)
	}
}

func TestLoadAnchorURLEnv(t *testing.T) {
	t.Setenv("DNS_ROOT_DIFF_ANCHOR_URL", "http://127.0.0.1:9999/anchors.xml")
	cfg, err := Load("")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AnchorURL != "http://127.0.0.1:9999/anchors.xml" {
		t.Errorf("AnchorURL = %q", cfg.AnchorURL)
	}
}

func TestLoadAnchorURLFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
anchor_url: "https://example.com/root-anchors.xml"
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.AnchorURL != "https://example.com/root-anchors.xml" {
		t.Errorf("AnchorURL = %q", cfg.AnchorURL)
	}
}

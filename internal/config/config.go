package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config はアプリケーション全体の設定。
type Config struct {
	ZoneURL       string        `yaml:"zone_url"`
	FetchInterval time.Duration `yaml:"fetch_interval"`
	DataDir       string        `yaml:"data_dir"`
	Slack         SlackConfig   `yaml:"slack"`
	Twitter       TwitterConfig `yaml:"twitter"`
}

type SlackConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

type TwitterConfig struct {
	Enabled      bool   `yaml:"enabled"`
	APIKey       string `yaml:"api_key"`
	APISecret    string `yaml:"api_secret"`
	AccessToken  string `yaml:"access_token"`
	AccessSecret string `yaml:"access_secret"`
}

// Default はデフォルト設定を返す。
func Default() Config {
	return Config{
		ZoneURL:       "https://www.internic.net/domain/root.zone",
		FetchInterval: 6 * time.Hour,
		DataDir:       "./data",
	}
}

// Load は YAML ファイルから設定を読み込み、環境変数でオーバーライドする。
// path が空の場合はデフォルト設定を返す。
func Load(path string) (Config, error) {
	cfg := Default()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config file: %w", err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse config file: %w", err)
		}
	}

	applyEnv(&cfg)
	return cfg, nil
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("DNS_ROOT_DIFF_ZONE_URL"); v != "" {
		cfg.ZoneURL = v
	}
	if v := os.Getenv("DNS_ROOT_DIFF_INTERVAL"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.FetchInterval = d
		}
	}
	if v := os.Getenv("DNS_ROOT_DIFF_DATA_DIR"); v != "" {
		cfg.DataDir = v
	}
	if v := os.Getenv("SLACK_WEBHOOK_URL"); v != "" {
		cfg.Slack.WebhookURL = v
		cfg.Slack.Enabled = true
	}
	if v := os.Getenv("TWITTER_API_KEY"); v != "" {
		cfg.Twitter.APIKey = v
	}
	if v := os.Getenv("TWITTER_API_SECRET"); v != "" {
		cfg.Twitter.APISecret = v
	}
	if v := os.Getenv("TWITTER_ACCESS_TOKEN"); v != "" {
		cfg.Twitter.AccessToken = v
	}
	if v := os.Getenv("TWITTER_ACCESS_SECRET"); v != "" {
		cfg.Twitter.AccessSecret = v
	}
	if cfg.Twitter.APIKey != "" && cfg.Twitter.AccessToken != "" {
		cfg.Twitter.Enabled = true
	}
}

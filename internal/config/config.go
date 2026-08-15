package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// defaultTwitterMaxPosts は1回の検知で連投する最大ツイート数のデフォルト。
const defaultTwitterMaxPosts = 4

// defaultBlueskyMaxPostChars は BlueSky 投稿の最大文字数デフォルト。
const defaultBlueskyMaxPostChars = 300

// Config はアプリケーション全体の設定。
type Config struct {
	ZoneURL       string        `yaml:"zone_url"`
	AnchorURL     string        `yaml:"anchor_url"`
	FetchInterval time.Duration `yaml:"fetch_interval"`
	DataDir       string        `yaml:"data_dir"`
	Slack         SlackConfig   `yaml:"slack"`
	Twitter       TwitterConfig `yaml:"twitter"`
	Bluesky       BlueskyConfig `yaml:"bluesky"`
	Web           WebConfig     `yaml:"web"`
}

// BlueskyConfig は BlueSky 通知の設定。
type BlueskyConfig struct {
	Enabled      bool   `yaml:"enabled"`
	Handle       string `yaml:"handle"`
	AppPassword  string `yaml:"app_password"`
	APIURL       string `yaml:"api_url"`
	MaxPostChars int    `yaml:"max_post_chars"`
}

// WebConfig は diff 閲覧用 Web サーバーの設定。
type WebConfig struct {
	Enabled bool   `yaml:"enabled"`
	Listen  string `yaml:"listen"`
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
	// OAuth 2.0 User Access Token (Bearer). Set when using OAuth 2.0 instead of OAuth 1.0a.
	OAuth2AccessToken  string `yaml:"oauth2_access_token"`
	OAuth2RefreshToken string `yaml:"oauth2_refresh_token"`
	OAuth2ClientID     string `yaml:"oauth2_client_id"`
	OAuth2ClientSecret string `yaml:"oauth2_client_secret"`
	// MaxPosts は1回の検知で連投する最大ツイート数。
	MaxPosts int `yaml:"max_posts"`
}

// Default はデフォルト設定を返す。
func Default() Config {
	return Config{
		ZoneURL:       "https://www.internic.net/domain/root.zone",
		AnchorURL:     "https://data.iana.org/root-anchors/root-anchors.xml",
		FetchInterval: 2 * time.Hour,
		DataDir:       "./data",
		Twitter: TwitterConfig{
			MaxPosts: defaultTwitterMaxPosts,
		},
		Bluesky: BlueskyConfig{
			APIURL:       "https://bsky.social/xrpc",
			MaxPostChars: defaultBlueskyMaxPostChars,
		},
		Web: WebConfig{
			Enabled: false,
			Listen:  "127.0.0.1:8080",
		},
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
	// web セクションで listen が省略された場合はデフォルトに補正する。
	if cfg.Web.Listen == "" {
		cfg.Web.Listen = Default().Web.Listen
	}
	// max_posts の省略・不正値はデフォルトに補正する。
	if cfg.Twitter.MaxPosts <= 0 {
		cfg.Twitter.MaxPosts = defaultTwitterMaxPosts
	}
	// fetch_interval の 0・負値は runLoop の time.NewTicker が panic するため
	// デフォルトに補正する (MaxPosts と同じパターン)。
	if cfg.FetchInterval <= 0 {
		cfg.FetchInterval = Default().FetchInterval
	}
	return cfg, nil
}

// SaveOAuth2Tokens は config ファイル上の OAuth2 トークンを更新して書き戻す。
// path が空の場合は何もしない。
//
// ファイル全体を再シリアライズせず yaml.Node 上で対象キーのみ書き換える。
// これによりコメントや書式、未定義キーが保持され、また未指定キーにデフォルト値が
// 追記される (例: anchor_url が省略された構成で root anchors 監視が有効化される)
// 事故を防ぐ。
func SaveOAuth2Tokens(path, accessToken, refreshToken string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config file: %w", err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse config file: %w", err)
	}
	if len(root.Content) == 0 {
		return fmt.Errorf("config file is empty")
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return fmt.Errorf("config file is not a mapping")
	}
	twitter := childMapping(top, "twitter")
	setString(twitter, "oauth2_access_token", accessToken)
	if refreshToken != "" {
		setString(twitter, "oauth2_refresh_token", refreshToken)
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, out, 0600); err != nil {
		return fmt.Errorf("write config file: %w", err)
	}
	return nil
}

// childMapping は mapping ノード内の name キーの値ノード (mapping) を返す。
// キーが無い場合は末尾に追加して新規作成する。既存の値が mapping でない場合は
// mapping ノードで置き換える。
func childMapping(mapping *yaml.Node, name string) *yaml.Node {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			val := mapping.Content[i+1]
			if val.Kind == yaml.MappingNode {
				return val
			}
			val.Kind = yaml.MappingNode
			val.Tag = "!!map"
			val.Value = ""
			val.Content = nil
			return val
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
		&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
	)
	return mapping.Content[len(mapping.Content)-1]
}

// setString は mapping ノード内の name キーの値を文字列スカラーで上書きする。
// キーが無い場合は末尾に追加する。
func setString(mapping *yaml.Node, name, value string) {
	scalar := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == name {
			mapping.Content[i+1] = scalar
			return
		}
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: name},
		scalar,
	)
}

func applyEnv(cfg *Config) {
	if v := os.Getenv("DNS_ROOT_DIFF_ZONE_URL"); v != "" {
		cfg.ZoneURL = v
	}
	if v := os.Getenv("DNS_ROOT_DIFF_ANCHOR_URL"); v != "" {
		cfg.AnchorURL = v
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
	if v := os.Getenv("TWITTER_OAUTH2_ACCESS_TOKEN"); v != "" {
		cfg.Twitter.OAuth2AccessToken = v
	}
	if v := os.Getenv("TWITTER_OAUTH2_REFRESH_TOKEN"); v != "" {
		cfg.Twitter.OAuth2RefreshToken = v
	}
	if v := os.Getenv("TWITTER_OAUTH2_CLIENT_ID"); v != "" {
		cfg.Twitter.OAuth2ClientID = v
	}
	if v := os.Getenv("TWITTER_OAUTH2_CLIENT_SECRET"); v != "" {
		cfg.Twitter.OAuth2ClientSecret = v
	}
	if v := os.Getenv("TWITTER_MAX_POSTS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Twitter.MaxPosts = n
		}
	}
	if (cfg.Twitter.APIKey != "" && cfg.Twitter.AccessToken != "") || cfg.Twitter.OAuth2AccessToken != "" {
		cfg.Twitter.Enabled = true
	}
	if v := os.Getenv("BLUESKY_HANDLE"); v != "" {
		cfg.Bluesky.Handle = v
	}
	if v := os.Getenv("BLUESKY_APP_PASSWORD"); v != "" {
		cfg.Bluesky.AppPassword = v
	}
	if v := os.Getenv("BLUESKY_API_URL"); v != "" {
		cfg.Bluesky.APIURL = v
	}
	if v := os.Getenv("BLUESKY_ENABLED"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Bluesky.Enabled = b
		}
	}
	if v := os.Getenv("BLUESKY_MAX_POST_CHARS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.Bluesky.MaxPostChars = n
		}
	}
	if cfg.Bluesky.Handle != "" && cfg.Bluesky.AppPassword != "" {
		cfg.Bluesky.Enabled = true
	}
	if v := os.Getenv("DNS_ROOT_DIFF_WEB_ENABLED"); v != "" {
		// 有効化・無効化の両方向に上書きできるよう真偽値として解析する。
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Web.Enabled = b
		}
	}
	if v := os.Getenv("DNS_ROOT_DIFF_WEB_LISTEN"); v != "" {
		cfg.Web.Listen = v
	}
}

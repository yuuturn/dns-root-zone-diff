package notify

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

// TwitterNotifier は X (Twitter) API v2 にツイートを投稿する。
type TwitterNotifier struct {
	apiKey       string
	apiSecret    string
	accessToken  string
	accessSecret string
	client       *http.Client
	apiURL       string // テスト用にオーバーライド可能
}

// NewTwitterNotifier は TwitterNotifier を生成する。
func NewTwitterNotifier(apiKey, apiSecret, accessToken, accessSecret string) *TwitterNotifier {
	return &TwitterNotifier{
		apiKey:       apiKey,
		apiSecret:    apiSecret,
		accessToken:  accessToken,
		accessSecret: accessSecret,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiURL: "https://api.twitter.com/2/tweets",
	}
}

func (tw *TwitterNotifier) Name() string {
	return "twitter"
}

// Notify は X に変更内容をツイートする。
func (tw *TwitterNotifier) Notify(ctx context.Context, changes []diff.Change) error {
	msg := FormatMessage(changes)
	if msg == "" {
		return nil
	}

	// 280文字制限に対応
	if len(msg) > 280 {
		msg = msg[:277] + "..."
	}

	err := tw.postTweet(ctx, msg)
	if err != nil {
		return fmt.Errorf("post tweet: %w", err)
	}
	return nil
}

func (tw *TwitterNotifier) postTweet(ctx context.Context, text string) error {
	payload := map[string]string{"text": text}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal tweet payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tw.apiURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create tweet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	authHeader := tw.oauthHeader(http.MethodPost, tw.apiURL)
	req.Header.Set("Authorization", authHeader)

	resp, err := tw.client.Do(req)
	if err != nil {
		return fmt.Errorf("send tweet: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("twitter API returned status %d", resp.StatusCode)
	}

	return nil
}

// oauthHeader は OAuth 1.0a Authorization ヘッダーを生成する。
func (tw *TwitterNotifier) oauthHeader(method, rawURL string) string {
	params := map[string]string{
		"oauth_consumer_key":     tw.apiKey,
		"oauth_nonce":            generateNonce(),
		"oauth_signature_method": "HMAC-SHA1",
		"oauth_timestamp":        fmt.Sprintf("%d", time.Now().Unix()),
		"oauth_token":            tw.accessToken,
		"oauth_version":          "1.0",
	}

	signature := tw.sign(method, rawURL, params)
	params["oauth_signature"] = signature

	var parts []string
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, k, url.QueryEscape(params[k])))
	}

	return "OAuth " + strings.Join(parts, ", ")
}

func (tw *TwitterNotifier) sign(method, rawURL string, params map[string]string) string {
	var paramParts []string
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		paramParts = append(paramParts, url.QueryEscape(k)+"="+url.QueryEscape(params[k]))
	}
	paramString := strings.Join(paramParts, "&")

	baseURL := rawURL
	if idx := strings.Index(rawURL, "?"); idx != -1 {
		baseURL = rawURL[:idx]
	}

	signingKey := url.QueryEscape(tw.apiSecret) + "&" + url.QueryEscape(tw.accessSecret)
	baseString := strings.ToUpper(method) + "&" + url.QueryEscape(baseURL) + "&" + url.QueryEscape(paramString)

	mac := hmac.New(sha1.New, []byte(signingKey))
	mac.Write([]byte(baseString))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func generateNonce() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

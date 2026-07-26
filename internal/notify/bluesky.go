package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yfujii/dns-root-diff/internal/config"
	"github.com/yfujii/dns-root-diff/internal/diff"
)

// BlueskyNotifier は BlueSky (AT Protocol) に通知を投稿する。
type BlueskyNotifier struct {
	cfg       config.BlueskyConfig
	client    *http.Client
	baseURL   string
	did       string
	accessJwt string
}

// NewBlueskyNotifier は BlueskyNotifier を生成する。
func NewBlueskyNotifier(cfg config.BlueskyConfig) *BlueskyNotifier {
	if cfg.APIURL == "" {
		cfg.APIURL = "https://bsky.social/xrpc"
	}
	if cfg.MaxPostChars <= 0 {
		cfg.MaxPostChars = 300
	}
	return &BlueskyNotifier{
		cfg: cfg,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: cfg.APIURL,
	}
}

func (n *BlueskyNotifier) Name() string {
	return "bluesky"
}

// Notify は BlueSky に diff 要約を投稿する。
// 実質的な変更がない場合は何も投稿しない。
func (n *BlueskyNotifier) Notify(ctx context.Context, changes []diff.Change) error {
	if len(changes) == 0 {
		return nil
	}

	sub := diff.Substantive(changes)
	if len(sub) == 0 {
		return nil
	}

	if err := n.ensureSession(ctx); err != nil {
		return fmt.Errorf("createSession: %w", err)
	}

	texts := FormatPosts(sub, FormatOptions{
		MaxLen:    n.cfg.MaxPostChars,
		Numbering: len(sub) > 0,
	})
	if len(texts) == 0 {
		return nil
	}

	for i, text := range texts {
		if err := n.postRecord(ctx, text); err != nil {
			return fmt.Errorf("createRecord %d/%d: %w", i+1, len(texts), err)
		}
	}

	return nil
}

func (n *BlueskyNotifier) ensureSession(ctx context.Context) error {
	if n.did != "" {
		return nil
	}
	payload := map[string]string{
		"identifier": n.cfg.Handle,
		"password":   n.cfg.AppPassword,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal session payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/com.atproto.server.createSession", strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("create session request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send session request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("createSession returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var session struct {
		DID       string `json:"did"`
		AccessJwt string `json:"accessJwt"`
	}
	if err := json.Unmarshal(respBody, &session); err != nil {
		return fmt.Errorf("decode session response: %w", err)
	}
	if session.DID == "" {
		return fmt.Errorf("createSession response missing did")
	}
	n.did = session.DID
	n.accessJwt = session.AccessJwt
	return nil
}

func (n *BlueskyNotifier) postRecord(ctx context.Context, text string) error {
	record := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	payload := map[string]interface{}{
		"repo":       n.did,
		"collection": "app.bsky.feed.post",
		"record":     record,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal createRecord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/com.atproto.repo.createRecord", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create record request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if n.accessJwt != "" {
		req.Header.Set("Authorization", "Bearer "+n.accessJwt)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("send record: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("createRecord returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return fmt.Errorf("decode createRecord response: %w", err)
	}
	if result.URI == "" {
		return fmt.Errorf("createRecord response missing uri")
	}
	return nil
}

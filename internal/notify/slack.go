package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

const (
	// slackMaxLen は Slack 1メッセージの最大文字数。X より緩いのでレコード単位の明細が収まりやすい。
	slackMaxLen = 3500
	// slackMaxParts は 1回の検知で送る最大メッセージ数。
	slackMaxParts = 3
)

// SlackNotifier は Slack Webhook に通知する。
type SlackNotifier struct {
	webhookURL string
	client     *http.Client
}

// NewSlackNotifier は SlackNotifier を生成する。
func NewSlackNotifier(webhookURL string) *SlackNotifier {
	return &SlackNotifier{
		webhookURL: webhookURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (s *SlackNotifier) Name() string {
	return "slack"
}

// Notify は Slack Webhook にメッセージを送信する。
// 実質的な変更がない (再署名のみの) 場合は何も送信しない。
func (s *SlackNotifier) Notify(ctx context.Context, changes []diff.Change) error {
	msgs := FormatPosts(changes, FormatOptions{
		MaxLen:    slackMaxLen,
		MaxParts:  slackMaxParts,
		Numbering: true,
	})
	for i, msg := range msgs {
		if err := s.post(ctx, msg); err != nil {
			return fmt.Errorf("slack message %d/%d: %w", i+1, len(msgs), err)
		}
	}
	return nil
}

func (s *SlackNotifier) post(ctx context.Context, msg string) error {
	payload := map[string]string{"text": msg}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal slack payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.webhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create slack request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("send slack notification: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}

	return nil
}

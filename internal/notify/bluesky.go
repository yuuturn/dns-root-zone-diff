package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/yfujii/dns-root-diff/internal/config"
	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/whois"
)

// BlueskyNotifier は BlueSky (AT Protocol) に通知を投稿する。
type BlueskyNotifier struct {
	cfg       config.BlueskyConfig
	client    *http.Client
	baseURL   string
	did       string
	accessJwt string
	mu        sync.Mutex
	// lastCreatedAt は投稿の createdAt を厳密に増加させるために保持する。
	lastCreatedAt time.Time
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
//
// FormatPosts には changes をそのまま渡す。SOA serial の遷移をタイトル行に
// 載せるため、再署名ノイズに混ざった SOA 変更をここで落としてはいけない
// (実質的な変更の絞り込みは FormatPosts が行う)。
func (n *BlueskyNotifier) Notify(ctx context.Context, changes []diff.Change) error {
	_, err := n.notify(ctx, FormatPosts(changes, FormatOptions{
		MaxLen:    n.cfg.MaxPostChars,
		Numbering: true,
	}))
	return err
}

// NotifyAnchors は root anchors の変更を BlueSky に投稿する。
// 実質的な変更がない場合は何も投稿しない。
func (n *BlueskyNotifier) NotifyAnchors(ctx context.Context, changes []diff.Change) error {
	_, err := n.notify(ctx, FormatPosts(changes, anchorFormatOptions(FormatOptions{
		MaxLen:    n.cfg.MaxPostChars,
		Numbering: true,
	})))
	return err
}

// NotifyTLDs は TLD リストの変更を BlueSky に投稿する。
// 最後の投稿に、追加・削除された TLD の WHOIS 情報をリプライとして連投する。
func (n *BlueskyNotifier) NotifyTLDs(ctx context.Context, changes []diff.Change, infos map[string]whois.Info) error {
	last, err := n.notify(ctx, FormatPosts(changes, tldFormatOptions(FormatOptions{
		MaxLen:    n.cfg.MaxPostChars,
		Numbering: true,
	})))
	if err != nil {
		return err
	}
	if last.URI == "" || last.CID == "" {
		return nil
	}

	for _, reply := range buildWhoisReplies(changes, infos) {
		text := fitRunes(reply.text, n.cfg.MaxPostChars)
		if err := n.postReply(ctx, text, last); err != nil {
			return fmt.Errorf("createRecord TLD whois reply (%s): %w", reply.tld, err)
		}
	}
	return nil
}

// postRef は BlueSky の投稿を指す strong ref (URI + CID)。
type postRef struct {
	URI string
	CID string
}

// notify は texts を順に投稿し、最後の投稿の参照を返す。
// テキストが無ければ何もしない。
func (n *BlueskyNotifier) notify(ctx context.Context, texts []string) (postRef, error) {
	if len(texts) == 0 {
		return postRef{}, nil
	}

	if err := n.ensureSession(ctx); err != nil {
		return postRef{}, fmt.Errorf("createSession: %w", err)
	}
	return n.postRecords(ctx, texts)
}

// postRecords は texts を順に投稿する。セッション失効時は refreshSession で
// 再認証してから1回だけリトライする。
func (n *BlueskyNotifier) postRecords(ctx context.Context, texts []string) (postRef, error) {
	var last postRef
	for i, text := range texts {
		ref, err := n.postRecordWithRetry(ctx, text, nil)
		if err != nil {
			return postRef{}, fmt.Errorf("createRecord %d/%d: %w", i+1, len(texts), err)
		}
		last = ref
	}
	return last, nil
}

// postRecordWithRetry は投稿し、セッション失効時は再認証して1回だけリトライする。
func (n *BlueskyNotifier) postRecordWithRetry(ctx context.Context, text string, parent *postRef) (postRef, error) {
	ref, err := n.postRecord(ctx, text, parent)
	if err != nil {
		if rerr := n.refreshSession(ctx); rerr != nil {
			return postRef{}, fmt.Errorf("refresh session: %w", rerr)
		}
		ref, err = n.postRecord(ctx, text, parent)
		if err != nil {
			return postRef{}, err
		}
	}
	return ref, nil
}

// postReply は parent へのリプライを投稿する。
func (n *BlueskyNotifier) postReply(ctx context.Context, text string, parent postRef) error {
	_, err := n.postRecordWithRetry(ctx, text, &parent)
	return err
}

func (n *BlueskyNotifier) refreshSession(ctx context.Context) error {
	n.mu.Lock()
	n.did = ""
	n.accessJwt = ""
	n.mu.Unlock()
	if err := n.ensureSession(ctx); err != nil {
		return err
	}
	return nil
}

func (n *BlueskyNotifier) ensureSession(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.did != "" && n.accessJwt != "" {
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

func (n *BlueskyNotifier) postRecord(ctx context.Context, text string, parent *postRef) (postRef, error) {
	// createdAt はミリ秒精度で厳密に増加する値にする。
	// BlueSky は同じ createdAt の投稿を重複として落とすことがあるため、
	// 短い間隔で連続投稿しても重複しないよう、前回より必ず後の時刻を使う。
	// 生の time.Now() はナノ秒精度のため、ミリ秒に Truncate してから比較し、
	// 同値なら +1ms 進める。
	n.mu.Lock()
	now := time.Now().UTC().Truncate(time.Millisecond)
	last := n.lastCreatedAt.Truncate(time.Millisecond)
	if !now.After(last) {
		now = last.Add(time.Millisecond)
	}
	n.lastCreatedAt = now
	n.mu.Unlock()
	createdAt := now.Format("2006-01-02T15:04:05.000Z")

	record := map[string]interface{}{
		"$type":     "app.bsky.feed.post",
		"text":      text,
		"createdAt": createdAt,
	}
	if parent != nil {
		// 通知本体はスレッドではないため、root と parent は同じ投稿を指す。
		ref := map[string]string{"uri": parent.URI, "cid": parent.CID}
		record["reply"] = map[string]interface{}{
			"root":   ref,
			"parent": ref,
		}
	}

	payload := map[string]interface{}{
		"repo":       n.did,
		"collection": "app.bsky.feed.post",
		"record":     record,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return postRef{}, fmt.Errorf("marshal createRecord payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.baseURL+"/com.atproto.repo.createRecord", bytes.NewReader(body))
	if err != nil {
		return postRef{}, fmt.Errorf("create record request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if n.accessJwt != "" {
		req.Header.Set("Authorization", "Bearer "+n.accessJwt)
	}

	resp, err := n.client.Do(req)
	if err != nil {
		return postRef{}, fmt.Errorf("send record: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	// AT Protocol の com.atproto.repo.createRecord は成功時に 200 OK を返す
	// (Twitter の 201 Created とは異なる)。両方を成功とみなす。
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return postRef{}, fmt.Errorf("createRecord returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return postRef{}, fmt.Errorf("decode createRecord response: %w", err)
	}
	if result.URI == "" {
		return postRef{}, fmt.Errorf("createRecord response missing uri")
	}
	return postRef{URI: result.URI, CID: result.CID}, nil
}

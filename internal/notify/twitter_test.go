package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

// manyDelegationChanges は 1ツイートに収まらない量の実質的な変更を作る。
func manyDelegationChanges(n int) []diff.Change {
	changes := make([]diff.Change, 0, n)
	for i := 0; i < n; i++ {
		changes = append(changes, diff.Change{
			Kind:     diff.ChangeAdded,
			Name:     fmt.Sprintf("verylongdomainname%02d.", i),
			Type:     "NS",
			NewRData: "ns1.verylongdomainname.example.com.",
		})
	}
	return changes
}

func TestTwitterNotifySuccess(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)

		auth := r.Header.Get("Authorization")
		if auth == "" {
			t.Error("missing Authorization header")
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"123"}}`))
	}))
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}

	err := n.Notify(context.Background(), changes)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if received["text"] == "" {
		t.Error("Twitter received empty text")
	}
}

func TestTwitterNotifyNoChanges(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL

	err := n.Notify(context.Background(), nil)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if called {
		t.Error("Notify() should not call API when no changes")
	}
}

func TestTwitterNotifyHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}

	err := n.Notify(context.Background(), changes)
	if err == nil {
		t.Fatal("Notify() expected error for 403")
	}
}

func TestTwitterName(t *testing.T) {
	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	if n.Name() != "twitter" {
		t.Errorf("Name() = %q, want twitter", n.Name())
	}
}

// tweetCollector は投稿されたツイート本文を順に記録するテストサーバー。
type tweetCollector struct {
	*httptest.Server
	mu     sync.Mutex
	texts  []string
	posted chan struct{} // 1件記録するごとに通知 (バッファ付き)
}

func newTweetCollector(t *testing.T) *tweetCollector {
	t.Helper()
	c := &tweetCollector{posted: make(chan struct{}, 64)}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("unmarshal tweet payload: %v", err)
		}
		c.mu.Lock()
		c.texts = append(c.texts, payload["text"])
		c.mu.Unlock()
		select {
		case c.posted <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"123"}}`))
	}))
	return c
}

func (c *tweetCollector) Texts() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.texts...)
}

func TestTwitterSplitsLongMessageIntoNumberedPosts(t *testing.T) {
	srv := newTweetCollector(t)
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL
	n.postDelay = 0

	changes := manyDelegationChanges(50)
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	texts := srv.Texts()
	if len(texts) < 2 {
		t.Fatalf("got %d tweets, want >= 2", len(texts))
	}
	if len(texts) > defaultMaxPosts {
		t.Fatalf("got %d tweets, want <= %d", len(texts), defaultMaxPosts)
	}
	for i, text := range texts {
		if got := weightedLen(text); got > 280 {
			t.Errorf("tweet %d weighted length = %d, want <= 280:\n%s", i+1, got, text)
		}
		if want := fmt.Sprintf("(%d/%d)", i+1, len(texts)); !strings.Contains(text, want) {
			t.Errorf("tweet %d missing %q:\n%s", i+1, want, text)
		}
	}
}

func TestTwitterCountsMultibyteAsWeighted(t *testing.T) {
	srv := newTweetCollector(t)
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL
	n.postDelay = 0

	// 全角の RDATA。rune 数で数えると X の重み付き上限を超える。
	var changes []diff.Change
	for i := 0; i < 10; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("xn--example%02d.", i), Type: "TXT",
			NewRData: strings.Repeat("あ", 30),
		})
	}
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	texts := srv.Texts()
	if len(texts) == 0 {
		t.Fatal("no tweets posted")
	}
	for i, text := range texts {
		if got := weightedLen(text); got > 280 {
			t.Errorf("tweet %d weighted length = %d, want <= 280:\n%s", i+1, got, text)
		}
		if !utf8.ValidString(text) {
			t.Errorf("tweet %d is not valid UTF-8", i+1)
		}
	}
}

func TestTwitterSkipsResigningOnlyChanges(t *testing.T) {
	srv := newTweetCollector(t)
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL
	n.postDelay = 0

	// RRSIG の入れ替えと SOA serial の更新だけの回は投稿しない。
	changes := []diff.Change{
		{Kind: diff.ChangeRemoved, Name: "example.", Type: "RRSIG", OldRData: "DS 8 1 86400 ... old"},
		{Kind: diff.ChangeAdded, Name: "example.", Type: "RRSIG", NewRData: "DS 8 1 86400 ... new"},
		{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldRData: "a. b. 1 1 1 1 1", NewRData: "a. b. 2 1 1 1 1"},
	}
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if texts := srv.Texts(); len(texts) != 0 {
		t.Errorf("posted %d tweets for re-signing only changes: %v", len(texts), texts)
	}
}

func TestTwitterRespectsMaxPosts(t *testing.T) {
	srv := newTweetCollector(t)
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL
	n.postDelay = 0
	n.SetMaxPosts(2)

	var changes []diff.Change
	for i := 0; i < 300; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("tld%03d.", i), Type: "NS", NewRData: "ns1.example.net.",
		})
	}
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	texts := srv.Texts()
	if len(texts) != 2 {
		t.Fatalf("got %d tweets, want 2", len(texts))
	}
	if !strings.Contains(texts[1], "more changes") {
		t.Errorf("last tweet should report the dropped count:\n%s", texts[1])
	}
}

func TestTwitterSetMaxPostsIgnoresNonPositive(t *testing.T) {
	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.SetMaxPosts(0)
	if n.maxPosts != defaultMaxPosts {
		t.Errorf("maxPosts = %d, want %d", n.maxPosts, defaultMaxPosts)
	}
	n.SetMaxPosts(-1)
	if n.maxPosts != defaultMaxPosts {
		t.Errorf("maxPosts = %d, want %d", n.maxPosts, defaultMaxPosts)
	}
}

func TestTwitterStopsAfterPostFailure(t *testing.T) {
	var mu sync.Mutex
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n == 1 {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"123"}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL
	n.postDelay = 0

	err := n.Notify(context.Background(), manyDelegationChanges(50))
	if err == nil {
		t.Fatal("Notify() expected error when a tweet fails")
	}
	if !strings.Contains(err.Error(), "post tweet 2/") {
		t.Errorf("error should name the failing tweet, got: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 2 {
		t.Errorf("API called %d times, want 2 (stop after the failure)", calls)
	}
}

func TestTwitterPostDelayCancelsWithContext(t *testing.T) {
	srv := newTweetCollector(t)
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL
	n.postDelay = time.Hour // 2本目の待機で必ずブロックする

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- n.Notify(ctx, manyDelegationChanges(50)) }()

	// 1本目の投稿が終わるのを待ってからキャンセルする。
	select {
	case <-srv.posted:
	case <-time.After(5 * time.Second):
		t.Fatal("first tweet was not posted")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Notify() error = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Notify() did not return after cancel")
	}
}

func TestTwitterOAuth2BearerNotifySuccess(t *testing.T) {
	var authHeader string
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"456"}}`))
	}))
	defer srv.Close()

	n := NewTwitterOAuth2Notifier("oauth2-access-token", "", "", "", nil)
	n.apiURL = srv.URL

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if authHeader != "Bearer oauth2-access-token" {
		t.Errorf("Authorization = %q, want Bearer oauth2-access-token", authHeader)
	}
	if received["text"] == "" {
		t.Error("Twitter received empty text")
	}
}

func TestTwitterOAuth2Name(t *testing.T) {
	n := NewTwitterOAuth2Notifier("token", "", "", "", nil)
	if n.Name() != "twitter" {
		t.Errorf("Name() = %q, want twitter", n.Name())
	}
}

func TestTwitterOAuth2RefreshesOnUnauthorized(t *testing.T) {
	var tweetAuths []string
	var refreshCount int
	var savedAccess, savedRefresh string

	mux := http.NewServeMux()
	mux.HandleFunc("/2/tweets", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		tweetAuths = append(tweetAuths, auth)
		if auth == "Bearer expired-token" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"title":"Unauthorized"}`))
			return
		}
		if auth == "Bearer new-access-token" {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{"id":"789"}}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	})
	mux.HandleFunc("/2/oauth2/token", func(w http.ResponseWriter, r *http.Request) {
		refreshCount++
		if err := r.ParseForm(); err != nil {
			t.Errorf("ParseForm: %v", err)
		}
		if r.Form.Get("grant_type") != "refresh_token" {
			t.Errorf("grant_type = %q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("refresh_token") != "old-refresh" {
			t.Errorf("refresh_token = %q", r.Form.Get("refresh_token"))
		}
		user, pass, ok := r.BasicAuth()
		if !ok || user != "client-id" || pass != "client-secret" {
			t.Errorf("basic auth = %q/%q ok=%v", user, pass, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"new-access-token","refresh_token":"new-refresh","token_type":"bearer","expires_in":7200}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	n := NewTwitterOAuth2Notifier("expired-token", "old-refresh", "client-id", "client-secret", func(access, refresh string) error {
		savedAccess, savedRefresh = access, refresh
		return nil
	})
	n.apiURL = srv.URL + "/2/tweets"
	n.tokenURL = srv.URL + "/2/oauth2/token"

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if refreshCount != 1 {
		t.Errorf("refreshCount = %d, want 1", refreshCount)
	}
	if len(tweetAuths) != 2 {
		t.Fatalf("tweet attempts = %d, want 2: %v", len(tweetAuths), tweetAuths)
	}
	if tweetAuths[0] != "Bearer expired-token" || tweetAuths[1] != "Bearer new-access-token" {
		t.Errorf("tweetAuths = %v", tweetAuths)
	}
	if savedAccess != "new-access-token" || savedRefresh != "new-refresh" {
		t.Errorf("saved tokens = %q / %q", savedAccess, savedRefresh)
	}
}

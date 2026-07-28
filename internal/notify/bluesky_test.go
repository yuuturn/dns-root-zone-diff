package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/yfujii/dns-root-diff/internal/config"
	"github.com/yfujii/dns-root-diff/internal/diff"
)

func TestBlueskyNotifySuccess(t *testing.T) {
	var authHeader string
	var createRecordReq *http.Request
	var createRecordBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"did":"did:plc:123","accessJwt":"jwt","refreshJwt":"rjwt","handle":"user.bsky.social"}`))
			return
		}

		if r.URL.Path == "/xrpc/com.atproto.repo.createRecord" {
			authHeader = r.Header.Get("Authorization")
			createRecordReq = r
			createRecordBody = body
			// AT Protocol の createRecord は成功時に 200 OK を返す。
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"uri":"at://did:plc:123/app.bsky.feed.post/abc","cid":"bafy"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	n := NewBlueskyNotifier(config.BlueskyConfig{
		Handle:       "user.bsky.social",
		AppPassword:  "app-pass",
		APIURL:       srv.URL + "/xrpc",
		MaxPostChars: 300,
	})

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}

	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if createRecordReq == nil {
		t.Fatal("createRecord was not called")
	}
	if authHeader != "Bearer jwt" {
		t.Errorf("Authorization = %q, want Bearer jwt", authHeader)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(createRecordBody, &payload); err != nil {
		t.Fatalf("decode createRecord payload: %v", err)
	}
	if payload["repo"] != "did:plc:123" {
		t.Errorf("repo = %v, want did:plc:123", payload["repo"])
	}
	if payload["collection"] != "app.bsky.feed.post" {
		t.Errorf("collection = %v, want app.bsky.feed.post", payload["collection"])
	}
	record, ok := payload["record"].(map[string]interface{})
	if !ok {
		t.Fatalf("record type = %T, want object", payload["record"])
	}
	if record["$type"] != "app.bsky.feed.post" {
		t.Errorf("record.$type = %v, want app.bsky.feed.post", record["$type"])
	}
	text, ok := record["text"].(string)
	if !ok || text == "" {
		t.Errorf("record.text = %q, want non-empty", text)
	}
	if _, ok := record["createdAt"].(string); !ok {
		t.Error("record.createdAt should be a string")
	}
}

// TestBlueskyPostsAllPartsOnStatusOK は AT Protocol の createRecord が 200 OK を
// 返す実運用の挙動を再現する。過去のバグでは 200 をエラーとみなし、1 パーツ目で
// ループを抜けていた。ここでは複数パーツがすべて投稿されることを検証する。
func TestBlueskyPostsAllPartsOnStatusOK(t *testing.T) {
	var createRecordCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"did":"did:plc:123","accessJwt":"jwt","refreshJwt":"rjwt","handle":"user.bsky.social"}`))
			return
		}
		if r.URL.Path == "/xrpc/com.atproto.repo.createRecord" {
			createRecordCalls++
			// 実運用と同じく 200 OK で応答。
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"uri":"at://did:plc:123/app.bsky.feed.post/abc","cid":"bafy"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	n := NewBlueskyNotifier(config.BlueskyConfig{
		Handle:       "user.bsky.social",
		AppPassword:  "app-pass",
		APIURL:       srv.URL + "/xrpc",
		MaxPostChars: 60, // 小さくして複数パーツに分割させる
	})

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "a.", Type: "NS", NewRData: "ns1.a."},
		{Kind: diff.ChangeAdded, Name: "b.", Type: "NS", NewRData: "ns1.b."},
		{Kind: diff.ChangeAdded, Name: "c.", Type: "NS", NewRData: "ns1.c."},
	}

	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	// 200 を成功とみなすため、複数パーツがすべて投稿されるはず。
	// 過去のバグ (200 をエラー扱い) では 1 パーツ目でループを抜けて 1 回しか
	// 投稿されなかった。ここでは複数回投稿されることを検証する。
	if createRecordCalls < 2 {
		t.Fatalf("createRecord calls = %d, want >= 2 (all parts posted on 200 OK)", createRecordCalls)
	}
}

func TestBlueskySkipsResigningOnlyChanges(t *testing.T) {
	postCalls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		postCalls++
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"uri":"at://did:plc:123/app.bsky.feed.post/abc","cid":"bafy"}`))
	}))
	defer srv.Close()

	n := NewBlueskyNotifier(config.BlueskyConfig{
		Handle:       "user.bsky.social",
		AppPassword:  "app-pass",
		APIURL:       srv.URL + "/xrpc",
		MaxPostChars: 300,
	})
	n.client = &http.Client{Timeout: 5 * time.Second}

	changes := []diff.Change{
		{Kind: diff.ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400, OldRData: "DS 8 1 86400 20260806050000 20260724040000 57780 . AAAA"},
		{Kind: diff.ChangeAdded, Name: "example.", Type: "RRSIG", NewTTL: 86400, NewRData: "DS 8 1 86400 20260806170000 20260724160000 57780 . BBBB"},
		{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldRData: "a. b. 1 1 1 1 1", NewRData: "a. b. 2 1 1 1 1"},
	}

	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if postCalls != 0 {
		t.Errorf("createRecord calls = %d, want 0", postCalls)
	}
}

func TestBlueskyNotifyHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"did":"did:plc:123","accessJwt":"jwt","refreshJwt":"rjwt","handle":"user.bsky.social"}`))
			return
		}

		if r.URL.Path == "/xrpc/com.atproto.repo.createRecord" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"boom"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	n := NewBlueskyNotifier(config.BlueskyConfig{
		Handle:       "user.bsky.social",
		AppPassword:  "app-pass",
		APIURL:       srv.URL + "/xrpc",
		MaxPostChars: 300,
	})
	n.client = &http.Client{Timeout: 5 * time.Second}

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}

	err := n.Notify(context.Background(), changes)
	if err == nil {
		t.Fatal("Notify() expected error for 500")
	}
	if !strings.Contains(err.Error(), "createRecord") {
		t.Errorf("Notify() error = %v, want createRecord failure", err)
	}
}

func TestBlueskyName(t *testing.T) {
	n := NewBlueskyNotifier(config.BlueskyConfig{})
	if n.Name() != "bluesky" {
		t.Errorf("Name() = %q, want bluesky", n.Name())
	}
}

func TestBlueskyDefaults(t *testing.T) {
	n := NewBlueskyNotifier(config.BlueskyConfig{})
	if n == nil {
		t.Fatal("NewBlueskyNotifier() returned nil")
	}
}

func TestBlueskyRefreshesSessionOnCreateRecordUnauthorized(t *testing.T) {
	var sessionCalls int
	var createRecordCalls int
	var authHeaders []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)

		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			sessionCalls++
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"did":"did:plc:123","accessJwt":"jwt-` + strconv.Itoa(sessionCalls) + `","refreshJwt":"rjwt","handle":"user.bsky.social"}`))
			return
		}

		if r.URL.Path == "/xrpc/com.atproto.repo.createRecord" {
			createRecordCalls++
			authHeaders = append(authHeaders, r.Header.Get("Authorization"))
			if createRecordCalls == 1 {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"Unauthorized"}`))
				return
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"uri":"at://did:plc:123/app.bsky.feed.post/` + strconv.Itoa(createRecordCalls) + `","cid":"bafy"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	n := NewBlueskyNotifier(config.BlueskyConfig{
		Handle:       "user.bsky.social",
		AppPassword:  "app-pass",
		APIURL:       srv.URL + "/xrpc",
		MaxPostChars: 300,
	})
	n.client = &http.Client{Timeout: 5 * time.Second}

	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}

	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if sessionCalls != 2 {
		t.Fatalf("createSession calls = %d, want 2", sessionCalls)
	}
	if createRecordCalls != 2 {
		t.Fatalf("createRecord calls = %d, want 2", createRecordCalls)
	}
	if len(authHeaders) != 2 {
		t.Fatalf("auth headers = %v, want 2 entries", authHeaders)
	}
	if authHeaders[0] != "Bearer jwt-1" {
		t.Errorf("first auth header = %q, want Bearer jwt-1", authHeaders[0])
	}
	if authHeaders[1] != "Bearer jwt-2" {
		t.Errorf("second auth header = %q, want Bearer jwt-2", authHeaders[1])
	}
}

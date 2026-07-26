package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yfujii/dns-root-diff/internal/config"
	"github.com/yfujii/dns-root-diff/internal/diff"
)

func TestBlueskyNotifySuccess(t *testing.T) {
	var postCalls int
	var createRecordPayload map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		if r.URL.Path == "/xrpc/com.atproto.server.createSession" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"did":"did:plc:123","accessJwt":"jwt","refreshJwt":"rjwt","handle":"user.bsky.social"}`))
			return
		}

		if r.URL.Path == "/xrpc/com.atproto.repo.createRecord" {
			postCalls++
			_ = json.Unmarshal(body, &createRecordPayload)
			w.WriteHeader(http.StatusCreated)
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
	if postCalls != 1 {
		t.Fatalf("createRecord calls = %d, want 1", postCalls)
	}
	if createRecordPayload == nil || createRecordPayload["repo"] != "did:plc:123" {
		t.Errorf("repo = %q, want did:plc:123", createRecordPayload)
	}
	recordJSON := createRecordPayload["record"]
	var record map[string]interface{}
	if err := json.Unmarshal([]byte(recordJSON), &record); err != nil {
		t.Fatalf("decode record json: %v", err)
	}
	text, ok := record["text"].(string)
	if !ok || text == "" {
		t.Errorf("record.text = %q, want non-empty", text)
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

		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
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

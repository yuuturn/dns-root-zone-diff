package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

func TestSlackNotifySuccess(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}

	err := n.Notify(context.Background(), changes)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if received["text"] == "" {
		t.Error("Slack received empty text")
	}
	for _, want := range []string{postTitle, "[delegation]", "+ test. NS ns1.test."} {
		if !strings.Contains(received["text"], want) {
			t.Errorf("missing %q in:\n%s", want, received["text"])
		}
	}
}

func TestSlackSkipsResigningOnlyChanges(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	changes := []diff.Change{
		{Kind: diff.ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400,
			OldRData: "DS 8 1 86400 20260806050000 20260724040000 57780 . AAAA"},
		{Kind: diff.ChangeAdded, Name: "example.", Type: "RRSIG", NewTTL: 86400,
			NewRData: "DS 8 1 86400 20260806170000 20260724160000 57780 . BBBB"},
		{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldRData: "a. b. 1 1 1 1 1", NewRData: "a. b. 2 1 1 1 1"},
	}
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if called {
		t.Error("Notify() should not call webhook for re-signing only changes")
	}
}

func TestSlackSendsMultipleMessagesWhenSplit(t *testing.T) {
	var mu sync.Mutex
	var texts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		texts = append(texts, payload["text"])
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// slackMaxLen を超える量の実質的な変更。
	var changes []diff.Change
	for i := 0; i < 400; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("tld%03d.", i), Type: "NS",
			NewRData: "ns1.dns.registry.example.net.",
		})
	}

	n := NewSlackNotifier(srv.URL)
	if err := n.Notify(context.Background(), changes); err != nil {
		t.Fatalf("Notify() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(texts) < 2 {
		t.Fatalf("got %d messages, want >= 2", len(texts))
	}
	if len(texts) > slackMaxParts {
		t.Fatalf("got %d messages, want <= %d", len(texts), slackMaxParts)
	}
	for i, text := range texts {
		if got := utf8.RuneCountInString(text); got > slackMaxLen {
			t.Errorf("message %d is %d runes, want <= %d", i+1, got, slackMaxLen)
		}
		if want := fmt.Sprintf("(%d/%d)", i+1, len(texts)); !strings.Contains(text, want) {
			t.Errorf("message %d missing %q", i+1, want)
		}
	}
}

func TestSlackNotifyNoChanges(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	err := n.Notify(context.Background(), nil)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if called {
		t.Error("Notify() should not call webhook when no changes")
	}
}

func TestSlackNotifyHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewSlackNotifier(srv.URL)
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "test.", Type: "NS", NewRData: "ns1.test."},
	}

	err := n.Notify(context.Background(), changes)
	if err == nil {
		t.Fatal("Notify() expected error for 500")
	}
}

func TestSlackName(t *testing.T) {
	n := NewSlackNotifier("https://example.com")
	if n.Name() != "slack" {
		t.Errorf("Name() = %q, want slack", n.Name())
	}
}

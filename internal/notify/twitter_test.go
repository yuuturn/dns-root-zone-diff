package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

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

func TestTwitterTruncatesLongMessage(t *testing.T) {
	var received map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"123"}}`))
	}))
	defer srv.Close()

	n := NewTwitterNotifier("key", "secret", "token", "tokensecret")
	n.apiURL = srv.URL

	// 大量の変更で280文字を超えるメッセージを生成
	var changes []diff.Change
	for i := 0; i < 50; i++ {
		changes = append(changes, diff.Change{
			Kind:     diff.ChangeAdded,
			Name:     "verylongdomainname" + string(rune('a'+i%26)) + ".",
			Type:     "NS",
			NewRData: "ns1.verylongdomainname.example.com.",
		})
	}

	err := n.Notify(context.Background(), changes)
	if err != nil {
		t.Fatalf("Notify() error = %v", err)
	}
	if len(received["text"]) > 280 {
		t.Errorf("tweet text length = %d, want <= 280", len(received["text"]))
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

	n := NewTwitterOAuth2Notifier("oauth2-access-token")
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
	n := NewTwitterOAuth2Notifier("token")
	if n.Name() != "twitter" {
		t.Errorf("Name() = %q, want twitter", n.Name())
	}
}

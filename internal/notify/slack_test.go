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

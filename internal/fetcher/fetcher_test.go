package fetcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchSuccess(t *testing.T) {
	zoneData := ".\t86400\tIN\tSOA\ta.root-servers.net. nstld.verisign-grs.com. 2026072301 1800 900 604800 86400\n"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(zoneData))
	}))
	defer srv.Close()

	f := New(srv.URL, 5*time.Second)
	data, err := f.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if string(data) != zoneData {
		t.Errorf("Fetch() = %q, want %q", string(data), zoneData)
	}
}

func TestFetchHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	f := New(srv.URL, 5*time.Second)
	_, err := f.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() expected error for 404")
	}
}

func TestFetchTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		_, _ = w.Write([]byte("data"))
	}))
	defer srv.Close()

	f := New(srv.URL, 100*time.Millisecond)
	ctx := context.Background()
	_, err := f.Fetch(ctx)
	if err == nil {
		t.Fatal("Fetch() expected timeout error")
	}
}

func TestFetchContextCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
	}))
	defer srv.Close()

	f := New(srv.URL, 10*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := f.Fetch(ctx)
	if err == nil {
		t.Fatal("Fetch() expected error on cancelled context")
	}
}

func TestFetchEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	f := New(srv.URL, 5*time.Second)
	_, err := f.Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() expected error for empty body")
	}
}

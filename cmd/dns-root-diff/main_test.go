package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/yfujii/dns-root-diff/internal/config"
	"github.com/yfujii/dns-root-diff/internal/store"
	"github.com/yfujii/dns-root-diff/internal/zone"
)

func TestRunOnceInitialRun(t *testing.T) {
	zoneData := ".\t86400\tIN\tNS\ta.root-servers.net.\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(zoneData))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := config.Config{
		ZoneURL:       srv.URL,
		DataDir:       dir,
		FetchInterval: 0,
	}

	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}

	s := store.New(dir)
	if !s.Exists() {
		t.Error("store file should exist after initial run")
	}

	data, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	records, err := zone.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Errorf("records = %d, want 1", len(records))
	}
}

func TestRunOnceDetectsChanges(t *testing.T) {
	oldZone := ".\t86400\tIN\tNS\ta.root-servers.net.\n"
	newZone := ".\t86400\tIN\tNS\ta.root-servers.net.\n" +
		"bbb.\t172800\tIN\tNS\tns1.bbb.\n"

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_, _ = w.Write([]byte(oldZone))
		} else {
			_, _ = w.Write([]byte(newZone))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := config.Config{
		ZoneURL:       srv.URL,
		DataDir:       dir,
		FetchInterval: 0,
	}

	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("first runOnce() error = %v", err)
	}
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("second runOnce() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "root.zone"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != newZone {
		t.Errorf("saved zone mismatch\n got: %q\nwant: %q", string(data), newZone)
	}
}

func TestRunOnceNoChanges(t *testing.T) {
	zoneData := ".	86400	IN	NS	a.root-servers.net.\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(zoneData))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := config.Config{
		ZoneURL:       srv.URL,
		DataDir:       dir,
		FetchInterval: 0,
	}

	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("first runOnce() error = %v", err)
	}
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("second runOnce() error = %v", err)
	}
}

func TestRunOnceRecordsHistory(t *testing.T) {
	oldZone := ".\t86400\tIN\tSOA\ta.root-servers.net. nstld.verisign-grs.com. 2026072400 1800 900 604800 86400\n" +
		".\t86400\tIN\tNS\ta.root-servers.net.\n"
	newZone := ".\t86400\tIN\tSOA\ta.root-servers.net. nstld.verisign-grs.com. 2026072500 1800 900 604800 86400\n" +
		".\t86400\tIN\tNS\ta.root-servers.net.\n" +
		"bbb.\t172800\tIN\tNS\tns1.bbb.\n"

	var requestCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 1 {
			_, _ = w.Write([]byte(oldZone))
		} else {
			_, _ = w.Write([]byte(newZone))
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfg := config.Config{
		ZoneURL:       srv.URL,
		DataDir:       dir,
		FetchInterval: 0,
	}

	// 初回実行: 前回スナップショットがないため履歴は記録されない
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("first runOnce() error = %v", err)
	}
	h := store.NewHistory(dir)
	entries, err := h.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("history after initial run = %d entries, want 0", len(entries))
	}

	// 2回目: 変更が検知され履歴に記録される
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("second runOnce() error = %v", err)
	}
	entries, err = h.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.OldSerial != "2026072400" || e.NewSerial != "2026072500" {
		t.Errorf("OldSerial=%q NewSerial=%q", e.OldSerial, e.NewSerial)
	}
	if e.Summary.Total != 2 {
		t.Errorf("Summary.Total = %d, want 2 (SOA modified + NS added)", e.Summary.Total)
	}
}

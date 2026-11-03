package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yfujii/dns-root-diff/internal/config"
	"github.com/yfujii/dns-root-diff/internal/store"
)

func TestRunOnceKeepsZoneBaselineOnEmptyPayload(t *testing.T) {
	const goodZone = ".\t86400\tIN\tNS\ta.root-servers.net.\n"
	calls := 0
	zoneSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			_, _ = w.Write([]byte(goodZone))
			return
		}
		// 200 で空白だけを返す (fetcher は長さ 0 ではないので通す)。
		_, _ = w.Write([]byte("\n"))
	}))
	defer zoneSrv.Close()

	dir := t.TempDir()
	cfg := config.Config{ZoneURL: zoneSrv.URL, DataDir: dir}
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("first runOnce() error = %v", err)
	}

	err := runOnce(context.Background(), cfg, "")
	if err == nil {
		t.Fatal("second runOnce() error = nil, want empty zone parse error")
	}
	if !strings.Contains(err.Error(), "parse zone") {
		t.Errorf("runOnce() error = %v, want parse zone failure", err)
	}

	data, loadErr := store.New(dir).Load()
	if loadErr != nil {
		t.Fatalf("load zone snapshot: %v", loadErr)
	}
	if string(data) != goodZone {
		t.Errorf("zone snapshot was overwritten by an empty payload: %q", data)
	}
	entries, listErr := store.NewHistory(dir).List()
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(entries) != 0 {
		t.Errorf("history entries = %d, want 0", len(entries))
	}
}

func TestRunOnceKeepsAnchorBaselineOnInvalidXML(t *testing.T) {
	const goodZone = ".\t86400\tIN\tNS\ta.root-servers.net.\n"
	zoneSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(goodZone))
	}))
	defer zoneSrv.Close()

	anchorCalls := 0
	anchorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		anchorCalls++
		if anchorCalls == 1 {
			_, _ = w.Write([]byte(anchorXML))
			return
		}
		// 200 で well-formed なエラー XML を返す。
		_, _ = w.Write([]byte(`<Error><Code>AccessDenied</Code></Error>`))
	}))
	defer anchorSrv.Close()

	dir := t.TempDir()
	cfg := config.Config{
		ZoneURL:   zoneSrv.URL,
		AnchorURL: anchorSrv.URL,
		DataDir:   dir,
	}
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("first runOnce() error = %v", err)
	}

	err := runOnce(context.Background(), cfg, "")
	if err == nil {
		t.Fatal("second runOnce() error = nil, want anchor parse error")
	}
	if !strings.Contains(err.Error(), "parse root anchors") {
		t.Errorf("runOnce() error = %v, want parse root anchors failure", err)
	}

	data, loadErr := store.NewAnchor(dir).Load()
	if loadErr != nil {
		t.Fatalf("load anchor snapshot: %v", loadErr)
	}
	if string(data) != anchorXML {
		t.Errorf("anchor snapshot was overwritten by an invalid payload:\n%s", data)
	}
	entries, listErr := store.NewAnchorHistory(dir).List()
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(entries) != 0 {
		t.Errorf("anchor history entries = %d, want 0", len(entries))
	}
}

func TestRunOnceRebaselinesOnCorruptZoneSnapshot(t *testing.T) {
	const goodZone = ".\t86400\tIN\tNS\ta.root-servers.net.\n"
	zoneSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(goodZone))
	}))
	defer zoneSrv.Close()

	dir := t.TempDir()
	if err := store.New(dir).Save([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{ZoneURL: zoneSrv.URL, DataDir: dir}
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runOnce() error = %v, want automatic re-baseline", err)
	}

	data, err := store.New(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != goodZone {
		t.Errorf("zone snapshot = %q, want the new baseline", data)
	}
	entries, err := store.NewHistory(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("history entries = %d, want 0 after re-baseline", len(entries))
	}
}

func TestRunOnceRebaselinesOnCorruptAnchorSnapshot(t *testing.T) {
	const goodZone = ".\t86400\tIN\tNS\ta.root-servers.net.\n"
	zoneSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(goodZone))
	}))
	defer zoneSrv.Close()
	anchorSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(anchorXML))
	}))
	defer anchorSrv.Close()

	dir := t.TempDir()
	if err := store.NewAnchor(dir).Save([]byte(`<Error><Code>AccessDenied</Code></Error>`)); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ZoneURL:   zoneSrv.URL,
		AnchorURL: anchorSrv.URL,
		DataDir:   dir,
	}
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runOnce() error = %v, want automatic re-baseline", err)
	}

	data, err := store.NewAnchor(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != anchorXML {
		t.Errorf("anchor snapshot was not re-baselined:\n%s", data)
	}
	entries, err := store.NewAnchorHistory(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("anchor history entries = %d, want 0 after re-baseline", len(entries))
	}
}

func TestRunOnceRebaselinesOnCorruptTLDSnapshot(t *testing.T) {
	const goodZone = ".\t86400\tIN\tNS\ta.root-servers.net.\n"
	zoneSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(goodZone))
	}))
	defer zoneSrv.Close()
	tldData := tldList("2026102900", "COM", "NET")
	tldSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(tldData))
	}))
	defer tldSrv.Close()

	dir := t.TempDir()
	if err := store.NewTLD(dir).Save([]byte("\n")); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ZoneURL: zoneSrv.URL,
		TLDURL:  tldSrv.URL,
		DataDir: dir,
	}
	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runOnce() error = %v, want automatic re-baseline", err)
	}

	data, err := store.NewTLD(dir).Load()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != tldData {
		t.Errorf("TLD snapshot was not re-baselined:\n%s", data)
	}
	entries, err := store.NewTLDHistory(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("TLD history entries = %d, want 0 after re-baseline", len(entries))
	}
}

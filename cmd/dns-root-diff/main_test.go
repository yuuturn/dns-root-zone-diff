package main

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

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

// resigningZones は再署名だけが起きた2世代のゾーンを返す (RRSIG 入れ替え + SOA serial bump)。
func resigningZones() (oldZone, newZone string) {
	base := ".\t86400\tIN\tNS\ta.root-servers.net.\n" +
		"bbb.\t172800\tIN\tNS\tns1.bbb.\n"
	soa := func(serial string) string {
		return ".\t86400\tIN\tSOA\ta.root-servers.net. nstld.verisign-grs.com. " + serial + " 1800 900 604800 86400\n"
	}
	rrsig := func(expiry string) string {
		return "bbb.\t86400\tIN\tRRSIG\tDS 8 1 86400 " + expiry + " 20260724040000 57780 . AAAA\n"
	}
	return soa("2026072400") + base + rrsig("20260806050000"),
		soa("2026072500") + base + rrsig("20260806170000")
}

func TestRunOnceSkipsNotifyForResigningOnly(t *testing.T) {
	oldZone, newZone := resigningZones()

	var mu sync.Mutex
	var notified int
	slack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		notified++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer slack.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(newZone))
	}))
	defer srv.Close()

	dir := t.TempDir()
	// 前回スナップショットを用意し、再署名だけが起きた1回分を実行する。
	if err := store.New(dir).Save([]byte(oldZone)); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ZoneURL:       srv.URL,
		DataDir:       dir,
		FetchInterval: 0,
		Slack:         config.SlackConfig{Enabled: true, WebhookURL: slack.URL},
	}

	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}

	mu.Lock()
	got := notified
	mu.Unlock()
	if got != 0 {
		t.Errorf("notified %d times for re-signing only changes, want 0", got)
	}

	// 通知しない回も履歴には残す。
	entries, err := store.NewHistory(dir).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("history entries = %d, want 1", len(entries))
	}
	if entries[0].Summary.ByCategory["signature"] == 0 {
		t.Errorf("history should record the signature changes: %+v", entries[0].Summary)
	}
}

func TestRunOnceNotifiesSubstantiveChanges(t *testing.T) {
	oldZone, newZone := resigningZones()
	// 再署名に加えて新規委譲がある回は通知する。
	newZone += "ccc.\t172800\tIN\tNS\tns1.ccc.\n"

	var mu sync.Mutex
	var texts []string
	slack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]string
		_ = json.Unmarshal(body, &payload)
		mu.Lock()
		texts = append(texts, payload["text"])
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer slack.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(newZone))
	}))
	defer srv.Close()

	dir := t.TempDir()
	if err := store.New(dir).Save([]byte(oldZone)); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		ZoneURL:       srv.URL,
		DataDir:       dir,
		FetchInterval: 0,
		Slack:         config.SlackConfig{Enabled: true, WebhookURL: slack.URL},
	}

	if err := runOnce(context.Background(), cfg, ""); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(texts) != 1 {
		t.Fatalf("notified %d times, want 1", len(texts))
	}
	if !strings.Contains(texts[0], "+ ccc. NS ns1.ccc.") {
		t.Errorf("notification should describe the new delegation:\n%s", texts[0])
	}
	// RRSIG は (Name, Type) が1レコードのみなので modified 1件に畳まれる。
	if !strings.Contains(texts[0], "re-signing: 1 RRSIG (omitted)") {
		t.Errorf("notification should summarize the re-signing noise:\n%s", texts[0])
	}
}

func TestRunLoopWebListenError(t *testing.T) {
	// 先にポートを占有して web サーバーの listen を失敗させる
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()

	cfg := config.Config{
		ZoneURL:       "http://127.0.0.1:0/unused",
		DataDir:       t.TempDir(),
		FetchInterval: time.Hour,
		Web: config.WebConfig{
			Enabled: true,
			Listen:  ln.Addr().String(),
		},
	}

	errCh := make(chan error, 1)
	go func() { errCh <- runLoop(cfg, "") }()

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatal("runLoop() = nil, want listen error")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runLoop() did not return on web listen failure")
	}
}

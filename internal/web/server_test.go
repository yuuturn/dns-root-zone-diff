package web

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/yfujii/dns-root-diff/internal/store"
)

// mockHistory は HistoryReader のテスト用実装。
type mockHistory struct {
	entries []store.Entry
}

func (m *mockHistory) List() ([]store.Entry, error) {
	return m.entries, nil
}

func (m *mockHistory) Get(id string) (store.Entry, error) {
	if !strings.Contains(id, "-") || strings.ContainsAny(id, "/. ") {
		return store.Entry{}, fmt.Errorf("%w: %q", store.ErrInvalidID, id)
	}
	for _, e := range m.entries {
		if e.ID == id {
			return e, nil
		}
	}
	return store.Entry{}, fmt.Errorf("not found: %w", fs.ErrNotExist)
}

func testEntries(n int) []store.Entry {
	entries := make([]store.Entry, 0, n)
	for i := n; i >= 1; i-- {
		ts := time.Date(2026, 7, i, 6, 0, 0, 0, time.UTC)
		entries = append(entries, store.Entry{
			ID:         fmt.Sprintf("%sT060000Z-20260700", ts.Format("20060102")),
			DetectedAt: ts,
			OldSerial:  "old",
			NewSerial:  "new",
			Summary:    store.Summary{Total: 1, Added: 1, ByCategory: map[string]int{"delegation": 1}},
			Changes: []store.ChangeJSON{
				{Kind: "added", Category: "delegation", Name: "example.", Type: "NS", NewTTL: 172800, NewRData: "a.nic.example."},
			},
		})
	}
	return entries
}

func testStatic() fs.FS {
	return fstest.MapFS{
		"index.html":         {Data: []byte("<html>spa</html>")},
		"assets/app-abc.js":  {Data: []byte("console.log('app')")},
		"assets/app-abc.css": {Data: []byte("body{}")},
	}
}

func newTestServer(entries []store.Entry) *httptest.Server {
	s := New(&mockHistory{entries: entries}, testStatic())
	return httptest.NewServer(s.Handler())
}

func getJSON(t *testing.T, url string, wantStatus int) map[string]any {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != wantStatus {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return body
}

func TestListDiffs(t *testing.T) {
	srv := newTestServer(testEntries(3))
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/diffs", http.StatusOK)
	if body["total"].(float64) != 3 {
		t.Errorf("total = %v, want 3", body["total"])
	}
	diffs := body["diffs"].([]any)
	if len(diffs) != 3 {
		t.Fatalf("len(diffs) = %d, want 3", len(diffs))
	}
	first := diffs[0].(map[string]any)
	if first["id"] != "20260703T060000Z-20260700" {
		t.Errorf("diffs[0].id = %v, want newest", first["id"])
	}
	if _, ok := first["changes"]; ok {
		t.Error("list response should not include changes")
	}
	if first["summary"].(map[string]any)["total"].(float64) != 1 {
		t.Errorf("summary.total = %v", first["summary"])
	}
}

func TestListDiffsPagination(t *testing.T) {
	srv := newTestServer(testEntries(25))
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/diffs?page=2&per_page=10", http.StatusOK)
	if body["total"].(float64) != 25 {
		t.Errorf("total = %v, want 25", body["total"])
	}
	if body["page"].(float64) != 2 || body["per_page"].(float64) != 10 {
		t.Errorf("page=%v per_page=%v", body["page"], body["per_page"])
	}
	diffs := body["diffs"].([]any)
	if len(diffs) != 10 {
		t.Fatalf("len(diffs) = %d, want 10", len(diffs))
	}
	first := diffs[0].(map[string]any)
	if first["id"] != "20260715T060000Z-20260700" {
		t.Errorf("page 2 first id = %v", first["id"])
	}

	// 範囲外ページは空リスト
	body = getJSON(t, srv.URL+"/api/diffs?page=99&per_page=10", http.StatusOK)
	if len(body["diffs"].([]any)) != 0 {
		t.Errorf("out-of-range page should return empty list")
	}

	// 不正なパラメータはデフォルトに補正
	body = getJSON(t, srv.URL+"/api/diffs?page=-1&per_page=abc", http.StatusOK)
	if body["page"].(float64) != 1 || body["per_page"].(float64) != 20 {
		t.Errorf("invalid params: page=%v per_page=%v, want 1/20", body["page"], body["per_page"])
	}
}

func TestGetDiff(t *testing.T) {
	entries := testEntries(1)
	srv := newTestServer(entries)
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/diffs/"+entries[0].ID, http.StatusOK)
	if body["id"] != entries[0].ID {
		t.Errorf("id = %v", body["id"])
	}
	changes := body["changes"].([]any)
	if len(changes) != 1 {
		t.Fatalf("len(changes) = %d, want 1", len(changes))
	}
	c := changes[0].(map[string]any)
	if c["kind"] != "added" || c["name"] != "example." {
		t.Errorf("changes[0] = %v", c)
	}
}

func TestGetDiffNotFound(t *testing.T) {
	srv := newTestServer(testEntries(1))
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/diffs/20990101T000000Z-2099010100", http.StatusNotFound)
	if body["error"] == "" {
		t.Error("error message missing")
	}
}

func TestGetDiffInvalidID(t *testing.T) {
	srv := newTestServer(testEntries(1))
	defer srv.Close()

	getJSON(t, srv.URL+"/api/diffs/invalid%20id", http.StatusBadRequest)
}

func TestHealth(t *testing.T) {
	srv := newTestServer(nil)
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/health", http.StatusOK)
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
}

func TestStaticFile(t *testing.T) {
	srv := newTestServer(nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/assets/app-abc.js")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q, want immutable for hashed assets", cc)
	}
}

func TestSPAFallback(t *testing.T) {
	srv := newTestServer(nil)
	defer srv.Close()

	for _, path := range []string{"/", "/diffs/20260725T060000Z-20260700", "/nonexistent"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body := make([]byte, 64)
		n, _ := resp.Body.Read(body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s status = %d, want 200", path, resp.StatusCode)
		}
		if !strings.Contains(string(body[:n]), "spa") {
			t.Errorf("GET %s did not serve index.html: %q", path, string(body[:n]))
		}
		if cc := resp.Header.Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Errorf("GET %s Cache-Control = %q, index.html must not be immutable", path, cc)
		}
	}
}

func TestAPIUnknownPathNotSPA(t *testing.T) {
	srv := newTestServer(nil)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /api/nonexistent status = %d, want 404", resp.StatusCode)
	}
}

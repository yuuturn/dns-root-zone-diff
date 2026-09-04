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

func u32ptr(v uint32) *uint32 { return &v }

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
				{Kind: "added", Category: "delegation", Name: "example.", Type: "NS", NewTTL: u32ptr(172800), NewRData: "a.nic.example."},
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
	return newTestServerWithAnchors(entries, nil)
}

func newTestServerWithAnchors(entries, anchorEntries []store.Entry) *httptest.Server {
	s := New(&mockHistory{entries: entries}, &mockHistory{entries: anchorEntries}, testStatic())
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

	// 巨大な page 値でも整数オーバーフローで panic せず空リストを返す
	for _, q := range []string{
		"page=9223372036854775807",
		"page=9223372036854775807&per_page=100",
		"page=92233720368547758",
	} {
		body = getJSON(t, srv.URL+"/api/diffs?"+q, http.StatusOK)
		if len(body["diffs"].([]any)) != 0 {
			t.Errorf("huge page (%s) should return empty list", q)
		}
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

// detailTestEntry は詳細ページングのテスト用に 200 changes を持つエントリを生成する。
// 最初の 150 件は signature、残り 50 件は delegation。
func detailTestEntry() store.Entry {
	changes := make([]store.ChangeJSON, 0, 200)
	for i := 0; i < 200; i++ {
		category := "signature"
		if i >= 150 {
			category = "delegation"
		}
		changes = append(changes, store.ChangeJSON{
			Kind:     "added",
			Category: category,
			Name:     fmt.Sprintf("node%03d.example.", i),
			Type:     "A",
			NewTTL:   u32ptr(300),
			NewRData: "1.2.3.4",
		})
	}
	return store.Entry{
		ID:         "20260701T060000Z-2026070100",
		DetectedAt: time.Date(2026, 7, 1, 6, 0, 0, 0, time.UTC),
		OldSerial:  "old",
		NewSerial:  "new",
		Summary: store.Summary{
			Total:      200,
			Added:      200,
			ByCategory: map[string]int{"signature": 150, "delegation": 50},
		},
		Changes: changes,
	}
}

func TestGetDiffDetailPagination(t *testing.T) {
	entry := detailTestEntry()
	srv := newTestServer([]store.Entry{entry})
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/diffs/"+entry.ID+"?page=2&per_page=100", http.StatusOK)
	if body["id"] != entry.ID {
		t.Errorf("id = %v", body["id"])
	}
	if body["changes_total"].(float64) != 200 {
		t.Errorf("changes_total = %v, want 200", body["changes_total"])
	}
	if body["page"].(float64) != 2 || body["per_page"].(float64) != 100 {
		t.Errorf("page=%v per_page=%v, want 2/100", body["page"], body["per_page"])
	}
	if body["total_pages"].(float64) != 2 {
		t.Errorf("total_pages = %v, want 2", body["total_pages"])
	}
	changes := body["changes"].([]any)
	if len(changes) != 100 {
		t.Fatalf("len(changes) = %d, want 100", len(changes))
	}
	first := changes[0].(map[string]any)
	if first["name"] != "node100.example." {
		t.Errorf("changes[0].name = %v, want node100.example.", first["name"])
	}

	// 範囲外ページは空リスト (total などのメタ情報は維持)
	body = getJSON(t, srv.URL+"/api/diffs/"+entry.ID+"?page=99&per_page=100", http.StatusOK)
	if len(body["changes"].([]any)) != 0 {
		t.Error("out-of-range page should return empty changes")
	}
	if body["changes_total"].(float64) != 200 {
		t.Errorf("changes_total = %v, want 200", body["changes_total"])
	}
}

func TestGetDiffDetailCategoryPagination(t *testing.T) {
	entry := detailTestEntry()
	srv := newTestServer([]store.Entry{entry})
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/diffs/"+entry.ID+"?category=signature&page=2&per_page=100", http.StatusOK)
	if body["changes_total"].(float64) != 150 {
		t.Errorf("changes_total = %v, want 150", body["changes_total"])
	}
	if body["total_pages"].(float64) != 2 {
		t.Errorf("total_pages = %v, want 2", body["total_pages"])
	}
	changes := body["changes"].([]any)
	if len(changes) != 50 {
		t.Fatalf("len(changes) = %d, want 50", len(changes))
	}
	for i, c := range changes {
		if got := c.(map[string]any)["category"]; got != "signature" {
			t.Fatalf("changes[%d].category = %v, want signature", i, got)
		}
	}
	first := changes[0].(map[string]any)
	if first["name"] != "node100.example." {
		t.Errorf("changes[0].name = %v, want node100.example.", first["name"])
	}
}

func TestGetDiffDetailCategoryOnly(t *testing.T) {
	entry := detailTestEntry()
	srv := newTestServer([]store.Entry{entry})
	defer srv.Close()

	// page/per_page を指定しない場合は従来どおり全件 (フィルタのみ適用)
	body := getJSON(t, srv.URL+"/api/diffs/"+entry.ID+"?category=delegation", http.StatusOK)
	changes := body["changes"].([]any)
	if len(changes) != 50 {
		t.Fatalf("len(changes) = %d, want 50", len(changes))
	}
	if _, ok := body["changes_total"]; ok {
		t.Error("non-paged response should not include changes_total")
	}
}

func TestGetDiffDetailInvalidParams(t *testing.T) {
	entry := detailTestEntry()
	srv := newTestServer([]store.Entry{entry})
	defer srv.Close()

	// 不正パラメータはデフォルトに補正、per_page は上限 100 にクランプ
	body := getJSON(t, srv.URL+"/api/diffs/"+entry.ID+"?page=abc&per_page=5000", http.StatusOK)
	if body["page"].(float64) != 1 {
		t.Errorf("page = %v, want 1", body["page"])
	}
	if body["per_page"].(float64) != 100 {
		t.Errorf("per_page = %v, want 100 (clamped)", body["per_page"])
	}
	changes := body["changes"].([]any)
	if len(changes) != 100 {
		t.Errorf("len(changes) = %d, want 100", len(changes))
	}

	// 巨大な page 値でも panic せず空リスト
	body = getJSON(t, srv.URL+"/api/diffs/"+entry.ID+"?page=9223372036854775807&per_page=100", http.StatusOK)
	if len(body["changes"].([]any)) != 0 {
		t.Error("huge page should return empty changes")
	}
}

func TestGetDiffDetailETagPerVariant(t *testing.T) {
	entry := detailTestEntry()
	srv := newTestServer([]store.Entry{entry})
	defer srv.Close()

	url1 := srv.URL + "/api/diffs/" + entry.ID + "?page=1&per_page=100"
	url2 := srv.URL + "/api/diffs/" + entry.ID + "?page=2&per_page=100"

	etag1 := getETag(t, url1)
	etag2 := getETag(t, url2)
	if etag1 == "" || etag2 == "" {
		t.Fatal("ETag missing")
	}
	if etag1 == etag2 {
		t.Errorf("ETag should differ between page variants: %s", etag1)
	}

	// 別ページの ETag を送っても 304 にはならない
	req, _ := http.NewRequest("GET", url2, nil)
	req.Header.Set("If-None-Match", etag1)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status with mismatched ETag = %d, want 200", resp.StatusCode)
	}

	// 一致する ETag なら 304
	req, _ = http.NewRequest("GET", url2, nil)
	req.Header.Set("If-None-Match", etag2)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotModified {
		t.Errorf("status with matched ETag = %d, want 304", resp.StatusCode)
	}
}

func getETag(t *testing.T, url string) string {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", url, resp.StatusCode, http.StatusOK)
	}
	return resp.Header.Get("ETag")
}

func TestGetAnchorDiffDetailPagination(t *testing.T) {
	entry := detailTestEntry()
	srv := newTestServerWithAnchors(nil, []store.Entry{entry})
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/anchors/diffs/"+entry.ID+"?category=delegation&page=1&per_page=10", http.StatusOK)
	if body["changes_total"].(float64) != 50 {
		t.Errorf("changes_total = %v, want 50", body["changes_total"])
	}
	if body["total_pages"].(float64) != 5 {
		t.Errorf("total_pages = %v, want 5", body["total_pages"])
	}
	changes := body["changes"].([]any)
	if len(changes) != 10 {
		t.Fatalf("len(changes) = %d, want 10", len(changes))
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

func TestListAnchorDiffs(t *testing.T) {
	entries := testEntries(2)
	srv := newTestServerWithAnchors(nil, entries)
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/anchors/diffs", http.StatusOK)
	if body["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", body["total"])
	}
	diffs := body["diffs"].([]any)
	if len(diffs) != 2 {
		t.Fatalf("len(diffs) = %d, want 2", len(diffs))
	}
	if _, ok := diffs[0].(map[string]any)["changes"]; ok {
		t.Error("anchor list response should not include changes")
	}
}

func TestGetAnchorDiff(t *testing.T) {
	entries := testEntries(1)
	srv := newTestServerWithAnchors(nil, entries)
	defer srv.Close()

	body := getJSON(t, srv.URL+"/api/anchors/diffs/"+entries[0].ID, http.StatusOK)
	if body["id"] != entries[0].ID {
		t.Errorf("id = %v", body["id"])
	}
	if _, ok := body["changes"]; !ok {
		t.Error("detail response should include changes")
	}
}

func TestGetAnchorDiffNotFound(t *testing.T) {
	srv := newTestServerWithAnchors(nil, nil)
	defer srv.Close()

	getJSON(t, srv.URL+"/api/anchors/diffs/no-such-id", http.StatusNotFound)
}

func TestGetAnchorDiffInvalidID(t *testing.T) {
	srv := newTestServerWithAnchors(nil, nil)
	defer srv.Close()

	getJSON(t, srv.URL+"/api/anchors/diffs/invalid%20id", http.StatusBadRequest)
}

func TestAnchorAndZoneListsAreSeparate(t *testing.T) {
	zoneEntries := testEntries(3)
	anchorEntries := testEntries(1)
	srv := newTestServerWithAnchors(zoneEntries, anchorEntries)
	defer srv.Close()

	zoneBody := getJSON(t, srv.URL+"/api/diffs", http.StatusOK)
	if zoneBody["total"].(float64) != 3 {
		t.Errorf("zone total = %v, want 3", zoneBody["total"])
	}
	anchorBody := getJSON(t, srv.URL+"/api/anchors/diffs", http.StatusOK)
	if anchorBody["total"].(float64) != 1 {
		t.Errorf("anchor total = %v, want 1", anchorBody["total"])
	}
}

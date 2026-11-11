package tldhistory

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/yfujii/dns-root-diff/internal/store"
)

func u32ptr(v uint32) *uint32 { return &v }

// writeDiff は history の 1 エントリをテスト用ディレクトリに書き込む。
func writeDiff(t *testing.T, dataDir, id string, detectedAt time.Time, changes []store.ChangeJSON) {
	t.Helper()
	dir := filepath.Join(dataDir, "diffs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	e := store.Entry{
		ID:         id,
		DetectedAt: detectedAt,
		OldSerial:  "old",
		NewSerial:  "new",
		Summary:    store.Summary{Total: len(changes)},
		Changes:    changes,
	}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func change(kind, category, name, rrType, rdata string) store.ChangeJSON {
	return store.ChangeJSON{
		Kind:     kind,
		Category: category,
		Name:     name,
		Type:     rrType,
		NewTTL:   u32ptr(300),
		NewRData: rdata,
	}
}

func TestTLDOf(t *testing.T) {
	tests := []struct {
		name string
		want string
		ok   bool
	}{
		{"com.", "com", true},
		{"COM.", "com", true},
		{"example.", "example", true},
		{"ns1.example.", "example", true},
		{"a.gtld-servers.net.", "net", true},
		{"xn--p1ai.", "xn--p1ai", true},
		{".", "", false},
		{"", "", false},
		{"   ", "", false},
		{"invalid name.", "", false},
		{"bad_label.", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := TLDOf(tt.name)
			if ok != tt.ok || got != tt.want {
				t.Errorf("TLDOf(%q) = (%q, %v), want (%q, %v)", tt.name, got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestNormalizeTLD(t *testing.T) {
	for _, in := range []string{"com", "COM", "com.", ".com", " com. "} {
		got, err := NormalizeTLD(in)
		if err != nil || got != "com" {
			t.Errorf("NormalizeTLD(%q) = (%q, %v), want com", in, got, err)
		}
	}
	for _, in := range []string{"", ".", "bad name", "example.com", "a_b"} {
		if _, err := NormalizeTLD(in); !errors.Is(err, ErrInvalidTLD) {
			t.Errorf("NormalizeTLD(%q) error = %v, want ErrInvalidTLD", in, err)
		}
	}
}

func seedHistory(t *testing.T) string {
	t.Helper()
	dataDir := t.TempDir()
	base := time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC)

	writeDiff(t, dataDir, "20260101T060000Z-2026010101", base, []store.ChangeJSON{
		change("added", "delegation", "example.", "NS", "a.nic.example."),
		change("added", "glue", "ns1.example.", "A", "192.0.2.1"),
		change("added", "DNSSEC", "example.", "DS", "12345 8 2 ABCD"),
		change("added", "delegation", "com.", "NS", "a.gtld-servers.net."),
		change("added", "glue", "a.gtld-servers.net.", "A", "192.0.2.2"),
		change("added", "signature", "example.", "RRSIG", "NS 8 1 300 20300101 20260101 1 example. ABC"),
		change("modified", "zone", ".", "SOA", "a.root-servers.net. nstld.example. 1 1800 900 604800 86400"),
		change("added", "signature", "org.", "RRSIG", "NS 8 1 300 20300101 20260101 1 org. ABC"),
	})

	writeDiff(t, dataDir, "20260102T060000Z-2026010201", base.Add(24*time.Hour), []store.ChangeJSON{
		change("removed", "delegation", "example.", "NS", "a.nic.example."),
		change("added", "glue", "ns2.example.", "AAAA", "2001:db8::1"),
		change("modified", "delegation", "com.", "NS", "a.gtld-servers.net."),
	})

	writeDiff(t, dataDir, "20260103T060000Z-2026010301", base.Add(48*time.Hour), []store.ChangeJSON{
		change("added", "DNSSEC", "net.", "DS", "54321 8 2 EFGH"),
	})

	return dataDir
}

func TestListGroupsByTLDAndSortsByLatest(t *testing.T) {
	idx := NewIndex(seedHistory(t))

	summaries, err := idx.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 3 {
		t.Fatalf("len(summaries) = %d, want 3: %+v", len(summaries), summaries)
	}

	// net が最新 (1/3)、次に com / example (1/2、同時刻は TLD 昇順)。
	if summaries[0].TLD != "net" || summaries[1].TLD != "com" || summaries[2].TLD != "example" {
		t.Fatalf("order = [%s, %s, %s], want [net, com, example]",
			summaries[0].TLD, summaries[1].TLD, summaries[2].TLD)
	}

	want := map[string]struct {
		events   int
		changes  int
		category map[string]int
	}{
		"example": {
			events:  2,
			changes: 5,
			category: map[string]int{
				"delegation": 2,
				"glue":       2,
				"DNSSEC":     1,
			},
		},
		"com": {
			events:   2,
			changes:  2,
			category: map[string]int{"delegation": 2},
		},
		"net": {
			events:   2,
			changes:  2,
			category: map[string]int{"glue": 1, "DNSSEC": 1},
		},
	}
	for _, got := range summaries {
		w := want[got.TLD]
		if got.Events != w.events || got.Changes != w.changes {
			t.Errorf("%s: events=%d changes=%d, want %d/%d",
				got.TLD, got.Events, got.Changes, w.events, w.changes)
		}
		if len(got.ByCategory) != len(w.category) {
			t.Errorf("%s: by_category=%v, want %v", got.TLD, got.ByCategory, w.category)
			continue
		}
		for k, v := range w.category {
			if got.ByCategory[k] != v {
				t.Errorf("%s: by_category[%s] = %d, want %d", got.TLD, k, got.ByCategory[k], v)
			}
		}
	}

	if got := summaries[1].LastDetectedAt; !got.Equal(time.Date(2026, 1, 2, 6, 0, 0, 0, time.UTC)) {
		t.Errorf("example last_detected_at = %v", got)
	}
}

func TestGetTimeline(t *testing.T) {
	idx := NewIndex(seedHistory(t))

	detail, err := idx.Get("EXAMPLE.")
	if err != nil {
		t.Fatal(err)
	}
	if detail.TLD != "example" {
		t.Errorf("TLD = %q, want example", detail.TLD)
	}
	if len(detail.Events) != 2 {
		t.Fatalf("len(events) = %d, want 2", len(detail.Events))
	}
	if detail.Events[0].ID != "20260102T060000Z-2026010201" {
		t.Errorf("events[0].ID = %q, want newest first", detail.Events[0].ID)
	}
	if detail.Events[0].Summary.Total != 2 {
		t.Errorf("events[0].summary.total = %d, want 2", detail.Events[0].Summary.Total)
	}
	// RRSIG (signature) は履歴から除外される。
	for _, ev := range detail.Events {
		for _, c := range ev.Changes {
			if c.Category == "signature" {
				t.Errorf("signature change should be excluded: %+v", c)
			}
		}
	}
	if got := len(detail.Events[1].Changes); got != 3 {
		t.Errorf("events[1] changes = %d, want 3", got)
	}
}

func TestGetUnknownTLDIsEmpty(t *testing.T) {
	idx := NewIndex(seedHistory(t))

	detail, err := idx.Get("no-such-tld")
	if err != nil {
		t.Fatal(err)
	}
	if detail.TLD != "no-such-tld" || len(detail.Events) != 0 {
		t.Errorf("detail = %+v, want empty", detail)
	}
}

func TestGetInvalidTLD(t *testing.T) {
	idx := NewIndex(seedHistory(t))
	if _, err := idx.Get("bad/name"); !errors.Is(err, ErrInvalidTLD) {
		t.Errorf("error = %v, want ErrInvalidTLD", err)
	}
}

func TestIndexPersistsAndRefreshesIncrementally(t *testing.T) {
	dataDir := seedHistory(t)
	idx := NewIndex(dataDir)
	if _, err := idx.List(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dataDir, "tld-history", "index.json")); err != nil {
		t.Fatalf("index file not written: %v", err)
	}

	// 新しいインスタンスは永続インデックスを読み、既存ファイルを再パースしない。
	idx2 := NewIndex(dataDir)
	if _, err := idx2.List(); err != nil {
		t.Fatal(err)
	}
	if idx2.parsed != 0 {
		t.Errorf("parsed = %d, want 0 (loaded from index)", idx2.parsed)
	}

	// 追加された diff だけを取り込む。
	writeDiff(t, dataDir, "20260104T060000Z-2026010401",
		time.Date(2026, 1, 4, 6, 0, 0, 0, time.UTC),
		[]store.ChangeJSON{change("added", "delegation", "newtld.", "NS", "ns1.newtld.")})
	if _, err := idx2.List(); err != nil {
		t.Fatal(err)
	}
	if idx2.parsed != 1 {
		t.Errorf("parsed = %d, want 1 (only the new diff)", idx2.parsed)
	}
	detail, err := idx2.Get("newtld")
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Events) != 1 {
		t.Fatalf("newtld events = %d, want 1", len(detail.Events))
	}
}

func TestIndexRebuildsWhenFilesRemoved(t *testing.T) {
	dataDir := seedHistory(t)
	idx := NewIndex(dataDir)
	if _, err := idx.List(); err != nil {
		t.Fatal(err)
	}

	// 最新の diff を消すと、インデックスは現存ファイルから再構築される。
	if err := os.Remove(filepath.Join(dataDir, "diffs", "20260103T060000Z-2026010301.json")); err != nil {
		t.Fatal(err)
	}
	idx2 := NewIndex(dataDir)
	summaries, err := idx2.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range summaries {
		if s.TLD == "net" && s.Events != 1 {
			t.Errorf("net events = %d, want 1 after removing the DS diff", s.Events)
		}
	}
}

func TestIndexRebuildsCorruptIndex(t *testing.T) {
	dataDir := seedHistory(t)
	idx := NewIndex(dataDir)
	if _, err := idx.List(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dataDir, "tld-history", "index.json")
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatal(err)
	}

	idx2 := NewIndex(dataDir)
	summaries, err := idx2.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 3 {
		t.Errorf("len(summaries) = %d, want 3 after rebuild", len(summaries))
	}
}

func TestIndexWithoutDiffs(t *testing.T) {
	idx := NewIndex(t.TempDir())
	summaries, err := idx.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 0 {
		t.Errorf("len(summaries) = %d, want 0", len(summaries))
	}
}

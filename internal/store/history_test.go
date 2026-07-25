package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

func testChanges() []diff.Change {
	return []diff.Change{
		{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "NS", NewTTL: 172800, NewRData: "a.nic.newgtld."},
		{Kind: diff.ChangeRemoved, Name: "oldgtld.", Type: "NS", OldTTL: 172800, OldRData: "a.nic.oldgtld."},
		{Kind: diff.ChangeModified, Name: "aaa.", Type: "DS", OldTTL: 86400, NewTTL: 86400, OldRData: "1 8 2 OLD", NewRData: "2 8 2 NEW"},
		{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 86400, OldRData: "old serial", NewRData: "new serial"},
	}
}

func TestNewEntry(t *testing.T) {
	detected := time.Date(2026, 7, 25, 6, 30, 0, 0, time.UTC)
	e := NewEntry(detected, "2026072400", "2026072500", testChanges())

	if e.ID != "20260725T063000Z-2026072500" {
		t.Errorf("ID = %q, want %q", e.ID, "20260725T063000Z-2026072500")
	}
	if !e.DetectedAt.Equal(detected) {
		t.Errorf("DetectedAt = %v", e.DetectedAt)
	}
	if e.OldSerial != "2026072400" || e.NewSerial != "2026072500" {
		t.Errorf("OldSerial=%q NewSerial=%q", e.OldSerial, e.NewSerial)
	}
	if e.Summary.Total != 4 {
		t.Errorf("Summary.Total = %d, want 4", e.Summary.Total)
	}
	if e.Summary.Added != 1 || e.Summary.Removed != 1 || e.Summary.Modified != 2 {
		t.Errorf("Summary = %+v", e.Summary)
	}
	if e.Summary.ByCategory["delegation"] != 2 {
		t.Errorf("ByCategory[delegation] = %d, want 2", e.Summary.ByCategory["delegation"])
	}
	if e.Summary.ByCategory["DNSSEC"] != 1 {
		t.Errorf("ByCategory[DNSSEC] = %d, want 1", e.Summary.ByCategory["DNSSEC"])
	}
	if e.Summary.ByCategory["zone"] != 1 {
		t.Errorf("ByCategory[zone] = %d, want 1", e.Summary.ByCategory["zone"])
	}
	if len(e.Changes) != 4 {
		t.Fatalf("len(Changes) = %d, want 4", len(e.Changes))
	}
	c := e.Changes[0]
	if c.Kind != "added" || c.Category != "delegation" || c.Name != "newgtld." {
		t.Errorf("Changes[0] = %+v", c)
	}
	if c.NewTTL == nil || *c.NewTTL != 172800 {
		t.Errorf("Changes[0].NewTTL = %v, want 172800", c.NewTTL)
	}
	// added 変更に旧側は存在しないため old_ttl は持たない
	if c.OldTTL != nil {
		t.Errorf("Changes[0].OldTTL = %v, want nil for added change", c.OldTTL)
	}
}

func TestNewEntryTTLZeroPreserved(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeModified, Name: "aaa.", Type: "NS", OldTTL: 300, NewTTL: 0, OldRData: "a.nic.aaa.", NewRData: "a.nic.aaa."},
		{Kind: diff.ChangeRemoved, Name: "bbb.", Type: "NS", OldTTL: 0, OldRData: "a.nic.bbb."},
	}
	e := NewEntry(time.Date(2026, 7, 25, 6, 30, 0, 0, time.UTC), "old", "new", changes)

	mod := e.Changes[0]
	if mod.OldTTL == nil || *mod.OldTTL != 300 {
		t.Errorf("modified OldTTL = %v, want 300", mod.OldTTL)
	}
	if mod.NewTTL == nil || *mod.NewTTL != 0 {
		t.Errorf("modified NewTTL = %v, want 0", mod.NewTTL)
	}

	// TTL 0 が JSON 出力から消えないこと (omitempty で欠落しない)
	data, err := json.Marshal(mod)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"new_ttl":0`) {
		t.Errorf("marshalled change = %s, want it to contain \"new_ttl\":0", data)
	}

	rem := e.Changes[1]
	if rem.OldTTL == nil || *rem.OldTTL != 0 {
		t.Errorf("removed OldTTL = %v, want 0", rem.OldTTL)
	}
	if rem.NewTTL != nil {
		t.Errorf("removed NewTTL = %v, want nil", rem.NewTTL)
	}
}

func TestHistoryAppendAndGet(t *testing.T) {
	h := NewHistory(t.TempDir())
	e := NewEntry(time.Date(2026, 7, 25, 6, 30, 0, 0, time.UTC), "2026072400", "2026072500", testChanges())

	if err := h.Append(e); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	got, err := h.Get(e.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.ID != e.ID || got.NewSerial != e.NewSerial || len(got.Changes) != len(e.Changes) {
		t.Errorf("Get() = %+v", got)
	}
}

func TestHistoryGetNotExists(t *testing.T) {
	h := NewHistory(t.TempDir())

	_, err := h.Get("20260101T000000Z-2026010100")
	if err == nil {
		t.Fatal("Get() expected error for missing entry")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Get() error = %v, want fs.ErrNotExist", err)
	}
}

func TestHistoryGetInvalidID(t *testing.T) {
	h := NewHistory(t.TempDir())

	for _, id := range []string{"../../../etc/passwd", "foo/bar", "", "abc def"} {
		if _, err := h.Get(id); !errors.Is(err, ErrInvalidID) {
			t.Errorf("Get(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestHistoryListNewestFirst(t *testing.T) {
	h := NewHistory(t.TempDir())

	times := []time.Time{
		time.Date(2026, 7, 23, 4, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 25, 6, 30, 0, 0, time.UTC),
		time.Date(2026, 7, 24, 5, 15, 0, 0, time.UTC),
	}
	for i, ts := range times {
		e := NewEntry(ts, "old", "new", testChanges()[:i+1])
		if err := h.Append(e); err != nil {
			t.Fatalf("Append() error = %v", err)
		}
	}

	entries, err := h.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("len(entries) = %d, want 3", len(entries))
	}
	for i := 1; i < len(entries); i++ {
		if entries[i-1].ID < entries[i].ID {
			t.Errorf("entries not sorted newest first: %q before %q", entries[i-1].ID, entries[i].ID)
		}
	}
	if !entries[0].DetectedAt.Equal(times[1]) {
		t.Errorf("entries[0].DetectedAt = %v, want %v", entries[0].DetectedAt, times[1])
	}
}

func TestHistoryListEmptyDir(t *testing.T) {
	h := NewHistory(filepath.Join(t.TempDir(), "nonexistent"))

	entries, err := h.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("len(entries) = %d, want 0", len(entries))
	}
}

func TestHistoryAppendLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	h := NewHistory(dir)
	e := NewEntry(time.Date(2026, 7, 25, 6, 30, 0, 0, time.UTC), "old", "new", testChanges())

	if err := h.Append(e); err != nil {
		t.Fatalf("Append() error = %v", err)
	}

	files, err := os.ReadDir(filepath.Join(dir, "diffs"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("len(files) = %d, want 1", len(files))
	}
	if files[0].Name() != e.ID+".json" {
		t.Errorf("file name = %q, want %q", files[0].Name(), e.ID+".json")
	}
}

func TestHistoryListSkipsInvalidFiles(t *testing.T) {
	dir := t.TempDir()
	h := NewHistory(dir)
	e := NewEntry(time.Date(2026, 7, 25, 6, 30, 0, 0, time.UTC), "old", "new", testChanges())
	if err := h.Append(e); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diffs", "garbage.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "diffs", "README.txt"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := h.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("len(entries) = %d, want 1", len(entries))
	}
}

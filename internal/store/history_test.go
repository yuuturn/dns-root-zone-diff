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
	// 一時ファイルが残っていないこと。index ファイルは除外して数える。
	var entryFiles []string
	for _, f := range files {
		if strings.HasPrefix(f.Name(), ".tmp-") {
			t.Errorf("temp file remains: %q", f.Name())
		}
		if f.Name() == ".index.json" {
			continue
		}
		entryFiles = append(entryFiles, f.Name())
	}
	if len(entryFiles) != 1 {
		t.Fatalf("len(entryFiles) = %d, want 1 (files=%v)", len(entryFiles), files)
	}
	if entryFiles[0] != e.ID+".json" {
		t.Errorf("file name = %q, want %q", entryFiles[0], e.ID+".json")
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

func TestAnchorHistoryAppendListGet(t *testing.T) {
	dir := t.TempDir()
	h := NewAnchorHistory(dir)
	e := NewEntry(time.Now().UTC(), "old-id", "0C05FDD6-422C-4910-8ED6-430ED15E11C2", []diff.Change{
		{Kind: diff.ChangeAdded, Name: "Kmyv6jo", Type: "DS", NewRData: "38696 8 2 683D"},
	})
	if err := h.Append(e); err != nil {
		t.Fatalf("Append() error = %v (idPattern must allow dashes in serial)", err)
	}
	entries, err := h.List()
	if err != nil || len(entries) != 1 {
		t.Fatalf("List() = %d entries, %v", len(entries), err)
	}
	got, err := h.Get(entries[0].ID)
	if err != nil || got.ID != entries[0].ID {
		t.Fatalf("Get() = %+v, %v", got, err)
	}
	// zone 履歴とは別ディレクトリであること
	zoneH := NewHistory(dir)
	if zs, _ := zoneH.List(); len(zs) != 0 {
		t.Errorf("zone history should be empty, got %d", len(zs))
	}
}

func TestAnchorHistoryRejectsBadID(t *testing.T) {
	h := NewAnchorHistory(t.TempDir())
	bad := NewEntry(time.Now().UTC(), "old", "id with spaces!!", nil)
	if err := h.Append(bad); err == nil {
		t.Error("Append() = nil error, want ErrInvalidID")
	}
}

// root zone の DNSKEY (IANA 公開値: KSK 20326 / ZSK 8763)。
const (
	testKeyKSK = "257 3 8 AwEAAaz/tAm8yTn4Mfeh5eyI96WSVexTBAvkMgJzkKTOiW1vkIbzxeF3+/4RgWOq7HrxRixHlFlExOLAJr5emLvN7SWXgnLh4+B5xQlNVz8Og8kvArMtNROxVQuCaSnIDdD5LKyWbRd2n9WGe2R8PzgCmr3EgVLrjyBxWezF0jLHwVN8efS3rCj/EWgvIWgb9tarpVUDK/b58Da+sqqls3eNbuv7pr+eoZG+SrDK6nWeL3c6H5Apxz7LjVc1uTIdsIXxuOLYA4/ilBmSVIzuDWfdRUfhHdY6+cn8HFRm+2hM8AnXGXws9555KrUB5qihylGa8subX2Nn6UwNR1AkUTV74bU="
	testKeyZSK = "256 3 8 AwEAAcorNxNc9t0ue1ZenPjVMRujlQ2YLBJ0pR/4XMjHby9vr03cRxFSKnWpxeg5PL384D9VeqdS1bjrGyTsHtoagzI3NiXFd8sjHMOnZxu5pp/+UBIg7tasYgVMMdUiM1Yu/c+t30d1Qw/v6NYLNnOnywTu6HuUh7Y1sZRq/GZcrxLENXHQoyKpOrOr8R3+qd6wK69IKB80ZBZtkX9rdEqKJk+yYXjEZzZ0zkAFthmNgCreAoU9PVlU66dyOsaLjf3wKUlUm9SNOKs+tK6LuSmRbK6+1T/tPaRaMBSEuv2G6vXfnJ1uyMbGvtpRn/YDcvTjrp6DzE6XyF44QdwJ3oKx4ek="
)

func TestNewEntryKeyInfo(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: ".", Type: "DNSKEY", NewTTL: 172800, NewRData: testKeyKSK},
		{Kind: diff.ChangeRemoved, Name: ".", Type: "DNSKEY", OldTTL: 172800, OldRData: testKeyZSK},
		{Kind: diff.ChangeModified, Name: ".", Type: "DNSKEY", OldTTL: 172800, NewTTL: 172800, OldRData: testKeyZSK, NewRData: testKeyKSK},
		{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "DS", NewTTL: 172800, NewRData: "46645 13 2 A4DD7495"},
	}
	e := NewEntry(time.Date(2026, 7, 25, 6, 30, 0, 0, time.UTC), "old", "new", changes)

	wantAdded := &KeyJSON{Tag: 20326, Role: "ksk", Size: "2048b"}
	if got := e.Changes[0].NewKey; got == nil || *got != *wantAdded {
		t.Errorf("Changes[0].NewKey = %+v, want %+v", got, wantAdded)
	}
	if e.Changes[0].OldKey != nil {
		t.Errorf("Changes[0].OldKey = %+v, want nil for added change", e.Changes[0].OldKey)
	}

	wantRemoved := &KeyJSON{Tag: 8763, Role: "zsk", Size: "2048b"}
	if got := e.Changes[1].OldKey; got == nil || *got != *wantRemoved {
		t.Errorf("Changes[1].OldKey = %+v, want %+v", got, wantRemoved)
	}

	if got := e.Changes[2].OldKey; got == nil || *got != *wantRemoved {
		t.Errorf("Changes[2].OldKey = %+v, want %+v", got, wantRemoved)
	}
	if got := e.Changes[2].NewKey; got == nil || *got != *wantAdded {
		t.Errorf("Changes[2].NewKey = %+v, want %+v", got, wantAdded)
	}

	// DNSKEY 以外の変更には鍵情報を付けない。
	if e.Changes[3].OldKey != nil || e.Changes[3].NewKey != nil {
		t.Errorf("DS change has key info: %+v", e.Changes[3])
	}
}

func TestKeyJSONJSONRoundTrip(t *testing.T) {
	e := NewEntry(time.Now().UTC(), "old", "new", []diff.Change{
		{Kind: diff.ChangeRemoved, Name: ".", Type: "DNSKEY", OldTTL: 172800, OldRData: testKeyZSK},
	})
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"old_key":{"tag":8763,"role":"zsk","size":"2048b"}`) {
		t.Errorf("marshaled entry does not contain old_key:\n%s", data)
	}
	// 鍵情報が無い変更ではフィールド自体を出さない。
	if strings.Contains(string(data), "new_key") {
		t.Errorf("added-side key field should be omitted:\n%s", data)
	}

	var decoded Entry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := decoded.Changes[0].OldKey; got == nil || got.Tag != 8763 || got.Role != "zsk" || got.Size != "2048b" {
		t.Errorf("decoded OldKey = %+v", got)
	}
}

func TestChangeJSONWithoutKeyInfoUnmarshals(t *testing.T) {
	// 鍵情報を追加する前に保存された履歴も読めること。
	const legacy = `{"id":"20260725T063000Z-1","detected_at":"2026-07-25T06:30:00Z","old_serial":"a","new_serial":"1",
		"summary":{"total":1,"added":0,"removed":1,"modified":0,"by_category":{"DNSSEC":1}},
		"changes":[{"kind":"removed","category":"DNSSEC","name":".","type":"DNSKEY","old_ttl":172800,"old_rdata":"256 3 8 AwEAA"}]}`
	var e Entry
	if err := json.Unmarshal([]byte(legacy), &e); err != nil {
		t.Fatalf("unmarshal legacy entry: %v", err)
	}
	if len(e.Changes) != 1 || e.Changes[0].OldKey != nil {
		t.Errorf("Changes = %+v, want one change without key info", e.Changes)
	}
}

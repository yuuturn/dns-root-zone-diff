package tld

import (
	"strings"
	"testing"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

const sample = `# Version 2026102900, Last Updated Thu Oct 29 07:07:01 2026 UTC
COM
AAA
xn--vermgensberatung-pwb
`

func TestParse(t *testing.T) {
	list, err := Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if list.Version != "2026102900" {
		t.Errorf("Version = %q, want 2026102900", list.Version)
	}
	want := []string{"aaa", "com", "xn--vermgensberatung-pwb"}
	if strings.Join(list.TLDs, ",") != strings.Join(want, ",") {
		t.Errorf("TLDs = %v, want %v (lowercase, sorted)", list.TLDs, want)
	}
}

func TestParseSkipsBlankLinesAndCRLF(t *testing.T) {
	data := "# Version 1, Last Updated now\r\n\r\nCOM\r\n\r\nNET\r\n"
	list, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(list.TLDs) != 2 || list.TLDs[0] != "com" || list.TLDs[1] != "net" {
		t.Errorf("TLDs = %v, want [com net]", list.TLDs)
	}
}

func TestParseDeduplicates(t *testing.T) {
	list, err := Parse([]byte("COM\ncom\nCOM\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(list.TLDs) != 1 {
		t.Errorf("TLDs = %v, want 1 entry", list.TLDs)
	}
}

func TestParseWithoutVersion(t *testing.T) {
	// ヘッダが無くても TLD 集合は扱える (Serial は unknown になる)。
	list, err := Parse([]byte("COM\nNET\n"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if list.Version != "" {
		t.Errorf("Version = %q, want empty", list.Version)
	}
	if len(list.TLDs) != 2 {
		t.Errorf("TLDs = %v, want 2 entries", list.TLDs)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "空", data: ""},
		{name: "コメントのみ", data: "# Version 1, Last Updated now\n"},
		{name: "HTML", data: "<html><body>error</body></html>\n"},
		{name: "空白を含む行", data: "COM\nNOT A TLD\n"},
		{name: "不正な文字", data: "COM\nbad_tld\n"},
		{name: "先頭ハイフン", data: "COM\n-bad\n"},
		{name: "長すぎるラベル", data: "COM\n" + strings.Repeat("a", 64) + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Parse([]byte(tt.data)); err == nil {
				t.Fatal("Parse() = nil, want error")
			}
		})
	}
}

func TestParseErrorMentionsLine(t *testing.T) {
	_, err := Parse([]byte("# Version 1, Last Updated now\nCOM\nbad_tld\n"))
	if err == nil {
		t.Fatal("Parse() = nil, want error")
	}
	if !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error should mention the failing line: %v", err)
	}
}

func TestToRecords(t *testing.T) {
	records := ToRecords(List{TLDs: []string{"com", "aaa"}})
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].Name != "com." || records[1].Name != "aaa." {
		t.Errorf("names = %q, %q; want FQDN form", records[0].Name, records[1].Name)
	}
	for _, r := range records {
		if r.Type != diff.TypeTLD {
			t.Errorf("type = %q, want %q", r.Type, diff.TypeTLD)
		}
		if r.RData != "" {
			t.Errorf("rdata = %q, want empty", r.RData)
		}
	}
}

func TestDiff(t *testing.T) {
	oldList := List{Version: "1", TLDs: []string{"aaa", "com"}}
	newList := List{Version: "2", TLDs: []string{"aaa", "new"}}

	changes := Diff(oldList, newList)
	diff.SortChanges(changes)
	if len(changes) != 2 {
		t.Fatalf("changes = %d, want 2: %+v", len(changes), changes)
	}
	if changes[0].Kind != diff.ChangeRemoved || changes[0].Name != "com." {
		t.Errorf("changes[0] = %+v, want removed com.", changes[0])
	}
	if changes[1].Kind != diff.ChangeAdded || changes[1].Name != "new." {
		t.Errorf("changes[1] = %+v, want added new.", changes[1])
	}
	if changes[1].Type != diff.TypeTLD {
		t.Errorf("Type = %q, want %q", changes[1].Type, diff.TypeTLD)
	}
}

func TestDiffVersionOnly(t *testing.T) {
	// バージョンだけが進み TLD 集合が同じ回は差分なし (通知もしない)。
	oldList := List{Version: "1", TLDs: []string{"com"}}
	newList := List{Version: "2", TLDs: []string{"com"}}
	if changes := Diff(oldList, newList); len(changes) != 0 {
		t.Errorf("changes = %+v, want none", changes)
	}
}

func TestSerial(t *testing.T) {
	if got := Serial(List{Version: "2026102900"}); got != "2026102900" {
		t.Errorf("Serial() = %q, want 2026102900", got)
	}
	if got := Serial(List{}); got != "unknown" {
		t.Errorf("Serial() = %q, want unknown", got)
	}
}

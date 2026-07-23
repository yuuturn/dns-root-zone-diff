package notify

import (
	"strings"
	"testing"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

func TestFormatMessageEmpty(t *testing.T) {
	msg := FormatMessage(nil)
	if msg != "" {
		t.Errorf("FormatMessage(nil) = %q, want empty", msg)
	}
}

func TestFormatMessageAdded(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "NS", NewRData: "ns1.newgtld."},
	}
	msg := FormatMessage(changes)
	if !strings.Contains(msg, "1 change(s)") {
		t.Errorf("missing change count in: %q", msg)
	}
	if !strings.Contains(msg, "[delegation]") {
		t.Errorf("missing delegation category in: %q", msg)
	}
	if !strings.Contains(msg, "+ newgtld. NS") {
		t.Errorf("missing added record in: %q", msg)
	}
}

func TestFormatMessageRemoved(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeRemoved, Name: "oldgtld.", Type: "NS", OldRData: "ns1.oldgtld."},
	}
	msg := FormatMessage(changes)
	if !strings.Contains(msg, "- oldgtld. NS") {
		t.Errorf("missing removed record in: %q", msg)
	}
}

func TestFormatMessageModified(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldRData: "serial 2026072301", NewRData: "serial 2026072302"},
	}
	msg := FormatMessage(changes)
	if !strings.Contains(msg, "~ . SOA") {
		t.Errorf("missing modified record in: %q", msg)
	}
	if !strings.Contains(msg, "old:") || !strings.Contains(msg, "new:") {
		t.Errorf("missing old/new values in: %q", msg)
	}
}

func TestFormatMessageMultipleCategories(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "NS", NewRData: "ns1.newgtld."},
		{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "DS", NewRData: "12345 8 2 ABCDEF"},
		{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldRData: "old", NewRData: "new"},
	}
	msg := FormatMessage(changes)
	if !strings.Contains(msg, "[delegation]") {
		t.Errorf("missing delegation in: %q", msg)
	}
	if !strings.Contains(msg, "[DNSSEC]") {
		t.Errorf("missing DNSSEC in: %q", msg)
	}
	if !strings.Contains(msg, "[other]") {
		t.Errorf("missing other in: %q", msg)
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if truncate(short, 60) != short {
		t.Errorf("truncate(%q, 60) = %q", short, truncate(short, 60))
	}

	long := strings.Repeat("a", 100)
	result := truncate(long, 60)
	if len(result) != 63 { // 60 + "..."
		t.Errorf("truncate(long, 60) len = %d, want 63", len(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated string should end with ...")
	}
}

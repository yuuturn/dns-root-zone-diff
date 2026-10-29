package notify

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/whois"
)

func helsinkiWhois() whois.Info {
	return whois.Info{
		Domain:  "HELSINKI",
		Status:  "FORMER",
		Remarks: "Registration information:",
		Created: "2015-12-10",
		Changed: "2026-10-28",
		Source:  "IANA",
	}
}

func helsinkiInfos() map[string]whois.Info {
	return map[string]whois.Info{"helsinki": helsinkiWhois()}
}

func TestFormatWhoisReply(t *testing.T) {
	got := FormatWhoisReply("helsinki", helsinkiWhois())
	want := `% whois helsinki.
...
domain:       HELSINKI
status:       FORMER
remarks:      Registration information:
created:      2015-12-10
changed:      2026-10-28
source:       IANA`
	if got != want {
		t.Errorf("FormatWhoisReply() =\n%q\nwant:\n%q", got, want)
	}
}

func TestFormatWhoisReplyOmitsEmptyFields(t *testing.T) {
	got := FormatWhoisReply("example", whois.Info{
		Domain:  "EXAMPLE",
		Status:  "ACTIVE",
		Created: "2020-01-01",
		Source:  "IANA",
	})
	want := `% whois example.
...
domain:       EXAMPLE
status:       ACTIVE
created:      2020-01-01
source:       IANA`
	if got != want {
		t.Errorf("FormatWhoisReply() =\n%q\nwant:\n%q", got, want)
	}
	for _, dontWant := range []string{"remarks:", "changed:"} {
		if strings.Contains(got, dontWant) {
			t.Errorf("empty field %q should be omitted:\n%s", dontWant, got)
		}
	}
}

func TestFormatWhoisReplyIDN(t *testing.T) {
	got := FormatWhoisReply("xn--vermgensberatung-pwb", whois.Info{
		Domain:    "vermögensberatung",
		DomainACE: "XN--VERMGENSBERATUNG-PWB",
		Status:    "ACTIVE",
		Source:    "IANA",
	})
	if !strings.Contains(got, "domain:       vermögensberatung") {
		t.Errorf("IDN domain should be shown as returned by IANA:\n%s", got)
	}
}

func TestBuildWhoisReplies(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeRemoved, Name: "helsinki.", Type: diff.TypeTLD},
		{Kind: diff.ChangeAdded, Name: "newtld.", Type: diff.TypeTLD},
		{Kind: diff.ChangeAdded, Name: "unknown.", Type: diff.TypeTLD},
		{Kind: diff.ChangeAdded, Name: "delegation.", Type: "NS", NewRData: "ns1.delegation."},
		{Kind: diff.ChangeModified, Name: "modified.", Type: diff.TypeTLD},
	}
	infos := map[string]whois.Info{
		"helsinki": helsinkiWhois(),
		"newtld":   {Domain: "NEWTLD", Status: "ACTIVE", Created: "2026-10-01", Source: "IANA"},
	}

	replies := buildWhoisReplies(changes, infos)
	if len(replies) != 2 {
		t.Fatalf("replies = %d, want 2: %+v", len(replies), replies)
	}
	if replies[0].tld != "helsinki" || !strings.Contains(replies[0].text, "status:       FORMER") {
		t.Errorf("replies[0] = %+v", replies[0])
	}
	if replies[1].tld != "newtld" || !strings.Contains(replies[1].text, "status:       ACTIVE") {
		t.Errorf("replies[1] = %+v", replies[1])
	}
}

func TestBuildWhoisRepliesNilInfo(t *testing.T) {
	changes := []diff.Change{{Kind: diff.ChangeRemoved, Name: "helsinki.", Type: diff.TypeTLD}}
	if replies := buildWhoisReplies(changes, nil); len(replies) != 0 {
		t.Errorf("replies = %+v, want none", replies)
	}
}

func TestFitWeighted(t *testing.T) {
	if got := fitWeighted("hello", 10); got != "hello" {
		t.Errorf("fitWeighted(short) = %q", got)
	}

	long := strings.Repeat("a", 100)
	got := fitWeighted(long, 20)
	if weightedLen(got) > 20 {
		t.Errorf("fitWeighted() weighted length = %d, want <= 20", weightedLen(got))
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("fitWeighted() = %q, want ellipsis", got)
	}

	// 非 ASCII は重み2で数えられるため、rune 数ではなく重みで収める。
	multibyte := strings.Repeat("あ", 100)
	got = fitWeighted(multibyte, 20)
	if weightedLen(got) > 20 {
		t.Errorf("fitWeighted(multibyte) weighted length = %d, want <= 20", weightedLen(got))
	}
	if !utf8.ValidString(got) {
		t.Error("fitWeighted() broke UTF-8")
	}

	if got := fitWeighted(long, 2); got != ".." {
		t.Errorf("fitWeighted(limit=2) = %q, want %q", got, "..")
	}
}

func TestFitRunes(t *testing.T) {
	if got := fitRunes("hello", 10); got != "hello" {
		t.Errorf("fitRunes(short) = %q", got)
	}
	long := strings.Repeat("あ", 100)
	got := fitRunes(long, 20)
	if n := utf8.RuneCountInString(got); n > 20 {
		t.Errorf("fitRunes() rune length = %d, want <= 20", n)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("fitRunes() = %q, want ellipsis", got)
	}
	if got := fitRunes(long, 2); utf8.RuneCountInString(got) != 2 {
		t.Errorf("fitRunes(limit=2) rune length = %d, want 2", utf8.RuneCountInString(got))
	}
}

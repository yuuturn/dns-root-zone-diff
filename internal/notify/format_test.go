package notify

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

// tweetOpts は X 相当のフォーマットオプション。
func tweetOpts() FormatOptions {
	return FormatOptions{MaxLen: 280, MaxParts: 4, Numbering: true}
}

// resigningChanges は再署名のみの変更 (RRSIG 入れ替え + SOA serial bump) を作る。
func resigningChanges(n int) []diff.Change {
	changes := []diff.Change{
		{Kind: diff.ChangeModified, Name: ".", Type: "SOA",
			OldRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072501 1800 900 604800 86400",
			NewRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072502 1800 900 604800 86400"},
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("tld%d.", i)
		changes = append(changes,
			diff.Change{Kind: diff.ChangeRemoved, Name: name, Type: "RRSIG", OldRData: "DS 8 1 86400 20260806050000 20260724040000 57780 . old"},
			diff.Change{Kind: diff.ChangeAdded, Name: name, Type: "RRSIG", NewRData: "DS 8 1 86400 20260806170000 20260724160000 57780 . new"},
		)
	}
	return changes
}

func TestFormatPostsEmpty(t *testing.T) {
	if got := FormatPosts(nil, tweetOpts()); got != nil {
		t.Errorf("FormatPosts(nil) = %v, want nil", got)
	}
}

func TestFormatPostsResigningOnlyReturnsNil(t *testing.T) {
	changes := resigningChanges(1400)
	if len(changes) < 2000 {
		t.Fatalf("setup: got only %d changes", len(changes))
	}
	if got := FormatPosts(changes, tweetOpts()); got != nil {
		t.Errorf("re-signing only should produce no posts, got %d parts:\n%s", len(got), strings.Join(got, "\n---\n"))
	}
}

func TestFormatPostsZoneOnlyReturnsNil(t *testing.T) {
	// ZONEMD ダイジェストは再署名ごとに変わるため実質的変更として扱わない。
	changes := []diff.Change{
		{Kind: diff.ChangeModified, Name: ".", Type: "ZONEMD", OldRData: "2026072501 1 241 old", NewRData: "2026072502 1 241 new"},
	}
	if got := FormatPosts(changes, tweetOpts()); got != nil {
		t.Errorf("zone-only should produce no posts, got %v", got)
	}
}

func TestFormatPostsReportsNonMechanicalZoneChanges(t *testing.T) {
	tests := []struct {
		name   string
		change diff.Change
		want   string
	}{
		{
			name: "SOA MNAME change",
			change: diff.Change{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 86400,
				OldRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072501 1800 900 604800 86400",
				NewRData: "b.root-servers.net. nstld.verisign-grs.com. 2026072502 1800 900 604800 86400"},
			want: "~ . SOA",
		},
		{
			name: "ZONEMD hash algorithm change",
			change: diff.Change{Kind: diff.ChangeModified, Name: ".", Type: "ZONEMD", OldTTL: 86400, NewTTL: 86400,
				OldRData: "2026072501 1 241 ABCDEF", NewRData: "2026072502 1 242 FEDCBA"},
			want: "~ . ZONEMD",
		},
		{
			name:   "ZONEMD removed",
			change: diff.Change{Kind: diff.ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072501 1 241 ABCDEF"},
			want:   "- . ZONEMD",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 再署名ノイズに埋もれさせても通知されること。
			changes := append(resigningChanges(1400), tt.change)
			parts := FormatPosts(changes, tweetOpts())
			if len(parts) == 0 {
				t.Fatal("FormatPosts() = nil, want the zone change reported")
			}
			joined := strings.Join(parts, "\n")
			if !strings.Contains(joined, "[zone]") {
				t.Errorf("missing [zone] heading in:\n%s", joined)
			}
			if !strings.Contains(joined, tt.want) {
				t.Errorf("missing %q in:\n%s", tt.want, joined)
			}
		})
	}
}

func TestFormatPostsSinglePartOverview(t *testing.T) {
	changes := append(resigningChanges(3),
		diff.Change{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "NS", NewRData: "ns1.newgtld."},
		diff.Change{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "DS", NewRData: "12345 8 2 ABCDEF"},
	)
	parts := FormatPosts(changes, tweetOpts())
	if len(parts) != 1 {
		t.Fatalf("got %d parts, want 1:\n%s", len(parts), strings.Join(parts, "\n---\n"))
	}
	msg := parts[0]
	for _, want := range []string{
		postTitle,
		"serial 2026072501 -> 2026072502",
		"delegation 1 / DNSSEC 1",
		"re-signing: 6 RRSIG (omitted)",
		"[delegation]",
		"+ newgtld. NS ns1.newgtld.",
		"[DNSSEC]",
		"+ newgtld. DS 12345 8 2 ABCDEF",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
	// 単一パーツでは番号を付けない。
	if strings.Contains(msg, "(1/1)") {
		t.Errorf("single part should not be numbered:\n%s", msg)
	}
}

func TestFormatPostsRecordLineFormats(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeRemoved, Name: "oldgtld.", Type: "NS", OldRData: "ns1.oldgtld."},
		{Kind: diff.ChangeModified, Name: "moved.", Type: "NS", OldRData: "a.example.", NewRData: "b.example."},
		{Kind: diff.ChangeModified, Name: "ttlonly.", Type: "NS", OldTTL: 86400, NewTTL: 172800, OldRData: "ns1.ttlonly.", NewRData: "ns1.ttlonly."},
	}
	msg := strings.Join(FormatPosts(changes, tweetOpts()), "\n")
	for _, want := range []string{
		"- oldgtld. NS ns1.oldgtld.",
		"~ moved. NS a.example. -> b.example.",
		"~ ttlonly. NS ttl 86400 -> 172800",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
	// SOA 変更がなければ serial 行は出さない。
	if strings.Contains(msg, "serial") {
		t.Errorf("unexpected serial line in:\n%s", msg)
	}
	// signature 変更がなければ re-signing 行は出さない。
	if strings.Contains(msg, "re-signing") {
		t.Errorf("unexpected re-signing line in:\n%s", msg)
	}
}

func TestFormatPostsSplitsAndNumbers(t *testing.T) {
	// 1パーツに収まらない件数のレコード単位明細。
	var changes []diff.Change
	for i := 0; i < 8; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("example%d.", i), Type: "NS",
			NewRData: fmt.Sprintf("ns1.example%d.net.", i),
		})
	}
	parts := FormatPosts(changes, tweetOpts())
	if len(parts) < 2 {
		t.Fatalf("got %d parts, want >= 2:\n%s", len(parts), strings.Join(parts, "\n---\n"))
	}
	for i, p := range parts {
		if n := utf8.RuneCountInString(p); n > 280 {
			t.Errorf("part %d is %d runes (> 280):\n%s", i+1, n, p)
		}
		want := fmt.Sprintf("%s (%d/%d)", postTitle, i+1, len(parts))
		if !strings.HasPrefix(p, want) {
			t.Errorf("part %d should start with %q, got:\n%s", i+1, want, p)
		}
	}
	// 全変更が投稿に含まれること。
	joined := strings.Join(parts, "\n")
	for i := 0; i < 8; i++ {
		if !strings.Contains(joined, fmt.Sprintf("example%d. NS", i)) {
			t.Errorf("missing example%d. in:\n%s", i, joined)
		}
	}
	if strings.Contains(joined, "more changes") {
		t.Errorf("nothing should be dropped:\n%s", joined)
	}
}

func TestFormatPostsRepeatsHeadingAcrossParts(t *testing.T) {
	var changes []diff.Change
	for i := 0; i < 10; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("example%02d.", i), Type: "NS",
			NewRData: fmt.Sprintf("ns1.somewhat-long-nameserver-name%02d.net.", i),
		})
	}
	parts := FormatPosts(changes, tweetOpts())
	if len(parts) < 2 {
		t.Fatalf("got %d parts, want >= 2", len(parts))
	}
	for i, p := range parts {
		if !strings.Contains(p, "[delegation]") {
			t.Errorf("part %d should repeat the category heading:\n%s", i+1, p)
		}
	}
}

func TestFormatPostsFallsBackToTLDAggregation(t *testing.T) {
	// レコード単位では MaxParts に収まらないが、TLD 集約なら収まる件数。
	var changes []diff.Change
	for i := 0; i < 12; i++ {
		name := fmt.Sprintf("tld%02d.", i)
		changes = append(changes,
			diff.Change{Kind: diff.ChangeAdded, Name: name, Type: "NS", NewRData: "ns1.new-registry-nameserver.example.net."},
			diff.Change{Kind: diff.ChangeRemoved, Name: name, Type: "NS", OldRData: "ns1.old-registry-nameserver.example.net."},
		)
	}
	parts := FormatPosts(changes, FormatOptions{MaxLen: 280, MaxParts: 4, Numbering: true})
	joined := strings.Join(parts, "\n")
	if len(parts) > 4 {
		t.Fatalf("got %d parts, want <= 4", len(parts))
	}
	if !strings.Contains(joined, "tld00. NS +1 -1") {
		t.Errorf("expected TLD-aggregated line in:\n%s", joined)
	}
	if strings.Contains(joined, "ns1.new-registry-nameserver") {
		t.Errorf("aggregated mode should not contain rdata:\n%s", joined)
	}
	if strings.Contains(joined, "more changes") {
		t.Errorf("aggregation should fit without truncation:\n%s", joined)
	}
	for i, p := range parts {
		if n := utf8.RuneCountInString(p); n > 280 {
			t.Errorf("part %d is %d runes (> 280)", i+1, n)
		}
	}
}

func TestFormatPostsTruncatesBeyondMaxParts(t *testing.T) {
	// TLD 集約でも収まらない大量の実質的変更。
	var changes []diff.Change
	for i := 0; i < 300; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("tld%03d.", i), Type: "NS",
			NewRData: "ns1.example.net.",
		})
	}
	parts := FormatPosts(changes, FormatOptions{MaxLen: 280, MaxParts: 2, Numbering: true})
	if len(parts) != 2 {
		t.Fatalf("got %d parts, want 2:\n%s", len(parts), strings.Join(parts, "\n---\n"))
	}
	last := parts[len(parts)-1]
	if !strings.Contains(last, "more changes") {
		t.Errorf("last part should state the dropped count:\n%s", last)
	}
	for i, p := range parts {
		if n := utf8.RuneCountInString(p); n > 280 {
			t.Errorf("part %d is %d runes (> 280):\n%s", i+1, n, p)
		}
	}
	// 落とした件数が、載せた行数と整合すること (合計 300 件)。
	var reported int
	if _, err := fmt.Sscanf(last[strings.Index(last, "... +"):], "... +%d more changes", &reported); err != nil {
		t.Fatalf("parse dropped count from %q: %v", last, err)
	}
	shown := strings.Count(strings.Join(parts, "\n"), " NS +1")
	if reported+shown != 300 {
		t.Errorf("dropped %d + shown %d != 300", reported, shown)
	}
}

func TestFormatPostsHandlesMultibyteRData(t *testing.T) {
	long := strings.Repeat("あ", 120)
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "xn--multibyte.", Type: "TXT", NewRData: long},
	}
	parts := FormatPosts(changes, tweetOpts())
	if len(parts) == 0 {
		t.Fatal("want at least 1 part")
	}
	for i, p := range parts {
		if n := utf8.RuneCountInString(p); n > 280 {
			t.Errorf("part %d is %d runes (> 280)", i+1, n)
		}
		if !utf8.ValidString(p) {
			t.Errorf("part %d is not valid UTF-8", i+1)
		}
	}
}

func TestFormatPostsSlackOptions(t *testing.T) {
	// Slack は上限が緩いのでレコード単位のまま 1 メッセージに収まる。
	var changes []diff.Change
	for i := 0; i < 30; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("tld%02d.", i), Type: "NS",
			NewRData: "ns1.example.net.",
		})
	}
	parts := FormatPosts(changes, FormatOptions{MaxLen: slackMaxLen, MaxParts: slackMaxParts, Numbering: true})
	if len(parts) != 1 {
		t.Fatalf("got %d parts, want 1", len(parts))
	}
	if !strings.Contains(parts[0], "+ tld29. NS ns1.example.net.") {
		t.Errorf("expected record-level detail in:\n%s", parts[0])
	}
}

func TestFormatPostsZeroMaxLen(t *testing.T) {
	changes := []diff.Change{{Kind: diff.ChangeAdded, Name: "a.", Type: "NS", NewRData: "ns1.a."}}
	if got := FormatPosts(changes, FormatOptions{}); got != nil {
		t.Errorf("MaxLen 0 should produce no posts, got %v", got)
	}
}

func TestTruncate(t *testing.T) {
	short := "hello"
	if truncate(short, 60) != short {
		t.Errorf("truncate(%q, 60) = %q", short, truncate(short, 60))
	}

	long := strings.Repeat("a", 100)
	result := truncate(long, 60)
	if utf8.RuneCountInString(result) != 63 { // 60 + "..."
		t.Errorf("truncate(long, 60) rune len = %d, want 63", utf8.RuneCountInString(result))
	}
	if !strings.HasSuffix(result, "...") {
		t.Error("truncated string should end with ...")
	}

	// マルチバイト文字は rune 単位で切り詰め、UTF-8 を壊さない。
	multibyte := strings.Repeat("あ", 100)
	result = truncate(multibyte, 60)
	if utf8.RuneCountInString(result) != 63 {
		t.Errorf("truncate(multibyte, 60) rune len = %d, want 63", utf8.RuneCountInString(result))
	}
	if !utf8.ValidString(result) {
		t.Error("truncated multibyte string should stay valid UTF-8")
	}
}

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
	return FormatOptions{MaxLen: 280, MaxParts: 4, Numbering: true, Weighted: true}
}

// compactTweetOpts は X 向けの圧縮フォーマットオプション。serial / re-signing 行を省く。
func compactTweetOpts() FormatOptions {
	opts := tweetOpts()
	opts.CompactOverview = true
	return opts
}

func TestFormatPostsCompactOverviewOmitsSerialAndResigning(t *testing.T) {
	changes := append(resigningChanges(3),
		diff.Change{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "NS", NewRData: "ns1.newgtld."},
		diff.Change{Kind: diff.ChangeAdded, Name: "newgtld.", Type: "DS", NewRData: "12345 8 2 ABCDEF"},
	)
	parts := FormatPosts(changes, compactTweetOpts())
	if len(parts) != 1 {
		t.Fatalf("got %d parts, want 1:\n%s", len(parts), strings.Join(parts, "\n---\n"))
	}
	msg := parts[0]
	for _, want := range []string{
		postTitle,
		"delegation 1 / DNSSEC 1",
		"[delegation]",
		"+ newgtld. NS ns1.newgtld.",
		"[DNSSEC]",
		"+ newgtld. DS 12345 8 2 ABCDEF",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("missing %q in:\n%s", want, msg)
		}
	}
	for _, dontWant := range []string{"serial", "re-signing"} {
		if strings.Contains(msg, dontWant) {
			t.Errorf("compact overview should not contain %q line:\n%s", dontWant, msg)
		}
	}
}

func TestFormatPostsCompactOverviewAnchorsOmitsSerial(t *testing.T) {
	// root anchors 通知でも compact モードは serial 行を出さない
	// (anchor に serial は付かないが、行ロジックを共通化した結果を確認する)。
	opts := compactTweetOpts()
	opts.Title = "DNS Root Anchors changes"
	opts.RDataMaxLen = anchorRDataMaxLen
	parts := FormatPosts([]diff.Change{
		{Kind: diff.ChangeAdded, Name: "19036", Type: "DS", NewRData: "19036 13 2 AABBCCDD"},
	}, opts)
	if len(parts) != 1 {
		t.Fatalf("got %d parts, want 1:\n%s", len(parts), strings.Join(parts, "\n---\n"))
	}
	if strings.Contains(parts[0], "serial") {
		t.Errorf("anchor compact overview should not contain serial line:\n%s", parts[0])
	}
	if !strings.Contains(parts[0], "[DNSSEC]") {
		t.Errorf("missing [DNSSEC] section:\n%s", parts[0])
	}
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
			// serial 更新と TTL 変更が同時に起きた場合、機械的変更ではない理由
			// (TTL が変わったこと) が明細から分かること。
			name: "SOA serial and TTL change",
			change: diff.Change{Kind: diff.ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 172800,
				OldRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072501 1800 900 604800 86400",
				NewRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072502 1800 900 604800 86400"},
			want: "(ttl 86400 -> 172800)",
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
	changes = append(changes, diff.Change{
		Kind: diff.ChangeModified, Name: "both.", Type: "NS", OldTTL: 86400, NewTTL: 172800,
		OldRData: "a.both.", NewRData: "b.both.",
	})
	msg := strings.Join(FormatPosts(changes, tweetOpts()), "\n")
	for _, want := range []string{
		"- oldgtld. NS ns1.oldgtld.",
		"~ moved. NS a.example. -> b.example.",
		"~ ttlonly. NS ttl 86400 -> 172800",
		// RDATA と TTL が同時に変わった場合は両方出す。
		"~ both. NS a.both. -> b.both. (ttl 86400 -> 172800)",
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

// TestFormatPostsCompactOverviewSplitsAndRepeatsHeading は compact モードでも
// 分割と heading 再掲が正しく動くこと (X 実投稿の (1/3) (2/3) (3/3) パターンを担保) を確認する。
// 8/3 の実投稿を模した DNSSEC 6 件 + signature 1 件 + 追加 DNSSEC 4 件の構成で、
// 少なくとも 2 パート以上に分かれ、各パートに [DNSSEC] または [signature] が
// 再掲されることを検証する。
func TestFormatPostsCompactOverviewSplitsAndRepeatsHeading(t *testing.T) {
	changes := []diff.Change{
		// 1パート目に押し込む DNSSEC (6件) と signature (1件)
		{Kind: diff.ChangeAdded, Name: "al.", Type: "DS", NewRData: "46645 13 2 A4DD7495AEA086F4602ACF3932BADDAABBCCDD"},
		{Kind: diff.ChangeModified, Name: "al.", Type: "NSEC",
			OldRData: "alibaba. NS RRSIG NSEC", NewRData: "alibaba. NS DS RRSIG NSEC"},
		{Kind: diff.ChangeAdded, Name: "al.", Type: "RRSIG",
			NewRData: "DS 8 1 86400 20260817050000 20260804040000 57780 . dummy"},
		// 2パート目に押し込む DNSSEC (4件)
		{Kind: diff.ChangeAdded, Name: "alsace.", Type: "DS", NewRData: "6291 13 2 49DDA30E62091F08C72B6DD7A3C99EAABBCCDD"},
		{Kind: diff.ChangeAdded, Name: "mma.", Type: "DS", NewRData: "24045 13 2 CDB760636E956607DF832CE56C98CAABBCCDD"},
		{Kind: diff.ChangeAdded, Name: "museum.", Type: "DS", NewRData: "23046 13 2 C19F3E83B491C5690EF879DD0C1E4AABBCCDD"},
		{Kind: diff.ChangeRemoved, Name: "total.", Type: "DS", OldRData: "2568 13 2 9AC9895253012C60A05E701FE237F2AABBCCDD"},
	}
	parts := FormatPosts(changes, compactTweetOpts())
	if len(parts) < 2 {
		t.Fatalf("got %d parts, want >= 2:\n%s", len(parts), strings.Join(parts, "\n---\n"))
	}
	joined := strings.Join(parts, "\n----\n")
	// どのパートにも serial / re-signing 行は出ない。
	if strings.Contains(joined, "serial") {
		t.Errorf("compact overview must not contain serial line:\n%s", joined)
	}
	if strings.Contains(joined, "re-signing") {
		t.Errorf("compact overview must not contain re-signing line:\n%s", joined)
	}
	// 1件目・2件目・3件目 … すべてに postTitle + (i/n) の番号が付く。
	for i, p := range parts {
		want := fmt.Sprintf("%s (%d/%d)", postTitle, i+1, len(parts))
		if !strings.HasPrefix(p, want) {
			t.Errorf("part %d should start with %q, got:\n%s", i+1, want, p)
		}
		if n := utf8.RuneCountInString(p); n > 280 {
			t.Errorf("part %d is %d runes (> 280):\n%s", i+1, n, p)
		}
	}
	// 分割後の各パートに [DNSSEC] または [signature] の heading が少なくとも1つある。
	for i, p := range parts {
		if !strings.Contains(p, "[DNSSEC]") && !strings.Contains(p, "[signature]") {
			t.Errorf("part %d missing category heading:\n%s", i+1, p)
		}
	}
	// 全レコードの明細が (どこかの) パートに含まれている。
	for _, want := range []string{
		"al. DS",
		"al. NSEC",
		"al. RRSIG",
		"alsace. DS",
		"mma. DS",
		"museum. DS",
		"total. DS",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in any part:\n%s", want, joined)
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

func TestShortRData(t *testing.T) {
	tests := []struct {
		rrType string
		in     string
		want   string
	}{
		// DS 系は DNSKEY を特定する key tag / algorithm / digest type まで残す。
		{"DS", "46645 13 2 A4DD7495AEA086F4602ACF3932BADDAABBC", "46645 13 2 ..."},
		{"CDS", "46645 13 2 A4DD7495AEA0", "46645 13 2 ..."},
		{"DNSKEY", "257 3 13 mdsswUyr3DPW", "257 3 13 ..."},
		{"CDNSKEY", "257 3 13 mdsswUyr3DPW", "257 3 13 ..."},
		// RRSIG は type covered / algorithm / labels / original TTL まで残す。
		{"RRSIG", "DS 8 1 86400 20260806050000 20260724040000 57780 . sig", "DS 8 1 86400 ..."},
		// それ以外は先頭フィールドのみ。
		{"NS", "ns1.dns.nic.gone.", "ns1.dns.nic.gone."},
		{"A", "192.0.2.1", "192.0.2.1"},
		{"AAAA", "2001:db8::1", "2001:db8::1"},
		{"TXT", "v=spf1 include:example.net -all", "v=spf1 ..."},
		{"UNKNOWN", "foo bar baz", "foo ..."},
		// フィールド数が上限以下なら省略記号を付けない。
		{"DS", "46645 13 2", "46645 13 2"},
		{"NS", "", ""},
	}
	for _, tt := range tests {
		if got := shortRData(tt.in, tt.rrType); got != tt.want {
			t.Errorf("shortRData(%q, %q) = %q, want %q", tt.in, tt.rrType, got, tt.want)
		}
	}
}

func TestFormatPostsFallsBackToShortRData(t *testing.T) {
	// レコード単位のフル RDATA (40字) では MaxParts に収まらないが、
	// DS の短縮 RDATA (key tag / algorithm / digest type) なら収まる件数。
	var changes []diff.Change
	for i := 0; i < 20; i++ {
		changes = append(changes, diff.Change{
			Kind:     diff.ChangeAdded,
			Name:     fmt.Sprintf("tld%02d.", i),
			Type:     "DS",
			NewRData: fmt.Sprintf("%05d 13 2 %s", 10000+i, strings.Repeat("A", 50)),
		})
	}
	opts := FormatOptions{MaxLen: 280, MaxParts: 4, Numbering: true, Weighted: true, CompactOverview: true}
	parts := FormatPosts(changes, opts)
	if len(parts) == 0 || len(parts) > opts.MaxParts {
		t.Fatalf("got %d parts, want 1..%d:\n%s", len(parts), opts.MaxParts, strings.Join(parts, "\n---\n"))
	}
	joined := strings.Join(parts, "\n")
	if !strings.Contains(joined, "+ tld00. DS 10000 13 2 ...") {
		t.Errorf("expected short RDATA record line in:\n%s", joined)
	}
	// 集約 (TLD ごとの +1) にはフォールバックしない。
	if strings.Contains(joined, "DS +1") {
		t.Errorf("should not aggregate when short RDATA fits:\n%s", joined)
	}
	if strings.Contains(joined, "more changes") {
		t.Errorf("nothing should be dropped:\n%s", joined)
	}
	for i := 0; i < 20; i++ {
		if !strings.Contains(joined, fmt.Sprintf("tld%02d. DS", i)) {
			t.Errorf("missing tld%02d. in:\n%s", i, joined)
		}
	}
	for i, p := range parts {
		if n := weightedLen(p); n > opts.MaxLen {
			t.Errorf("part %d is %d weighted chars (> %d):\n%s", i+1, n, opts.MaxLen, p)
		}
	}
}

func TestFormatChangeShortKeepsChangeKinds(t *testing.T) {
	// 短縮しても追加/削除/変更の別は明細行に残る。
	tests := []struct {
		name   string
		change diff.Change
		want   string
	}{
		{
			name:   "added",
			change: diff.Change{Kind: diff.ChangeAdded, Name: "new.", Type: "DS", NewRData: "11111 8 2 " + strings.Repeat("A", 50)},
			want:   "  + new. DS 11111 8 2 ...",
		},
		{
			name:   "removed",
			change: diff.Change{Kind: diff.ChangeRemoved, Name: "gone.", Type: "DS", OldRData: "11111 8 2 " + strings.Repeat("A", 50)},
			want:   "  - gone. DS 11111 8 2 ...",
		},
		{
			name: "modified",
			change: diff.Change{Kind: diff.ChangeModified, Name: "roll.", Type: "DS",
				OldRData: "22222 8 2 " + strings.Repeat("A", 50), NewRData: "33333 13 2 " + strings.Repeat("B", 50)},
			want: "  ~ roll. DS 22222 8 2 ... -> 33333 13 2 ...",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatChangeShort(tt.change); got != tt.want {
				t.Errorf("formatChangeShort() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatPostsManyDSChangesKeepsRecordDetail(t *testing.T) {
	// 実投稿相当: DS 変更 38 件。TLD 集約 (tld00. DS +1) ではなく、
	// 短縮 RDATA のレコード単位明細のまま max_posts に収まること。
	var changes []diff.Change
	for i := 0; i < 38; i++ {
		changes = append(changes, diff.Change{
			Kind:     diff.ChangeAdded,
			Name:     fmt.Sprintf("tld%02d.", i),
			Type:     "DS",
			NewRData: fmt.Sprintf("%05d 13 2 %s", 10000+i, strings.Repeat("A", 60)),
		})
	}
	opts := FormatOptions{
		MaxLen: tweetMaxLen, MaxParts: defaultMaxPosts,
		Numbering: true, Weighted: true, CompactOverview: true,
	}
	parts := FormatPosts(changes, opts)
	if len(parts) == 0 || len(parts) > opts.MaxParts {
		t.Fatalf("got %d parts, want 1..%d:\n%s", len(parts), opts.MaxParts, strings.Join(parts, "\n---\n"))
	}
	joined := strings.Join(parts, "\n")
	if !strings.Contains(joined, "+ tld00. DS 10000 13 2 ...") {
		t.Errorf("expected short RDATA record detail in:\n%s", joined)
	}
	if strings.Contains(joined, "DS +1") {
		t.Errorf("should not aggregate such a case:\n%s", joined)
	}
	for i := 0; i < 38; i++ {
		if !strings.Contains(joined, fmt.Sprintf("tld%02d. DS", i)) {
			t.Errorf("missing tld%02d. in:\n%s", i, joined)
		}
	}
}

func TestFormatPostsAggregatesWhenShortRDataTooLong(t *testing.T) {
	// 短縮してもなお MaxParts に収まらない大量の DS 変更は TLD 集約に落ちる。
	var changes []diff.Change
	for i := 0; i < 100; i++ {
		changes = append(changes, diff.Change{
			Kind:     diff.ChangeAdded,
			Name:     fmt.Sprintf("tld%03d.", i),
			Type:     "DS",
			NewRData: fmt.Sprintf("%05d 13 2 %s", 10000+i, strings.Repeat("A", 50)),
		})
	}
	opts := FormatOptions{MaxLen: 280, MaxParts: 3, Numbering: true, Weighted: true, CompactOverview: true}
	parts := FormatPosts(changes, opts)
	if len(parts) != 3 {
		t.Fatalf("got %d parts, want 3:\n%s", len(parts), strings.Join(parts, "\n---\n"))
	}
	joined := strings.Join(parts, "\n")
	if !strings.Contains(joined, "DS +1") {
		t.Errorf("expected TLD aggregation line in:\n%s", joined)
	}
}

// longName は 1行で上限を超える長さの owner name を返す。
func longName(tld string) string {
	return "ns1." + strings.Repeat("a", 260) + "." + tld
}

func TestFormatPostsFallsBackWhenARecordLineIsTooLong(t *testing.T) {
	// レコード単位では 1行が上限を超えて落とされてしまうため、TLD 集約に切り替える。
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: longName("jp."), Type: "A", NewRData: "192.0.2.1"},
	}
	parts := FormatPosts(changes, tweetOpts())
	if len(parts) == 0 {
		t.Fatal("FormatPosts() = nil, want the change reported")
	}
	joined := strings.Join(parts, "\n")
	if strings.Contains(joined, "more changes") {
		t.Errorf("should fall back to aggregation instead of dropping:\n%s", joined)
	}
	if !strings.Contains(joined, "jp. A +1") {
		t.Errorf("expected the TLD-aggregated line in:\n%s", joined)
	}
	for i, p := range parts {
		if n := weightedLen(p); n > 280 {
			t.Errorf("part %d is %d weighted chars (> 280):\n%s", i+1, n, p)
		}
	}
}

func TestFormatPostsKeepsLimitWhenNoteMustBeAdded(t *testing.T) {
	// 集約しても 1行が上限を超える (TLD 自体が長い) ケース。落とした件数を伝えつつ
	// どのパーツも上限を超えないこと。
	huge := strings.Repeat("a", 300) + "."
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: huge, Type: "NS", NewRData: "ns1.example.net."},
	}
	for _, maxParts := range []int{0, 1, 4} {
		opts := FormatOptions{MaxLen: 280, MaxParts: maxParts, Numbering: true, Weighted: true}
		parts := FormatPosts(changes, opts)
		if len(parts) == 0 {
			t.Fatalf("maxParts=%d: FormatPosts() = nil", maxParts)
		}
		joined := strings.Join(parts, "\n")
		if !strings.Contains(joined, "more changes") {
			t.Errorf("maxParts=%d: dropped changes should be reported:\n%s", maxParts, joined)
		}
		for i, p := range parts {
			if n := weightedLen(p); n > opts.MaxLen {
				t.Errorf("maxParts=%d: part %d is %d weighted chars (> %d):\n%s",
					maxParts, i+1, n, opts.MaxLen, p)
			}
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
		if n := weightedLen(p); n > 280 {
			t.Errorf("part %d is %d weighted chars (> 280):\n%s", i+1, n, p)
		}
		if !utf8.ValidString(p) {
			t.Errorf("part %d is not valid UTF-8", i+1)
		}
	}
}

func TestFormatPostsWeightedSplitsEarlierThanRuneCount(t *testing.T) {
	// 全角のみの明細。rune 数では 1パーツに収まるが、X の重み付きでは2文字分に
	// なるため分割されなければならない。
	var changes []diff.Change
	for i := 0; i < 4; i++ {
		changes = append(changes, diff.Change{
			Kind: diff.ChangeAdded, Name: fmt.Sprintf("xn--example%02d.", i), Type: "TXT",
			NewRData: strings.Repeat("あ", 30),
		})
	}

	runeParts := FormatPosts(changes, FormatOptions{MaxLen: 280, MaxParts: 4, Numbering: true})
	weightedParts := FormatPosts(changes, tweetOpts())

	if len(weightedParts) <= len(runeParts) {
		t.Errorf("weighted counting should split more: rune=%d parts, weighted=%d parts",
			len(runeParts), len(weightedParts))
	}
	for i, p := range weightedParts {
		if n := weightedLen(p); n > 280 {
			t.Errorf("weighted part %d is %d weighted chars (> 280):\n%s", i+1, n, p)
		}
	}
	// rune 数ベースだと X の上限を超えるパーツができることを確認する (退行検知)。
	over := false
	for _, p := range runeParts {
		if weightedLen(p) > 280 {
			over = true
		}
	}
	if !over {
		t.Skip("test data no longer exceeds the weighted limit under rune counting")
	}
}

func TestWeightedLen(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"あ", 2},          // CJK は重み2
		{"あい", 4},         // 全角2文字
		{"‐", 1},          // ハイフン (重み1の範囲)
		{"①", 2},          // ① (重み1の範囲外)
		{"\U0001F600", 2}, // 絵文字 (BMP 外)
		{"a\U0001F600あ", 1 + 2 + 2},
		// 自動リンクされるドメイン/URL は t.co の固定長として数える。
		{"http://a.io", urlWeight},
		{"example.net", urlWeight},
		// FQDN の末尾ドットは URL の外なので別に1文字加算される。
		{"a.co.", urlWeight + 1},
		{"ns1.newgtld.", urlWeight + 1},
		{"newgtld. NS ns1.newgtld.", 8 + 1 + 2 + 1 + urlWeight + 1},
		// 23文字を超える場合は実際の長さ (過小評価しない方向に倒す)。
		{"ns-tld1.charlestonroadregistry.com.", 34 + 1},
		// TLD にならないラベルで終わるものはリンクされない。
		{"192.0.2.1", 9},
		{"newgtld.", 8},
	}
	for _, tt := range tests {
		if got := weightedLen(tt.in); got != tt.want {
			t.Errorf("weightedLen(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestIsAutoLinked(t *testing.T) {
	linked := []string{
		"example.net", "ns1.example.net.", "http://example.com/x", "https://a.io",
		"ns1.xn--p1ai.", "a.root-servers.net.", "ns1.dns.nic.zip.",
	}
	for _, s := range linked {
		if !isAutoLinked(s) {
			t.Errorf("isAutoLinked(%q) = false, want true", s)
		}
	}
	notLinked := []string{
		// 単一ラベル (TLD 自体) は URL にならない。
		"", "newgtld.", "xn--p1ai.", "NS", "192.0.2.1", "2001:db8::1", "12345", "8", "ttl",
		"20260806050000", "A1B2C3D4E5F6",
	}
	for _, s := range notLinked {
		if isAutoLinked(s) {
			t.Errorf("isAutoLinked(%q) = true, want false", s)
		}
	}
}

func TestWeightedLenCountsWhitespace(t *testing.T) {
	// 改行や連続空白も1文字として数えること。
	if got, want := weightedLen("ab\ncd"), 5; got != want {
		t.Errorf("weightedLen(%q) = %d, want %d", "ab\ncd", got, want)
	}
	if got, want := weightedLen("ab  cd"), 6; got != want {
		t.Errorf("weightedLen(%q) = %d, want %d", "ab  cd", got, want)
	}
	if got, want := weightedLen("  ab"), 4; got != want {
		t.Errorf("weightedLen(%q) = %d, want %d", "  ab", got, want)
	}
}

func TestFormatPostsUnweightedUsesRuneCount(t *testing.T) {
	// Slack は rune 数で数える (X の重み付けは適用しない)。
	long := strings.Repeat("あ", 1000) // 1000 rune / 2000 重み
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "xn--multibyte.", Type: "TXT", NewRData: long},
	}
	parts := FormatPosts(changes, FormatOptions{MaxLen: 3500, MaxParts: 3})
	if len(parts) != 1 {
		t.Fatalf("got %d parts, want 1 (3500 runes is enough)", len(parts))
	}
	if n := utf8.RuneCountInString(parts[0]); n > 3500 {
		t.Errorf("part is %d runes (> 3500)", n)
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

func TestFormatPostsCustomTitleAndRDataLen(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "Kmyv6jo", Type: "DS", NewRData: "38696 8 2 683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16"},
	}
	posts := FormatPosts(changes, FormatOptions{
		Title:       "DNS Root Anchors changes",
		RDataMaxLen: 120,
		MaxLen:      280,
	})
	if len(posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(posts))
	}
	if !strings.HasPrefix(posts[0], "DNS Root Anchors changes") {
		t.Errorf("post title = %q, want custom title", posts[0])
	}
	if !strings.Contains(posts[0], "683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16") {
		t.Errorf("full digest must not be truncated with RDataMaxLen=120:\n%s", posts[0])
	}
}

func TestFormatPostsDefaultTitle(t *testing.T) {
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "example.", Type: "NS", NewRData: "a.nic.example."},
	}
	posts := FormatPosts(changes, FormatOptions{MaxLen: 280})
	if len(posts) != 1 || !strings.HasPrefix(posts[0], "DNS Root Zone changes") {
		t.Fatalf("default title expected, got %v", posts)
	}
}

func TestFormatPostsDefaultRDataTruncation(t *testing.T) {
	long := "38696 8 2 683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16"
	changes := []diff.Change{
		{Kind: diff.ChangeAdded, Name: "Kmyv6jo", Type: "DS", NewRData: long},
	}
	posts := FormatPosts(changes, FormatOptions{MaxLen: 280})
	if len(posts) != 1 {
		t.Fatalf("posts = %d, want 1", len(posts))
	}
	if strings.Contains(posts[0], long) {
		t.Errorf("long rdata must be truncated by default (RDataMaxLen=0):\n%s", posts[0])
	}
}

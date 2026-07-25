package notify

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/zone"
)

const (
	// postTitle は各パーツの先頭行。連続投稿でも単体で文脈が分かるよう全パーツに付ける。
	postTitle = "DNS Root Zone changes"
	// rdataMaxLen はレコード単位の明細行に載せる RDATA の最大文字数。
	rdataMaxLen = 40
	// moreReserve は打ち切り時の "... +N more changes" 行のために予約する文字数。
	moreReserve = 24
	// numberingReserve は " (i/n)" の初期予約幅 (総パーツ数 99 まで)。
	numberingReserve = len(" (99/99)")
)

// FormatOptions は投稿本文の生成オプション。
type FormatOptions struct {
	MaxLen    int  // 1パーツの最大文字数 (rune 数)
	MaxParts  int  // 最大パーツ数 (0 = 無制限)
	Numbering bool // タイトル行に " (i/n)" を付ける (総パーツ数が 2 以上のときのみ)
}

// detailLine は明細の1行と、その行が表す変更件数。
// 打ち切り時に「あと何件落ちたか」を数えるため件数を持つ。
type detailLine struct {
	text  string
	count int
}

// block は見出しと明細行のまとまり。パーツをまたぐ場合、見出しは続きのパーツに再掲する。
type block struct {
	heading string // "[delegation]" (概要ブロックは空)
	lines   []detailLine
}

// FormatPosts は変更内容を MaxLen 以内のパーツ列にフォーマットする。
// 実質的な変更 (再署名に伴う機械的変更を除いたもの) が無い場合は nil を返す。
//
// 明細はまずレコード単位で組み、MaxParts に収まらない場合は TLD ごとの集約に切り替える。
// それでも収まらない場合は MaxParts で打ち切り、末尾に落とした件数を明記する。
func FormatPosts(changes []diff.Change, opts FormatOptions) []string {
	if opts.MaxLen <= 0 {
		return nil
	}
	sub := diff.Substantive(changes)
	if len(sub) == 0 {
		return nil
	}

	overview := overviewBlock(changes, sub)

	for _, detail := range [][]block{recordBlocks(sub), aggregatedBlocks(sub)} {
		parts, _ := pack(append([]block{overview}, detail...), opts, 0)
		if opts.MaxParts <= 0 || len(parts) <= opts.MaxParts {
			return parts
		}
	}

	// 集約でも MaxParts に収まらない場合は打ち切る。
	parts, _ := pack(append([]block{overview}, aggregatedBlocks(sub)...), opts, opts.MaxParts)
	return parts
}

// overviewBlock は先頭パーツに置く概要ブロックを組み立てる。
func overviewBlock(all, sub []diff.Change) block {
	var lines []detailLine

	if old, new, ok := soaSerials(all); ok {
		lines = append(lines, detailLine{text: fmt.Sprintf("serial %s -> %s", old, new)})
	}

	counts := diff.CountByCategory(sub)
	var catParts []string
	for _, cat := range diff.Categories() {
		if n := counts[cat]; n > 0 {
			catParts = append(catParts, fmt.Sprintf("%s %d", cat, n))
		}
	}
	lines = append(lines, detailLine{text: strings.Join(catParts, " / ")})

	// 通知から省いた再署名ノイズの規模だけは伝える。
	if n := diff.CountMechanical(all, "RRSIG"); n > 0 {
		lines = append(lines, detailLine{text: fmt.Sprintf("re-signing: %d RRSIG (omitted)", n)})
	}

	lines = append(lines, detailLine{text: ""})
	return block{lines: lines}
}

// soaSerials は SOA の変更から新旧の serial を取り出す。
func soaSerials(changes []diff.Change) (oldSerial, newSerial string, ok bool) {
	for _, c := range changes {
		if c.Type != "SOA" || c.Kind != diff.ChangeModified {
			continue
		}
		o, oOK := zone.SerialFromRData(c.OldRData)
		n, nOK := zone.SerialFromRData(c.NewRData)
		if oOK && nOK {
			return o, n, true
		}
	}
	return "", "", false
}

// recordBlocks はレコード単位の明細ブロックをカテゴリごとに組み立てる。
func recordBlocks(sub []diff.Change) []block {
	grouped := diff.CategorizeChanges(sub)
	var blocks []block
	for _, cat := range diff.Categories() {
		catChanges := grouped[cat]
		if len(catChanges) == 0 {
			continue
		}
		lines := make([]detailLine, 0, len(catChanges))
		for _, c := range catChanges {
			lines = append(lines, detailLine{text: formatChange(c), count: 1})
		}
		blocks = append(blocks, block{heading: heading(cat), lines: lines})
	}
	return blocks
}

// aggregatedBlocks は TLD ごとに集約した明細ブロックをカテゴリごとに組み立てる。
func aggregatedBlocks(sub []diff.Change) []block {
	grouped := diff.CategorizeChanges(sub)
	var blocks []block
	for _, cat := range diff.Categories() {
		catChanges := grouped[cat]
		if len(catChanges) == 0 {
			continue
		}
		groups := diff.SummarizeByTLD(catChanges)
		lines := make([]detailLine, 0, len(groups))
		for _, g := range groups {
			lines = append(lines, detailLine{text: formatTLDGroup(g), count: g.Total})
		}
		blocks = append(blocks, block{heading: heading(cat), lines: lines})
	}
	return blocks
}

func heading(cat diff.Category) string {
	return "[" + cat.String() + "]"
}

// formatChange はレコード単位の明細行を組み立てる。
func formatChange(c diff.Change) string {
	switch c.Kind {
	case diff.ChangeAdded:
		return fmt.Sprintf("  + %s %s %s", c.Name, c.Type, truncate(c.NewRData, rdataMaxLen))
	case diff.ChangeRemoved:
		return fmt.Sprintf("  - %s %s %s", c.Name, c.Type, truncate(c.OldRData, rdataMaxLen))
	case diff.ChangeModified:
		if c.OldRData == c.NewRData {
			// RDATA が同じ modified は TTL 変更。
			return fmt.Sprintf("  ~ %s %s ttl %d -> %d", c.Name, c.Type, c.OldTTL, c.NewTTL)
		}
		return fmt.Sprintf("  ~ %s %s %s -> %s", c.Name, c.Type,
			truncate(c.OldRData, rdataMaxLen), truncate(c.NewRData, rdataMaxLen))
	default:
		return ""
	}
}

// formatTLDGroup は TLD 集約の明細行を組み立てる ("  example. NS +1 -1 ~1")。
func formatTLDGroup(g diff.TLDGroup) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "  %s", g.TLD)
	for _, tc := range g.Counts {
		fmt.Fprintf(&sb, " %s", tc.Type)
		if tc.Added > 0 {
			fmt.Fprintf(&sb, " +%d", tc.Added)
		}
		if tc.Removed > 0 {
			fmt.Fprintf(&sb, " -%d", tc.Removed)
		}
		if tc.Modified > 0 {
			fmt.Fprintf(&sb, " ~%d", tc.Modified)
		}
	}
	return sb.String()
}

// pack はブロック群を MaxLen 以内のパーツに詰める。
// maxParts > 0 の場合はパーツ数をそこで打ち切り、落とした変更件数を返す。
func pack(blocks []block, opts FormatOptions, maxParts int) (parts []string, dropped int) {
	packed, dropped := packOnce(blocks, opts.MaxLen, maxParts)

	numbered := opts.Numbering && len(packed) > 1
	if numbered {
		// 番号の分だけ上限が減るため詰め直す。桁数が予約幅を超えたら予約を広げる (通常1周で確定)。
		reserve := numberingReserve
		for i := 0; i < 3; i++ {
			packed, dropped = packOnce(blocks, opts.MaxLen-reserve, maxParts)
			need := utf8.RuneCountInString(fmt.Sprintf(" (%d/%d)", len(packed), len(packed)))
			if need <= reserve {
				break
			}
			reserve = need
		}
	}

	parts = make([]string, 0, len(packed))
	for i, lines := range packed {
		if numbered {
			lines[0] = fmt.Sprintf("%s (%d/%d)", lines[0], i+1, len(packed))
		}
		parts = append(parts, strings.Join(lines, "\n"))
	}
	return parts, dropped
}

// packOnce は行を limit 文字以内のパーツに貪欲に詰める。
func packOnce(blocks []block, limit, maxParts int) ([][]string, int) {
	var packed [][]string
	cur := []string{postTitle}
	curLen := utf8.RuneCountInString(postTitle)
	dropped := 0
	full := false

	flush := func() {
		packed = append(packed, cur)
		cur = []string{postTitle}
		curLen = utf8.RuneCountInString(postTitle)
	}
	// 最後に使えるパーツでは打ち切り行の分を予約する。
	effLimit := func() int {
		if maxParts > 0 && len(packed) == maxParts-1 {
			return limit - moreReserve
		}
		return limit
	}
	add := func(text string) {
		cur = append(cur, text)
		curLen += 1 + utf8.RuneCountInString(text)
	}

	for _, b := range blocks {
		pendingHeading := b.heading != ""
		for _, dl := range b.lines {
			if full {
				dropped += dl.count
				continue
			}
			need := func() int {
				n := 1 + utf8.RuneCountInString(dl.text)
				if pendingHeading {
					n += 1 + utf8.RuneCountInString(b.heading)
				}
				return n
			}
			if curLen+need() > effLimit() {
				if maxParts > 0 && len(packed)+1 >= maxParts {
					full = true
					dropped += dl.count
					continue
				}
				flush()
				pendingHeading = b.heading != ""
				if curLen+need() > effLimit() {
					// 1行だけで上限を超える場合は諦める (行は十分短いため通常は起きない)。
					dropped += dl.count
					continue
				}
			}
			if pendingHeading {
				add(b.heading)
				pendingHeading = false
			}
			add(dl.text)
		}
	}
	if len(cur) > 1 {
		flush()
	}

	if dropped > 0 {
		if len(packed) == 0 {
			packed = append(packed, []string{postTitle})
		}
		last := len(packed) - 1
		packed[last] = append(packed[last], fmt.Sprintf("... +%d more changes", dropped))
	}
	return packed, dropped
}

func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	return string([]rune(s)[:maxLen]) + "..."
}

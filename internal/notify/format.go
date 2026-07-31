package notify

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/zone"
)

const (
	// postTitle は各パーツの先頭行。連続投稿でも単体で文脈が分かるよう全パーツに付ける。
	postTitle = "DNS Root Zone changes"
	// anchorPostTitle は root anchors 変更投稿のタイトル。
	anchorPostTitle = "DNS Root Anchors changes"
	// anchorRDataMaxLen は anchors 投稿で DS ダイジェスト全体 (64文字) と退役日が
	// 見えるようにする RDATA 上限。
	anchorRDataMaxLen = 120
	// rdataMaxLen はレコード単位の明細行に載せる RDATA の最大文字数。
	rdataMaxLen = 40
	// moreReserve は打ち切り時の "... +N more changes" 行のために予約する文字数。
	moreReserve = 24
	// numberingReserve は " (i/n)" の初期予約幅 (総パーツ数 99 まで)。
	numberingReserve = len(" (99/99)")
	// urlWeight は X が URL を t.co に短縮した後の固定長 (twitter-text の
	// transformedURLLength)。短い URL もこの長さとして数えられる。
	urlWeight = 23
)

// FormatOptions は投稿本文の生成オプション。
type FormatOptions struct {
	MaxLen    int  // 1パーツの最大文字数
	MaxParts  int  // 最大パーツ数 (0 = 無制限)
	Numbering bool // タイトル行に " (i/n)" を付ける (総パーツ数が 2 以上のときのみ)
	// Weighted は X (twitter-text) の重み付き文字数で長さを数える。
	// X は大半の非 ASCII 文字を2文字分として数えるため、X 向けには必須。
	// false の場合は rune 数で数える。
	Weighted bool
	// Title は各パーツ先頭行のタイトル。空なら "DNS Root Zone changes"。
	// root anchors 通知では "DNS Root Anchors changes" を指定する。
	Title string
	// RDataMaxLen はレコード単位の明細行に載せる RDATA の最大文字数。0 なら 40。
	// root anchors の DS ダイジェスト (64文字 + 退役日) は 40 では切れるため、
	// anchors 通知では 120 を指定する。
	RDataMaxLen int
}

// title はパーツ先頭行のタイトルを返す。
func (o FormatOptions) title() string {
	if o.Title != "" {
		return o.Title
	}
	return postTitle
}

// rdataMaxLen は明細行の RDATA 上限を返す。
func (o FormatOptions) rdataMaxLen() int {
	if o.RDataMaxLen > 0 {
		return o.RDataMaxLen
	}
	return rdataMaxLen
}

// measure は MaxLen と比較する長さの計算関数を返す。
func (o FormatOptions) measure() func(string) int {
	if o.Weighted {
		return weightedLen
	}
	return utf8.RuneCountInString
}

// weightedLen は X (twitter-text) の重み付き文字数を返す。
// twitter-text の既定設定では下記の範囲が重み1、それ以外は重み2。
// また URL は t.co の固定長 (urlWeight) に置き換えて数えられるため、
// 自動リンクされるトークンは urlWeight を下回らないものとして数える。
//
// 結合文字列 (絵文字の ZWJ シーケンスなど) や未正規化の文字列、実際には
// 自動リンクされないトークンは X の計数より多めに数える。過小評価して
// 上限超過で投稿が拒否される方向には倒れない。
func weightedLen(s string) int {
	n := 0
	for i, token := range strings.Fields(s) {
		if i > 0 {
			n++ // トークン間の区切り
		}
		n += tokenWeight(token)
	}
	// strings.Fields は連続する空白をまとめるため、区切り文字の差分を補正する。
	if extra := countSpace(s) - separatorCount(s); extra > 0 {
		n += extra
	}
	return n
}

// tokenWeight は1トークンの重み付き文字数を返す。
// 自動リンクされる部分は t.co の固定長 (urlWeight) を下回らないものとして数え、
// URL に含まれない末尾の句読点 ("example.net." の末尾ドットなど) は別に加算する。
func tokenWeight(token string) int {
	url, trailing, ok := splitURLToken(token)
	if !ok {
		return runeWeightedLen(token)
	}
	w := runeWeightedLen(url)
	if w < urlWeight {
		w = urlWeight
	}
	return w + runeWeightedLen(trailing)
}

// countSpace は空白文字の数を返す。
func countSpace(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			n++
		}
	}
	return n
}

// separatorCount は strings.Fields で数えられる区切りの個数を返す。
func separatorCount(s string) int {
	if n := len(strings.Fields(s)); n > 0 {
		return n - 1
	}
	return 0
}

// runeWeightedLen は URL 換算を行わない重み付き文字数を返す。
func runeWeightedLen(s string) int {
	n := 0
	for _, r := range s {
		if isLightweightRune(r) {
			n++
		} else {
			n += 2
		}
	}
	return n
}

// autoLinkedRe は twitter-text が自動リンクするトークンの近似。
// scheme 付き URL と、TLD らしいラベル (英字のみ、または xn--) で終わるホスト名にマッチする。
// root zone の RDATA に現れるドメイン名は実在の TLD で終わるため、この近似で十分。
var autoLinkedRe = regexp.MustCompile(
	`^(?:[A-Za-z][A-Za-z0-9+.\-]*://)?[A-Za-z0-9][A-Za-z0-9\-]*(?:\.[A-Za-z0-9][A-Za-z0-9\-]*)*\.(?:xn--[A-Za-z0-9\-]+|[A-Za-z]{2,})(?:[:/?#][^\s]*)?$`)

// urlTrailingCutset は URL に含まれない末尾の句読点。
const urlTrailingCutset = ".,;:)]}"

// splitURLToken はトークンを、自動リンクされる URL 部分と URL 外の末尾文字に分ける。
// 自動リンクされない場合は ok=false。
func splitURLToken(token string) (url, trailing string, ok bool) {
	url = strings.TrimRight(token, urlTrailingCutset)
	if !autoLinkedRe.MatchString(url) {
		return "", "", false
	}
	return url, token[len(url):], true
}

// isAutoLinked はトークンに X で自動リンクされる部分があるかを返す。
func isAutoLinked(token string) bool {
	_, _, ok := splitURLToken(token)
	return ok
}

// isLightweightRune は twitter-text で重み1と定義された範囲かを返す。
func isLightweightRune(r rune) bool {
	switch {
	case r <= 0x10FF,
		r >= 0x2000 && r <= 0x200D,
		r >= 0x2010 && r <= 0x201F,
		r >= 0x2032 && r <= 0x2037:
		return true
	default:
		return false
	}
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
// 明細はまずレコード単位で組み、MaxParts に収まらない場合や 1行が長すぎて明細を
// 落とす場合は TLD ごとの集約に切り替える。それでも収まらない場合は MaxParts で
// 打ち切り、末尾に落とした件数を明記する。
func FormatPosts(changes []diff.Change, opts FormatOptions) []string {
	if opts.MaxLen <= 0 {
		return nil
	}
	sub := diff.Substantive(changes)
	if len(sub) == 0 {
		return nil
	}

	overview := overviewBlock(changes, sub)

	for _, detail := range [][]block{recordBlocks(sub, opts), aggregatedBlocks(sub, opts)} {
		parts, dropped := pack(append([]block{overview}, detail...), opts, 0)
		// 1件も落とさずに MaxParts に収まった候補だけを採用する。
		if dropped == 0 && (opts.MaxParts <= 0 || len(parts) <= opts.MaxParts) {
			return parts
		}
	}

	// 集約でも収まらない場合は打ち切る。
	parts, _ := pack(append([]block{overview}, aggregatedBlocks(sub, opts)...), opts, opts.MaxParts)
	return parts
}

// anchorFormatOptions は root anchors 通知共通のフォーマットオプション。
// zone 通知との違い: タイトルが anchors 用になり、DS ダイジェスト全体が
// 見えるよう RDATA 上限が広がる。
func anchorFormatOptions(base FormatOptions) FormatOptions {
	base.Title = anchorPostTitle
	base.RDataMaxLen = anchorRDataMaxLen
	return base
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
func recordBlocks(sub []diff.Change, opts FormatOptions) []block {
	grouped := diff.CategorizeChanges(sub)
	var blocks []block
	for _, cat := range diff.Categories() {
		catChanges := grouped[cat]
		if len(catChanges) == 0 {
			continue
		}
		lines := make([]detailLine, 0, len(catChanges))
		for _, c := range catChanges {
			lines = append(lines, detailLine{text: formatChange(c, opts.rdataMaxLen()), count: 1})
		}
		blocks = append(blocks, block{heading: heading(cat), lines: lines})
	}
	return blocks
}

// aggregatedBlocks は TLD ごとに集約した明細ブロックをカテゴリごとに組み立てる。
func aggregatedBlocks(sub []diff.Change, opts FormatOptions) []block {
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
// rdataMaxLen は RDATA の表示上限 (truncate する文字数)。
func formatChange(c diff.Change, rdataMaxLen int) string {
	switch c.Kind {
	case diff.ChangeAdded:
		return fmt.Sprintf("  + %s %s %s", c.Name, c.Type, truncate(c.NewRData, rdataMaxLen))
	case diff.ChangeRemoved:
		return fmt.Sprintf("  - %s %s %s", c.Name, c.Type, truncate(c.OldRData, rdataMaxLen))
	case diff.ChangeModified:
		ttl := ""
		if c.OldTTL != c.NewTTL {
			ttl = fmt.Sprintf("ttl %d -> %d", c.OldTTL, c.NewTTL)
		}
		if c.OldRData == c.NewRData {
			// RDATA が同じ modified は TTL 変更のみ。
			return fmt.Sprintf("  ~ %s %s %s", c.Name, c.Type, ttl)
		}
		line := fmt.Sprintf("  ~ %s %s %s -> %s", c.Name, c.Type,
			truncate(c.OldRData, rdataMaxLen), truncate(c.NewRData, rdataMaxLen))
		if ttl != "" {
			// RDATA と TTL が同時に変わった場合は両方伝える。
			line += " (" + ttl + ")"
		}
		return line
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
	measure := opts.measure()
	packed, dropped := packOnce(blocks, opts.MaxLen, maxParts, measure, opts.title())

	numbered := opts.Numbering && len(packed) > 1
	if numbered {
		// 番号の分だけ上限が減るため詰め直す。桁数が予約幅を超えたら予約を広げる (通常1周で確定)。
		reserve := numberingReserve
		for i := 0; i < 3; i++ {
			packed, dropped = packOnce(blocks, opts.MaxLen-reserve, maxParts, measure, opts.title())
			need := measure(fmt.Sprintf(" (%d/%d)", len(packed), len(packed)))
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
// 長さは measure で数える (X は重み付き文字数、それ以外は rune 数)。
// title は各パーツの先頭行 (番号付け時は " (i/n)" が追記される)。
func packOnce(blocks []block, limit, maxParts int, measure func(string) int, title string) ([][]string, int) {
	var packed [][]string
	var packedLen []int // packed 各パーツの長さ (打ち切り行を追記できるか判定するため)
	cur := []string{title}
	curLen := measure(title)
	dropped := 0
	full := false

	flush := func() {
		packed = append(packed, cur)
		packedLen = append(packedLen, curLen)
		cur = []string{title}
		curLen = measure(title)
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
		curLen += 1 + measure(text)
	}

	for _, b := range blocks {
		pendingHeading := b.heading != ""
		for _, dl := range b.lines {
			if full {
				dropped += dl.count
				continue
			}
			need := func() int {
				n := 1 + measure(dl.text)
				if pendingHeading {
					n += 1 + measure(b.heading)
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
		note := fmt.Sprintf("... +%d more changes", dropped)
		// 末尾パーツに追記して上限を超えるなら別パーツにする。
		// maxParts 指定時は最後のパーツで moreReserve を予約しているのでここには来ない。
		last := len(packed) - 1
		if last >= 0 && packedLen[last]+1+measure(note) <= limit {
			packed[last] = append(packed[last], note)
			packedLen[last] += 1 + measure(note)
		} else {
			packed = append(packed, []string{title, note})
			packedLen = append(packedLen, measure(title)+1+measure(note))
		}
	}
	return packed, dropped
}

func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	return string([]rune(s)[:maxLen]) + "..."
}

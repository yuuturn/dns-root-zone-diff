package notify

import (
	"fmt"
	"strings"

	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/whois"
)

const (
	// whoisFieldWidth は WHOIS 由来の明細を "domain:       HELSINKI" の形に
	// 揃えるための、キー (コロン込み) の表示幅。
	whoisFieldWidth = 14
	// whoisEllipsis は長すぎるリプライを切り詰めるときの末尾。
	whoisEllipsis = "..."
)

// whoisReply は1つの TLD に対する WHOIS リプライ本文。
type whoisReply struct {
	tld  string
	text string
}

// FormatWhoisReply は TLD の WHOIS 情報を、追加・削除通知へのリプライ本文に
// 整形する。手動投稿で使われていた次の形式に合わせ、最低限のフィールドだけを載せる:
//
//	% whois helsinki.
//	...
//	domain:       HELSINKI
//	status:       FORMER
//	remarks:      Registration information:
//	created:      2015-12-10
//	changed:      2026-10-28
//	source:       IANA
//
// 空のフィールドは行ごと省略する。
func FormatWhoisReply(tld string, info whois.Info) string {
	lines := []string{"% whois " + tld + ".", whoisEllipsis}
	add := func(key, value string) {
		if value == "" {
			return
		}
		lines = append(lines, fmt.Sprintf("%-*s%s", whoisFieldWidth, key+":", value))
	}
	add("domain", info.Domain)
	add("status", info.Status)
	add("remarks", info.Remarks)
	add("created", info.Created)
	add("changed", info.Changed)
	add("source", info.Source)
	return strings.Join(lines, "\n")
}

// buildWhoisReplies は追加・削除された TLD のうち WHOIS 情報があるものについて
// リプライを組み立てる。順序は changes の順 (呼び出し側でソート済み)。
func buildWhoisReplies(changes []diff.Change, infos map[string]whois.Info) []whoisReply {
	if len(infos) == 0 {
		return nil
	}
	seen := make(map[string]bool)
	var replies []whoisReply
	for _, c := range changes {
		if c.Type != diff.TypeTLD {
			continue
		}
		if c.Kind != diff.ChangeAdded && c.Kind != diff.ChangeRemoved {
			continue
		}
		tld := strings.TrimSuffix(c.Name, ".")
		if seen[tld] {
			continue
		}
		info, ok := infos[tld]
		if !ok {
			continue
		}
		seen[tld] = true
		replies = append(replies, whoisReply{tld: tld, text: FormatWhoisReply(tld, info)})
	}
	return replies
}

// fitWeighted は文字列を X の重み付き文字数で limit 以内に収める。
// 切り詰めた場合は末尾に "..." を付ける。
func fitWeighted(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if weightedLen(s) <= limit {
		return s
	}
	if limit <= len(whoisEllipsis) {
		return whoisEllipsis[:limit]
	}

	runes := []rune(s)
	lo, hi := 0, len(runes)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if weightedLen(string(runes[:mid])+whoisEllipsis) <= limit {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(runes[:lo]) + whoisEllipsis
}

// fitRunes は文字列を rune 数で limit 以内に収める (BlueSky 用)。
// 切り詰めた場合は末尾に "..." を付ける。
func fitRunes(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	if limit <= len(whoisEllipsis) {
		return string(runes[:limit])
	}
	return string(runes[:limit-len(whoisEllipsis)]) + whoisEllipsis
}

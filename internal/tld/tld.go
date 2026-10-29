// Package tld は IANA の TLD リストを扱う。
// https://data.iana.org/TLD/tlds-alpha-by-domain.txt
package tld

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/zone"
)

// versionRe は先頭の "# Version <number>, Last Updated <text>" 行にマッチする。
var versionRe = regexp.MustCompile(`^#\s*Version\s+([0-9]+),`)

// labelRe は DNS のラベル (1〜63文字の英数字とハイフン、先頭末尾はハイフン以外)。
// TLD リストは ASCII のみだが、HTML エラーページなどを TLD として取り込まない
// よう厳密に検証する。
var labelRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// List は TLD リスト全体。
type List struct {
	Version string   // ヘッダの Version (例 "2026102900")。無ければ空
	TLDs    []string // 小文字・昇順・重複なしの TLD ラベル (例 "com")
}

// Parse は tlds-alpha-by-domain.txt のバイト列をパースする。
//
// 1行目は "# Version <number>, Last Updated ..." のコメント行で、以降は
// 1行1 TLD のラベルが大文字で並ぶ。空行とその他のコメント行は無視する。
// TLD は DNS 名として大文字小文字を区別しないため小文字に正規化し、
// 並び順に依存しないようソートする。
func Parse(data []byte) (List, error) {
	var list List
	seen := make(map[string]bool)

	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if list.Version == "" {
				if m := versionRe.FindStringSubmatch(line); m != nil {
					list.Version = m[1]
				}
			}
			continue
		}

		label := strings.ToLower(line)
		if !labelRe.MatchString(label) {
			return List{}, fmt.Errorf("parse tld list: line %d: invalid TLD %q", i+1, line)
		}
		if seen[label] {
			continue
		}
		seen[label] = true
		list.TLDs = append(list.TLDs, label)
	}

	if len(list.TLDs) == 0 {
		return List{}, fmt.Errorf("parse tld list: no TLDs found")
	}
	sort.Strings(list.TLDs)
	return list, nil
}

// ToRecords は List を diff エンジン用の擬似ゾーンレコードに変換する。
// 各 TLD は擬似 RR type "TLD" の1レコードとして表現する:
//
//	Name = "<tld>." (FQDN 形式)
//	Type = diff.TypeTLD
//	RData = "" (TLD リストにレコードデータは無い)
func ToRecords(list List) []zone.Record {
	recs := make([]zone.Record, 0, len(list.TLDs))
	for _, label := range list.TLDs {
		recs = append(recs, zone.Record{
			Name:  label + ".",
			Class: "IN",
			Type:  diff.TypeTLD,
		})
	}
	return recs
}

// Diff は新旧の TLD 集合を比較して変更点を返す。
// バージョンだけが進み TLD 集合が同じ場合は差分なしとなる。
func Diff(oldList, newList List) []diff.Change {
	return diff.Diff(ToRecords(oldList), ToRecords(newList))
}

// Serial は履歴エントリの serial 相当として使う TLD リストの Version を返す。
// Version が無い場合は "unknown" を返す。
func Serial(list List) string {
	if list.Version == "" {
		return "unknown"
	}
	return list.Version
}

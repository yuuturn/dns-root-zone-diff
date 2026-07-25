package diff

import (
	"sort"
	"strings"
)

// TypeCount は1つの RR type に対する種別ごとの変更件数。
type TypeCount struct {
	Type     string
	Added    int
	Removed  int
	Modified int
}

// Total はこの RR type の変更件数の合計。
func (tc TypeCount) Total() int {
	return tc.Added + tc.Removed + tc.Modified
}

// TLDGroup は1つの TLD にまとめた変更の集約。
type TLDGroup struct {
	TLD    string      // "example." (ゾーン apex は ".")
	Counts []TypeCount // RR type 昇順
	Total  int
}

// TLDOf はレコード名から TLD ラベルを取り出す。
// "ns1.example." -> "example."、"example." -> "example."、"." -> "."
func TLDOf(name string) string {
	trimmed := strings.TrimSuffix(name, ".")
	if trimmed == "" {
		return "."
	}
	if i := strings.LastIndex(trimmed, "."); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	return trimmed + "."
}

// SummarizeByTLD は変更群を TLD ごとに集約する。
// 並び順は件数の降順 → TLD 昇順で決定的。
func SummarizeByTLD(changes []Change) []TLDGroup {
	byTLD := make(map[string]map[string]*TypeCount)
	for _, c := range changes {
		tld := TLDOf(c.Name)
		byType, ok := byTLD[tld]
		if !ok {
			byType = make(map[string]*TypeCount)
			byTLD[tld] = byType
		}
		tc, ok := byType[c.Type]
		if !ok {
			tc = &TypeCount{Type: c.Type}
			byType[c.Type] = tc
		}
		switch c.Kind {
		case ChangeAdded:
			tc.Added++
		case ChangeRemoved:
			tc.Removed++
		case ChangeModified:
			tc.Modified++
		}
	}

	groups := make([]TLDGroup, 0, len(byTLD))
	for tld, byType := range byTLD {
		g := TLDGroup{TLD: tld, Counts: make([]TypeCount, 0, len(byType))}
		for _, tc := range byType {
			g.Counts = append(g.Counts, *tc)
			g.Total += tc.Total()
		}
		sort.Slice(g.Counts, func(i, j int) bool {
			return g.Counts[i].Type < g.Counts[j].Type
		})
		groups = append(groups, g)
	}

	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Total != groups[j].Total {
			return groups[i].Total > groups[j].Total
		}
		return groups[i].TLD < groups[j].TLD
	})
	return groups
}

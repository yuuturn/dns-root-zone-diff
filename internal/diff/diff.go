package diff

import (
	"sort"
	"strings"

	"github.com/yfujii/dns-root-diff/internal/zone"
)

// ChangeKind は変更の種類。
type ChangeKind int

const (
	ChangeAdded    ChangeKind = iota // レコード追加
	ChangeRemoved                    // レコード削除
	ChangeModified                   // レコード変更
)

func (k ChangeKind) String() string {
	switch k {
	case ChangeAdded:
		return "added"
	case ChangeRemoved:
		return "removed"
	case ChangeModified:
		return "modified"
	default:
		return "unknown"
	}
}

// Category は変更の分類。
type Category int

const (
	CategoryDelegation Category = iota // 移譲変更 (NS)
	CategoryDNSSEC                     // DNSSEC 変更 (DS, DNSKEY, NSEC, NSEC3PARAM)
	CategoryGlue                       // ネームサーバーのアドレス変更 (A, AAAA)
	CategoryOther                      // その他
	CategorySignature                  // 再署名 (RRSIG)
	CategoryZone                       // ゾーン管理 (SOA, ZONEMD)
)

func (c Category) String() string {
	switch c {
	case CategoryDelegation:
		return "delegation"
	case CategoryDNSSEC:
		return "DNSSEC"
	case CategoryGlue:
		return "glue"
	case CategoryOther:
		return "other"
	case CategorySignature:
		return "signature"
	case CategoryZone:
		return "zone"
	default:
		return "unknown"
	}
}

// categoryOrder はカテゴリの表示順。
var categoryOrder = []Category{
	CategoryDelegation,
	CategoryDNSSEC,
	CategoryGlue,
	CategoryZone,
	CategorySignature,
	CategoryOther,
}

// Categories は全カテゴリを表示順で返す。
func Categories() []Category {
	return append([]Category(nil), categoryOrder...)
}

// Change は1つの変更を表す。
type Change struct {
	Kind     ChangeKind
	Name     string
	Type     string
	OldTTL   uint32
	NewTTL   uint32
	OldRData string
	NewRData string
}

// nameType は (Name, Type) の組。
type nameType struct {
	Name string
	Type string
}

// Diff は新旧のレコード群を比較し、変更点を返す。
// レコードは (Name, Type, RData) の組で識別する。
func Diff(oldRecords, newRecords []zone.Record) []Change {
	// (Name, Type) ごとにグループ化
	oldByNT := make(map[nameType][]zone.Record)
	for _, r := range oldRecords {
		nt := nameType{r.Name, r.Type}
		oldByNT[nt] = append(oldByNT[nt], r)
	}

	newByNT := make(map[nameType][]zone.Record)
	for _, r := range newRecords {
		nt := nameType{r.Name, r.Type}
		newByNT[nt] = append(newByNT[nt], r)
	}

	// 全 (Name, Type) キーを収集
	allNT := make(map[nameType]bool)
	for nt := range oldByNT {
		allNT[nt] = true
	}
	for nt := range newByNT {
		allNT[nt] = true
	}

	var changes []Change

	for nt := range allNT {
		oldGroup := oldByNT[nt]
		newGroup := newByNT[nt]

		// RData ごとにインデックス
		oldByRData := make(map[string]zone.Record, len(oldGroup))
		for _, r := range oldGroup {
			oldByRData[r.RData] = r
		}
		newByRData := make(map[string]zone.Record, len(newGroup))
		for _, r := range newGroup {
			newByRData[r.RData] = r
		}

		// 共通 RData: TTL 変更の検出
		for rdata, oldR := range oldByRData {
			if newR, ok := newByRData[rdata]; ok {
				if oldR.TTL != newR.TTL {
					changes = append(changes, Change{
						Kind:     ChangeModified,
						Name:     oldR.Name,
						Type:     oldR.Type,
						OldTTL:   oldR.TTL,
						NewTTL:   newR.TTL,
						OldRData: oldR.RData,
						NewRData: newR.RData,
					})
				}
			}
		}

		// 削除・追加 RData の収集
		var removedRData, addedRData []string
		for rdata := range oldByRData {
			if _, ok := newByRData[rdata]; !ok {
				removedRData = append(removedRData, rdata)
			}
		}
		for rdata := range newByRData {
			if _, ok := oldByRData[rdata]; !ok {
				addedRData = append(addedRData, rdata)
			}
		}

		// 旧新ともに1レコードのみで RData が異なる場合は「変更」とみなす
		if len(oldGroup) == 1 && len(newGroup) == 1 && len(removedRData) == 1 && len(addedRData) == 1 {
			oldR := oldByRData[removedRData[0]]
			newR := newByRData[addedRData[0]]
			changes = append(changes, Change{
				Kind:     ChangeModified,
				Name:     oldR.Name,
				Type:     oldR.Type,
				OldTTL:   oldR.TTL,
				NewTTL:   newR.TTL,
				OldRData: oldR.RData,
				NewRData: newR.RData,
			})
		} else {
			// 複数レコードの場合は個別に削除・追加として報告
			for _, rdata := range removedRData {
				oldR := oldByRData[rdata]
				changes = append(changes, Change{
					Kind:     ChangeRemoved,
					Name:     oldR.Name,
					Type:     oldR.Type,
					OldTTL:   oldR.TTL,
					OldRData: oldR.RData,
				})
			}
			for _, rdata := range addedRData {
				newR := newByRData[rdata]
				changes = append(changes, Change{
					Kind:     ChangeAdded,
					Name:     newR.Name,
					Type:     newR.Type,
					NewTTL:   newR.TTL,
					NewRData: newR.RData,
				})
			}
		}
	}

	return changes
}

// SortChanges は変更群を (Name, Type, Kind, OldRData, NewRData) の順で安定ソートする。
// Diff の出力は map の反復順に依存し非決定的なため、通知や保存の前に呼ぶ。
func SortChanges(changes []Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		a, b := changes[i], changes[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Type != b.Type {
			return a.Type < b.Type
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.OldRData != b.OldRData {
			return a.OldRData < b.OldRData
		}
		return a.NewRData < b.NewRData
	})
}

// Categorize は変更をカテゴリに分類する。
func Categorize(c Change) Category {
	switch c.Type {
	case "NS":
		return CategoryDelegation
	case "DS", "DNSKEY", "NSEC", "NSEC3PARAM":
		return CategoryDNSSEC
	case "A", "AAAA":
		return CategoryGlue
	case "RRSIG":
		return CategorySignature
	case "SOA", "ZONEMD":
		return CategoryZone
	default:
		return CategoryOther
	}
}

// CategorizeChanges は変更群をカテゴリごとにグループ化する。
func CategorizeChanges(changes []Change) map[Category][]Change {
	grouped := make(map[Category][]Change)
	for _, c := range changes {
		cat := Categorize(c)
		grouped[cat] = append(grouped[cat], c)
	}
	return grouped
}

// CountByCategory はカテゴリごとの変更件数を返す。
func CountByCategory(changes []Change) map[Category]int {
	counts := make(map[Category]int)
	for _, c := range changes {
		counts[Categorize(c)]++
	}
	return counts
}

// IsMechanical は再署名に伴う機械的な変更かを返す。
// root zone は 12 時間ごとに再署名され、その際 RRSIG が全て入れ替わり、
// SOA serial と ZONEMD の serial/digest も必ず更新される。これらは運用上の
// 意味を持たないため通知の対象外とする。
//
// 同じ RR type でも、SOA の MNAME/RNAME/refresh/retry/expire/minimum や
// ZONEMD の scheme/hash algorithm の変更、TTL の変更、レコードの追加・削除は
// 機械的な変更ではないため false を返す。
func IsMechanical(c Change) bool {
	switch c.Type {
	case "RRSIG":
		return true
	case "SOA":
		// SOA RDATA: MNAME RNAME SERIAL REFRESH RETRY EXPIRE MINIMUM
		return isRDataUpdateOnly(c, 7, 2)
	case "ZONEMD":
		// ZONEMD RDATA: SERIAL SCHEME HASH-ALGORITHM DIGEST
		return isRDataUpdateOnly(c, 4, 0, 3)
	default:
		return false
	}
}

// isRDataUpdateOnly は TTL が変わらない modified で、RDATA の差分が
// updatable のフィールドだけに収まっているかを返す。
func isRDataUpdateOnly(c Change, wantFields int, updatable ...int) bool {
	if c.Kind != ChangeModified || c.OldTTL != c.NewTTL {
		return false
	}
	oldFields := strings.Fields(c.OldRData)
	newFields := strings.Fields(c.NewRData)
	if len(oldFields) != wantFields || len(newFields) != wantFields {
		return false
	}
	skip := make(map[int]bool, len(updatable))
	for _, i := range updatable {
		skip[i] = true
	}
	for i := range oldFields {
		if skip[i] {
			continue
		}
		if oldFields[i] != newFields[i] {
			return false
		}
	}
	return true
}

// Substantive は実質的な変更 (再署名に伴う機械的変更を除いたもの) のみを抽出する。
func Substantive(changes []Change) []Change {
	var out []Change
	for _, c := range changes {
		if !IsMechanical(c) {
			out = append(out, c)
		}
	}
	return out
}

// CountMechanical は機械的変更のうち、指定 RR type の件数を返す。
func CountMechanical(changes []Change, rrType string) int {
	n := 0
	for _, c := range changes {
		if c.Type == rrType && IsMechanical(c) {
			n++
		}
	}
	return n
}

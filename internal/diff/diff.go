package diff

import (
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
	CategoryDNSSEC                     // DNSSEC 変更 (DS, DNSKEY, RRSIG)
	CategoryOther                      // その他
)

func (c Category) String() string {
	switch c {
	case CategoryDelegation:
		return "delegation"
	case CategoryDNSSEC:
		return "DNSSEC"
	case CategoryOther:
		return "other"
	default:
		return "unknown"
	}
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

// Categorize は変更をカテゴリに分類する。
func Categorize(c Change) Category {
	switch c.Type {
	case "NS":
		return CategoryDelegation
	case "DS", "DNSKEY", "RRSIG":
		return CategoryDNSSEC
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

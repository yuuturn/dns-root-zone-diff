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

// mechanicalSpec は再署名のたびに RDATA の一部が更新される RR type の定義。
// keyFields は再署名では変わらないフィールドで、RRset 内でレコードを識別する。
// keyFields と TTL が一致する旧新のレコードは、定常的な更新として扱う。
type mechanicalSpec struct {
	fields    int
	keyFields []int
}

var mechanicalSpecs = map[string]mechanicalSpec{
	// RRSIG: TYPE-COVERED ALGORITHM LABELS ORIGINAL-TTL EXPIRATION INCEPTION KEY-TAG SIGNER SIGNATURE
	// 再署名で expiration/inception/signature が入れ替わる。ZSK ロール時は key tag も
	// 変わるが、鍵の交代自体は apex の DNSKEY 変更として通知される。
	"RRSIG": {fields: 9, keyFields: []int{0, 1, 2, 3, 7}},
	// SOA: MNAME RNAME SERIAL REFRESH RETRY EXPIRE MINIMUM
	"SOA": {fields: 7, keyFields: []int{0, 1, 3, 4, 5, 6}},
	// ZONEMD: SERIAL SCHEME HASH-ALGORITHM DIGEST
	"ZONEMD": {fields: 4, keyFields: []int{1, 2}},
}

// recordKey は RRset 内のレコードを再署名をまたいで識別する鍵。
type recordKey struct {
	rrType string
	name   string
	ttl    uint32
	fields string // keyFields の値を連結したもの
}

// keyOf は RDATA から recordKey を作る。フィールド数が定義と異なる場合は失敗する。
func (s mechanicalSpec) keyOf(rrType, name, rdata string, ttl uint32) (recordKey, bool) {
	fields := strings.Fields(rdata)
	if len(fields) != s.fields {
		return recordKey{}, false
	}
	parts := make([]string, 0, len(s.keyFields))
	for _, i := range s.keyFields {
		parts = append(parts, fields[i])
	}
	return recordKey{rrType: rrType, name: name, ttl: ttl, fields: strings.Join(parts, "\x00")}, true
}

// sameKey は旧新の RDATA が keyFields で一致するかを返す。
func (s mechanicalSpec) sameKey(oldRData, newRData string) bool {
	oldFields := strings.Fields(oldRData)
	newFields := strings.Fields(newRData)
	if len(oldFields) != s.fields || len(newFields) != s.fields {
		return false
	}
	for _, i := range s.keyFields {
		if oldFields[i] != newFields[i] {
			return false
		}
	}
	return true
}

// MarkMechanical は各変更が再署名に伴う機械的な変更かを、changes と同じ長さの
// スライスで返す。
//
// root zone は 12 時間ごとに再署名され、その際 RRSIG が全て入れ替わり、
// SOA serial と ZONEMD の serial/digest も必ず更新される。これらは運用上の
// 意味を持たないため通知の対象外とする。
//
// 判定は RR type ではなく旧新の対応付けで行う。RRset が複数レコードを持つ場合、
// Diff は modified に畳めず removed+added に分解するため、mechanicalSpec の
// keyFields と TTL で組を作り、組になったものだけを機械的変更とする。
// 相手のいない追加・削除 (署名の欠落や digest algorithm のロールオーバー) や、
// TTL・keyFields が変わった変更は実質的な変更として残す。
func MarkMechanical(changes []Change) []bool {
	mechanical := make([]bool, len(changes))

	removed := make(map[recordKey][]int)
	added := make(map[recordKey][]int)

	for i, c := range changes {
		spec, ok := mechanicalSpecs[c.Type]
		if !ok {
			continue
		}
		switch c.Kind {
		case ChangeModified:
			if c.OldTTL == c.NewTTL && spec.sameKey(c.OldRData, c.NewRData) {
				mechanical[i] = true
			}
		case ChangeRemoved:
			if k, ok := spec.keyOf(c.Type, c.Name, c.OldRData, c.OldTTL); ok {
				removed[k] = append(removed[k], i)
			}
		case ChangeAdded:
			if k, ok := spec.keyOf(c.Type, c.Name, c.NewRData, c.NewTTL); ok {
				added[k] = append(added[k], i)
			}
		}
	}

	for k, rem := range removed {
		add := added[k]
		for j := 0; j < len(rem) && j < len(add); j++ {
			mechanical[rem[j]] = true
			mechanical[add[j]] = true
		}
	}

	return mechanical
}

// Substantive は実質的な変更 (再署名に伴う機械的変更を除いたもの) のみを抽出する。
func Substantive(changes []Change) []Change {
	mechanical := MarkMechanical(changes)
	var out []Change
	for i, c := range changes {
		if !mechanical[i] {
			out = append(out, c)
		}
	}
	return out
}

// CountMechanical は機械的変更のうち、指定 RR type の件数を返す。
func CountMechanical(changes []Change, rrType string) int {
	mechanical := MarkMechanical(changes)
	n := 0
	for i, c := range changes {
		if c.Type == rrType && mechanical[i] {
			n++
		}
	}
	return n
}

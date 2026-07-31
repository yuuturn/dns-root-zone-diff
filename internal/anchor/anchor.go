// Package anchor は IANA の root anchors (DNSSEC トラストアンカー) ファイルを扱う。
// https://data.iana.org/root-anchors/root-anchors.xml
package anchor

import (
	"encoding/xml"
	"fmt"

	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/zone"
)

// KeyDigest は <KeyDigest> 要素1つ分 (1つの KSK/DNSKEY に対応する DS 情報)。
type KeyDigest struct {
	ID         string // id 属性 (例 "Kmyv6jo")
	ValidFrom  string // validFrom 属性 (ISO8601、無ければ空)
	ValidUntil string // validUntil 属性 (退役済みキーのみ、無ければ空)
	KeyTag     int
	Algorithm  int
	DigestType int
	Digest     string
	PublicKey  string // 無ければ空
	Flags      int    // 無ければ 0
}

// TrustAnchors は root-anchors.xml 全体。
type TrustAnchors struct {
	ID      string // TrustAnchor の id 属性
	Source  string
	Zone    string
	Digests []KeyDigest
}

type keyDigestXML struct {
	ID         string `xml:"id,attr"`
	ValidFrom  string `xml:"validFrom,attr"`
	ValidUntil string `xml:"validUntil,attr"`
	KeyTag     int    `xml:"KeyTag"`
	Algorithm  int    `xml:"Algorithm"`
	DigestType int    `xml:"DigestType"`
	Digest     string `xml:"Digest"`
	PublicKey  string `xml:"PublicKey"`
	Flags      int    `xml:"Flags"`
}

type trustAnchorXML struct {
	ID        string         `xml:"id,attr"`
	Source    string         `xml:"source,attr"`
	Zone      string         `xml:"Zone"`
	KeyDigest []keyDigestXML `xml:"KeyDigest"`
}

// Parse は root-anchors.xml のバイト列をパースする。
func Parse(data []byte) (TrustAnchors, error) {
	var raw trustAnchorXML
	if err := xml.Unmarshal(data, &raw); err != nil {
		return TrustAnchors{}, fmt.Errorf("parse root anchors xml: %w", err)
	}
	ta := TrustAnchors{ID: raw.ID, Source: raw.Source, Zone: raw.Zone}
	for _, kd := range raw.KeyDigest {
		ta.Digests = append(ta.Digests, KeyDigest(kd))
	}
	return ta, nil
}

// ToRecords は TrustAnchors を diff エンジン用の擬似ゾーンレコードに変換する。
// 各 KeyDigest は DS レコードとして表現する:
//
//	Name = KeyDigest の id (例 "Kmyv6jo")
//	Type = "DS" (diff.Categorize で CategoryDNSSEC になる)
//	RData = "<KeyTag> <Algorithm> <DigestType> <Digest> [<validUntilの日付>]"
//
// 退役済みキー (validUntil あり) は RData 末尾に日付 (YYYY-MM-DD) を持つため、
// 退役の瞬間 (validUntil 付与) が modified として差分検出される。
func ToRecords(ta TrustAnchors) []zone.Record {
	recs := make([]zone.Record, 0, len(ta.Digests))
	for _, kd := range ta.Digests {
		rdata := fmt.Sprintf("%d %d %d %s", kd.KeyTag, kd.Algorithm, kd.DigestType, kd.Digest)
		if kd.ValidUntil != "" && len(kd.ValidUntil) >= 10 {
			rdata += " " + kd.ValidUntil[:10]
		}
		recs = append(recs, zone.Record{
			Name:  kd.ID,
			TTL:   0,
			Class: "IN",
			Type:  "DS",
			RData: rdata,
		})
	}
	return recs
}

// Diff は新旧の TrustAnchors の KeyDigest 集合を比較して変更点を返す。
func Diff(oldTA, newTA TrustAnchors) []diff.Change {
	return diff.Diff(ToRecords(oldTA), ToRecords(newTA))
}

// Serial は履歴エントリの serial 相当として使う TrustAnchor の id を返す。
// id が無い場合は "unknown" を返す。
func Serial(ta TrustAnchors) string {
	if ta.ID == "" {
		return "unknown"
	}
	return ta.ID
}

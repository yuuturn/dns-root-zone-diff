// Package anchor は IANA の root anchors (DNSSEC トラストアンカー) ファイルを扱う。
// https://data.iana.org/root-anchors/root-anchors.xml
package anchor

import (
	"encoding/xml"
	"fmt"
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

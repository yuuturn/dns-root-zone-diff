package anchor

import (
	"strings"
	"testing"
)

const sampleXML = `<?xml version="1.0" encoding="UTF-8"?>
<TrustAnchor id="0C05FDD6-422C-4910-8ED6-430ED15E11C2" source="http://data.iana.org/root-anchors/root-anchors.xml">
    <Zone>.</Zone>
    <KeyDigest id="Kjqmt7v" validFrom="2010-07-15T00:00:00+00:00" validUntil="2019-01-11T00:00:00+00:00">
        <KeyTag>19036</KeyTag>
        <Algorithm>8</Algorithm>
        <DigestType>2</DigestType>
        <Digest>49AAC11D7B6F6446702E54A1607371607A1A41855200FD2CE1CDDE32F24E8FB5</Digest>
    </KeyDigest>
    <KeyDigest id="Kmyv6jo" validFrom="2024-07-18T00:00:00+00:00">
        <KeyTag>38696</KeyTag>
        <Algorithm>8</Algorithm>
        <DigestType>2</DigestType>
        <Digest>683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16</Digest>
        <PublicKey>AwEAAa96jeuknZlaeSrvyAJj6ZHv28hhOKkx3rLGXVaC6rXTsDc449/cidltpkyGwCJNnOAlFNKF2jBosZBU5eeHspaQWOmOElZsjICMQMC3aeHbGiShvZsx4wMYSjH8e7Vrhbu6irwCzVBApESjbUdpWWmEnhathWu1jo+siFUiRAAxm9qyJNg/wOZqqzL/dL/q8PkcRU5oUKEpUge71M3ej2/7CPqpdVwuMoTvoB+ZOT4YeGyxMvHmbrxlFzGOHOijtzN+u1TQNatX2XBuzZNQ1K+s2CXkPIZo7s6JgZyvaBevYtxPvYLw4z9mR7K2vaF18UYH9Z9GNUUeayffKC73PYc=</PublicKey>
        <Flags>257</Flags>
    </KeyDigest>
</TrustAnchor>
`

func TestParse(t *testing.T) {
	ta, err := Parse([]byte(sampleXML))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if ta.ID != "0C05FDD6-422C-4910-8ED6-430ED15E11C2" {
		t.Errorf("ID = %q", ta.ID)
	}
	if len(ta.Digests) != 2 {
		t.Fatalf("Digests = %d, want 2", len(ta.Digests))
	}
	k0 := ta.Digests[0]
	if k0.ID != "Kjqmt7v" || k0.KeyTag != 19036 || k0.Algorithm != 8 ||
		k0.DigestType != 2 || k0.ValidUntil != "2019-01-11T00:00:00+00:00" {
		t.Errorf("digest[0] = %+v", k0)
	}
	k1 := ta.Digests[1]
	if k1.ID != "Kmyv6jo" || k1.ValidUntil != "" || k1.PublicKey == "" || k1.Flags != 257 {
		t.Errorf("digest[1] = %+v", k1)
	}
}

func TestParseInvalid(t *testing.T) {
	if _, err := Parse([]byte("<TrustAnchor><KeyDigest></TrustAnchor>")); err == nil {
		t.Error("Parse() = nil error, want error for malformed XML")
	}
	if _, err := Parse([]byte("not xml at all")); err == nil {
		t.Error("Parse() = nil error, want error for non-XML input")
	}
}

func TestToRecords(t *testing.T) {
	ta, err := Parse([]byte(sampleXML))
	if err != nil {
		t.Fatal(err)
	}
	recs := ToRecords(ta)
	if len(recs) != 2 {
		t.Fatalf("records = %d, want 2", len(recs))
	}
	r := recs[0]
	if r.Name != "Kjqmt7v" || r.Type != "DS" {
		t.Errorf("record = %+v, want Name=Kjqmt7v Type=DS", r)
	}
	if r.RData != "19036 8 2 49AAC11D7B6F6446702E54A1607371607A1A41855200FD2CE1CDDE32F24E8FB5 2019-01-11" {
		t.Errorf("RData = %q (retired key must carry validUntil date)", r.RData)
	}
	if recs[1].RData != "38696 8 2 683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16" {
		t.Errorf("RData = %q (active key must have no trailing date)", recs[1].RData)
	}
}

func TestDiff(t *testing.T) {
	oldTa, err := Parse([]byte(sampleXML))
	if err != nil {
		t.Fatal(err)
	}
	// 1) キー追加
	newXML := strings.Replace(sampleXML, "</TrustAnchor>",
		`    <KeyDigest id="Knew000" validFrom="2026-07-01T00:00:00+00:00">
        <KeyTag>99999</KeyTag><Algorithm>8</Algorithm><DigestType>2</DigestType>
        <Digest>AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA</Digest>
    </KeyDigest>
</TrustAnchor>`, 1)
	newTa, err := Parse([]byte(newXML))
	if err != nil {
		t.Fatal(err)
	}
	changes := Diff(oldTa, newTa)
	if len(changes) != 1 {
		t.Fatalf("Diff() = %d changes, want 1 (added)", len(changes))
	}
	if changes[0].Kind.String() != "added" || changes[0].Name != "Knew000" {
		t.Errorf("change = %+v, want added Knew000", changes[0])
	}

	// 2) キー退役 (validUntil 付与) → modified
	retiredXML := strings.Replace(sampleXML,
		`<KeyDigest id="Kmyv6jo" validFrom="2024-07-18T00:00:00+00:00">`,
		`<KeyDigest id="Kmyv6jo" validFrom="2024-07-18T00:00:00+00:00" validUntil="2027-01-11T00:00:00+00:00">`, 1)
	retiredTa, err := Parse([]byte(retiredXML))
	if err != nil {
		t.Fatal(err)
	}
	changes = Diff(oldTa, retiredTa)
	if len(changes) != 1 {
		t.Fatalf("Diff() = %d changes, want 1 (retirement = modified)", len(changes))
	}
	c := changes[0]
	if c.Kind.String() != "modified" || c.Name != "Kmyv6jo" {
		t.Errorf("change = %+v, want modified Kmyv6jo", c)
	}
	if !strings.HasSuffix(c.NewRData, "2027-01-11") {
		t.Errorf("NewRData = %q, want trailing validUntil date", c.NewRData)
	}

	// 3) キー削除
	removedXML := strings.Replace(sampleXML,
		`    <KeyDigest id="Kjqmt7v" validFrom="2010-07-15T00:00:00+00:00" validUntil="2019-01-11T00:00:00+00:00">
        <KeyTag>19036</KeyTag>
        <Algorithm>8</Algorithm>
        <DigestType>2</DigestType>
        <Digest>49AAC11D7B6F6446702E54A1607371607A1A41855200FD2CE1CDDE32F24E8FB5</Digest>
    </KeyDigest>
`, "", 1)
	removedTa, err := Parse([]byte(removedXML))
	if err != nil {
		t.Fatal(err)
	}
	changes = Diff(oldTa, removedTa)
	if len(changes) != 1 || changes[0].Kind.String() != "removed" || changes[0].Name != "Kjqmt7v" {
		t.Fatalf("changes = %+v, want removed Kjqmt7v", changes)
	}

	// 4) 同一 → 差分なし
	if changes := Diff(oldTa, oldTa); len(changes) != 0 {
		t.Fatalf("Diff(same) = %d changes, want 0", len(changes))
	}
}

func TestSerial(t *testing.T) {
	ta, _ := Parse([]byte(sampleXML))
	if got := Serial(ta); got != "0C05FDD6-422C-4910-8ED6-430ED15E11C2" {
		t.Errorf("Serial() = %q", got)
	}
	if got := Serial(TrustAnchors{}); got != "unknown" {
		t.Errorf("Serial(empty) = %q, want unknown", got)
	}
}

package diff

import (
	"strings"
	"testing"

	"github.com/yfujii/dns-root-diff/internal/zone"
)

func TestDiffNoChanges(t *testing.T) {
	old := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
		{Name: "aaa.", TTL: 172800, Class: "IN", Type: "NS", RData: "a.nic.aaa."},
	}
	new := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
		{Name: "aaa.", TTL: 172800, Class: "IN", Type: "NS", RData: "a.nic.aaa."},
	}

	changes := Diff(old, new)
	if len(changes) != 0 {
		t.Errorf("Diff() returned %d changes, want 0", len(changes))
	}
}

func TestDiffAddedRecord(t *testing.T) {
	old := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
	}
	new := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
		{Name: "bbb.", TTL: 172800, Class: "IN", Type: "NS", RData: "a.nic.bbb."},
	}

	changes := Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("Diff() returned %d changes, want 1", len(changes))
	}
	c := changes[0]
	if c.Kind != ChangeAdded {
		t.Errorf("Kind = %v, want ChangeAdded", c.Kind)
	}
	if c.Name != "bbb." || c.Type != "NS" {
		t.Errorf("Name=%q Type=%q", c.Name, c.Type)
	}
}

func TestDiffRemovedRecord(t *testing.T) {
	old := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
		{Name: "bbb.", TTL: 172800, Class: "IN", Type: "NS", RData: "a.nic.bbb."},
	}
	new := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
	}

	changes := Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("Diff() returned %d changes, want 1", len(changes))
	}
	c := changes[0]
	if c.Kind != ChangeRemoved {
		t.Errorf("Kind = %v, want ChangeRemoved", c.Kind)
	}
	if c.Name != "bbb." || c.Type != "NS" {
		t.Errorf("Name=%q Type=%q", c.Name, c.Type)
	}
}

func TestDiffModifiedRecord(t *testing.T) {
	old := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "SOA", RData: "a.root-servers.net. nstld.verisign-grs.com. 2026072301 1800 900 604800 86400"},
	}
	new := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "SOA", RData: "a.root-servers.net. nstld.verisign-grs.com. 2026072302 1800 900 604800 86400"},
	}

	changes := Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("Diff() returned %d changes, want 1", len(changes))
	}
	c := changes[0]
	if c.Kind != ChangeModified {
		t.Errorf("Kind = %v, want ChangeModified", c.Kind)
	}
	if c.OldRData == "" || c.NewRData == "" {
		t.Error("OldRData and NewRData should be set for modified records")
	}
}

func TestDiffMultipleRecordsSameNameType(t *testing.T) {
	old := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "b.root-servers.net."},
	}
	new := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "c.root-servers.net."},
	}

	changes := Diff(old, new)
	if len(changes) != 2 {
		t.Fatalf("Diff() returned %d changes, want 2 (1 removed + 1 added)", len(changes))
	}
}

func TestDiffTTLChangeOnly(t *testing.T) {
	old := []zone.Record{
		{Name: "aaa.", TTL: 172800, Class: "IN", Type: "NS", RData: "a.nic.aaa."},
	}
	new := []zone.Record{
		{Name: "aaa.", TTL: 86400, Class: "IN", Type: "NS", RData: "a.nic.aaa."},
	}

	changes := Diff(old, new)
	if len(changes) != 1 {
		t.Fatalf("Diff() returned %d changes, want 1", len(changes))
	}
	if changes[0].Kind != ChangeModified {
		t.Errorf("Kind = %v, want ChangeModified", changes[0].Kind)
	}
}

func TestDiffEmptyOld(t *testing.T) {
	new := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
	}

	changes := Diff(nil, new)
	if len(changes) != 1 {
		t.Fatalf("Diff() returned %d changes, want 1", len(changes))
	}
	if changes[0].Kind != ChangeAdded {
		t.Errorf("Kind = %v, want ChangeAdded", changes[0].Kind)
	}
}

func TestDiffEmptyNew(t *testing.T) {
	old := []zone.Record{
		{Name: ".", TTL: 86400, Class: "IN", Type: "NS", RData: "a.root-servers.net."},
	}

	changes := Diff(old, nil)
	if len(changes) != 1 {
		t.Fatalf("Diff() returned %d changes, want 1", len(changes))
	}
	if changes[0].Kind != ChangeRemoved {
		t.Errorf("Kind = %v, want ChangeRemoved", changes[0].Kind)
	}
}

func TestCategorize(t *testing.T) {
	tests := []struct {
		name    string
		change  Change
		wantCat Category
	}{
		{
			name:    "NS added is delegation",
			change:  Change{Kind: ChangeAdded, Name: "newgtld.", Type: "NS", NewRData: "ns1.newgtld."},
			wantCat: CategoryDelegation,
		},
		{
			name:    "NS removed is delegation",
			change:  Change{Kind: ChangeRemoved, Name: "oldgtld.", Type: "NS", OldRData: "ns1.oldgtld."},
			wantCat: CategoryDelegation,
		},
		{
			name:    "DS added is DNSSEC",
			change:  Change{Kind: ChangeAdded, Name: "example.", Type: "DS", NewRData: "12345 8 2 ABCDEF"},
			wantCat: CategoryDNSSEC,
		},
		{
			name:    "DNSKEY modified is DNSSEC",
			change:  Change{Kind: ChangeModified, Name: ".", Type: "DNSKEY"},
			wantCat: CategoryDNSSEC,
		},
		{
			name:    "NSEC modified is DNSSEC",
			change:  Change{Kind: ChangeModified, Name: "example.", Type: "NSEC"},
			wantCat: CategoryDNSSEC,
		},
		{
			name:    "NSEC3 modified is DNSSEC",
			change:  Change{Kind: ChangeModified, Name: "example.", Type: "NSEC3"},
			wantCat: CategoryDNSSEC,
		},
		{
			name:    "NSEC3PARAM added is DNSSEC",
			change:  Change{Kind: ChangeAdded, Name: ".", Type: "NSEC3PARAM", NewRData: "1 0 0 -"},
			wantCat: CategoryDNSSEC,
		},
		{
			name:    "CDS added is DNSSEC",
			change:  Change{Kind: ChangeAdded, Name: "example.", Type: "CDS", NewRData: "12345 8 2 ABCDEF"},
			wantCat: CategoryDNSSEC,
		},
		{
			name:    "RRSIG modified is signature",
			change:  Change{Kind: ChangeModified, Name: ".", Type: "RRSIG"},
			wantCat: CategorySignature,
		},
		{
			name:    "SOA modified is zone",
			change:  Change{Kind: ChangeModified, Name: ".", Type: "SOA"},
			wantCat: CategoryZone,
		},
		{
			name:    "ZONEMD modified is zone",
			change:  Change{Kind: ChangeModified, Name: ".", Type: "ZONEMD"},
			wantCat: CategoryZone,
		},
		{
			name:    "A record added is glue",
			change:  Change{Kind: ChangeAdded, Name: "ns1.example.", Type: "A", NewRData: "192.0.2.1"},
			wantCat: CategoryGlue,
		},
		{
			name:    "AAAA record added is glue",
			change:  Change{Kind: ChangeAdded, Name: "ns1.example.", Type: "AAAA", NewRData: "2001:db8::1"},
			wantCat: CategoryGlue,
		},
		{
			name:    "TXT record added is other",
			change:  Change{Kind: ChangeAdded, Name: "example.", Type: "TXT", NewRData: "v=spf1"},
			wantCat: CategoryOther,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Categorize(tt.change)
			if got != tt.wantCat {
				t.Errorf("Categorize() = %v, want %v", got, tt.wantCat)
			}
		})
	}
}

func TestCategorizeChanges(t *testing.T) {
	changes := []Change{
		{Kind: ChangeAdded, Name: "newgtld.", Type: "NS"},
		{Kind: ChangeAdded, Name: "newgtld.", Type: "DS"},
		{Kind: ChangeModified, Name: ".", Type: "SOA"},
		{Kind: ChangeModified, Name: ".", Type: "RRSIG"},
	}

	grouped := CategorizeChanges(changes)
	if len(grouped[CategoryDelegation]) != 1 {
		t.Errorf("delegation changes = %d, want 1", len(grouped[CategoryDelegation]))
	}
	if len(grouped[CategoryDNSSEC]) != 1 {
		t.Errorf("DNSSEC changes = %d, want 1", len(grouped[CategoryDNSSEC]))
	}
	if len(grouped[CategoryZone]) != 1 {
		t.Errorf("zone changes = %d, want 1", len(grouped[CategoryZone]))
	}
	if len(grouped[CategorySignature]) != 1 {
		t.Errorf("signature changes = %d, want 1", len(grouped[CategorySignature]))
	}
}

// rrsig は RRSIG の RDATA を組み立てる。
// TYPE-COVERED ALGORITHM LABELS ORIGINAL-TTL EXPIRATION INCEPTION KEY-TAG SIGNER SIGNATURE
func rrsig(covered, expiration, inception, keyTag, signature string) string {
	return covered + " 8 1 86400 " + expiration + " " + inception + " " + keyTag + " . " + signature
}

func TestMarkMechanicalSingleChange(t *testing.T) {
	const (
		soaOld = "a.root-servers.net. nstld.verisign-grs.com. 2026072500 1800 900 604800 86400"
		zmdOld = "2026072500 1 241 ABCDEF"
	)
	tests := []struct {
		name   string
		change Change
		want   bool
	}{
		{
			name: "RRSIG re-signing is mechanical",
			change: Change{Kind: ChangeModified, Name: "example.", Type: "RRSIG", OldTTL: 86400, NewTTL: 86400,
				OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA"),
				NewRData: rrsig("DS", "20260806170000", "20260724160000", "57780", "BBBB")},
			want: true,
		},
		{
			name: "RRSIG key tag change (ZSK roll) is mechanical",
			change: Change{Kind: ChangeModified, Name: "example.", Type: "RRSIG", OldTTL: 86400, NewTTL: 86400,
				OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA"),
				NewRData: rrsig("DS", "20260806170000", "20260724160000", "12345", "BBBB")},
			want: true,
		},
		{
			name: "RRSIG TTL change is not mechanical",
			change: Change{Kind: ChangeModified, Name: "example.", Type: "RRSIG", OldTTL: 86400, NewTTL: 172800,
				OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA"),
				NewRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA")},
			want: false,
		},
		{
			name: "RRSIG algorithm change is not mechanical",
			change: Change{Kind: ChangeModified, Name: "example.", Type: "RRSIG", OldTTL: 86400, NewTTL: 86400,
				OldRData: "DS 8 1 86400 20260806050000 20260724040000 57780 . AAAA",
				NewRData: "DS 13 1 86400 20260806170000 20260724160000 57780 . BBBB"},
			want: false,
		},
		{
			name: "unpaired RRSIG removal is not mechanical",
			change: Change{Kind: ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400,
				OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA")},
			want: false,
		},
		{
			name: "unpaired RRSIG addition is not mechanical",
			change: Change{Kind: ChangeAdded, Name: "example.", Type: "RRSIG", NewTTL: 86400,
				NewRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA")},
			want: false,
		},
		{
			name: "SOA serial bump is mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 86400,
				OldRData: soaOld,
				NewRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072501 1800 900 604800 86400"},
			want: true,
		},
		{
			name: "SOA MNAME change is not mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 86400,
				OldRData: soaOld,
				NewRData: "b.root-servers.net. nstld.verisign-grs.com. 2026072501 1800 900 604800 86400"},
			want: false,
		},
		{
			name: "SOA refresh change is not mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 86400,
				OldRData: soaOld,
				NewRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072501 3600 900 604800 86400"},
			want: false,
		},
		{
			name: "SOA minimum change is not mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 86400,
				OldRData: soaOld,
				NewRData: "a.root-servers.net. nstld.verisign-grs.com. 2026072501 1800 900 604800 3600"},
			want: false,
		},
		{
			name: "SOA TTL change is not mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "SOA", OldTTL: 86400, NewTTL: 172800,
				OldRData: soaOld, NewRData: soaOld},
			want: false,
		},
		{
			name:   "SOA added is not mechanical",
			change: Change{Kind: ChangeAdded, Name: ".", Type: "SOA", NewRData: soaOld},
			want:   false,
		},
		{
			name: "SOA with malformed RDATA is not mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "SOA",
				OldRData: "a. b. 2026072500", NewRData: "a. b. 2026072501"},
			want: false,
		},
		{
			name: "ZONEMD serial and digest update is mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "ZONEMD", OldTTL: 86400, NewTTL: 86400,
				OldRData: zmdOld, NewRData: "2026072501 1 241 FEDCBA"},
			want: true,
		},
		{
			name: "ZONEMD hash algorithm change is not mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "ZONEMD", OldTTL: 86400, NewTTL: 86400,
				OldRData: zmdOld, NewRData: "2026072501 1 242 FEDCBA"},
			want: false,
		},
		{
			name: "ZONEMD scheme change is not mechanical",
			change: Change{Kind: ChangeModified, Name: ".", Type: "ZONEMD", OldTTL: 86400, NewTTL: 86400,
				OldRData: zmdOld, NewRData: "2026072501 2 241 FEDCBA"},
			want: false,
		},
		{
			name:   "ZONEMD removed is not mechanical",
			change: Change{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldRData: zmdOld},
			want:   false,
		},
		{
			name:   "NS change is not mechanical",
			change: Change{Kind: ChangeAdded, Name: "newgtld.", Type: "NS", NewRData: "ns1.newgtld."},
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MarkMechanical([]Change{tt.change})[0]; got != tt.want {
				t.Errorf("MarkMechanical() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMarkMechanicalPairsRRSIG(t *testing.T) {
	// RRSIG が複数ある RRset は Diff が removed+added に分解する。
	// (type covered, algorithm, labels, original TTL, signer, TTL) が一致する組は再署名。
	changes := []Change{
		{Kind: ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400,
			OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA")},
		{Kind: ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400,
			OldRData: rrsig("NSEC", "20260806050000", "20260724040000", "57780", "BBBB")},
		{Kind: ChangeAdded, Name: "example.", Type: "RRSIG", NewTTL: 86400,
			NewRData: rrsig("DS", "20260806170000", "20260724160000", "57780", "CCCC")},
		{Kind: ChangeAdded, Name: "example.", Type: "RRSIG", NewTTL: 86400,
			NewRData: rrsig("NSEC", "20260806170000", "20260724160000", "57780", "DDDD")},
	}
	if got := Substantive(changes); len(got) != 0 {
		t.Errorf("Substantive() = %+v, want empty (routine re-signing)", got)
	}
}

func TestMarkMechanicalKeepsUnpairedRRSIG(t *testing.T) {
	// DS の署名が再署名されず失われたケース: NSEC の入れ替えだけが組になる。
	changes := []Change{
		{Kind: ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400,
			OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA")},
		{Kind: ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400,
			OldRData: rrsig("NSEC", "20260806050000", "20260724040000", "57780", "BBBB")},
		{Kind: ChangeAdded, Name: "example.", Type: "RRSIG", NewTTL: 86400,
			NewRData: rrsig("NSEC", "20260806170000", "20260724160000", "57780", "DDDD")},
	}
	got := Substantive(changes)
	if len(got) != 1 {
		t.Fatalf("Substantive() = %d changes, want 1: %+v", len(got), got)
	}
	if got[0].Kind != ChangeRemoved || !strings.HasPrefix(got[0].OldRData, "DS ") {
		t.Errorf("Substantive() kept %+v, want the unpaired DS signature removal", got[0])
	}
}

func TestMarkMechanicalPairsMultipleZONEMD(t *testing.T) {
	// ZONEMD が複数レコードあると Diff は modified に畳めず removed+added になる。
	// (scheme, hash algorithm) が一致する組は serial/digest の更新なので機械的変更。
	changes := []Change{
		{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072500 1 1 AAAA"},
		{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072500 1 2 BBBB"},
		{Kind: ChangeAdded, Name: ".", Type: "ZONEMD", NewTTL: 86400, NewRData: "2026072501 1 1 CCCC"},
		{Kind: ChangeAdded, Name: ".", Type: "ZONEMD", NewTTL: 86400, NewRData: "2026072501 1 2 DDDD"},
	}
	if got := Substantive(changes); len(got) != 0 {
		t.Errorf("Substantive() = %+v, want empty (routine re-signing of 2 digests)", got)
	}
}

func TestMarkMechanicalKeepsUnpairedZONEMD(t *testing.T) {
	tests := []struct {
		name    string
		changes []Change
		want    int // 実質的な変更として残る件数
	}{
		{
			name: "new digest algorithm added",
			changes: []Change{
				{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072500 1 1 AAAA"},
				{Kind: ChangeAdded, Name: ".", Type: "ZONEMD", NewTTL: 86400, NewRData: "2026072501 1 1 CCCC"},
				{Kind: ChangeAdded, Name: ".", Type: "ZONEMD", NewTTL: 86400, NewRData: "2026072501 1 2 DDDD"},
			},
			want: 1, // algorithm 2 の追加だけが残る
		},
		{
			name: "digest algorithm retired",
			changes: []Change{
				{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072500 1 1 AAAA"},
				{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072500 1 2 BBBB"},
				{Kind: ChangeAdded, Name: ".", Type: "ZONEMD", NewTTL: 86400, NewRData: "2026072501 1 1 CCCC"},
			},
			want: 1, // algorithm 2 の削除だけが残る
		},
		{
			name: "TTL changed",
			changes: []Change{
				{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072500 1 1 AAAA"},
				{Kind: ChangeAdded, Name: ".", Type: "ZONEMD", NewTTL: 172800, NewRData: "2026072501 1 1 CCCC"},
			},
			want: 2,
		},
		{
			name: "scheme changed",
			changes: []Change{
				{Kind: ChangeRemoved, Name: ".", Type: "ZONEMD", OldTTL: 86400, OldRData: "2026072500 1 1 AAAA"},
				{Kind: ChangeAdded, Name: ".", Type: "ZONEMD", NewTTL: 86400, NewRData: "2026072501 2 1 CCCC"},
			},
			want: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Substantive(tt.changes); len(got) != tt.want {
				t.Errorf("Substantive() = %d changes, want %d: %+v", len(got), tt.want, got)
			}
		})
	}
}

func TestCountMechanical(t *testing.T) {
	changes := []Change{
		{Kind: ChangeRemoved, Name: "a.", Type: "RRSIG", OldTTL: 86400,
			OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA")},
		{Kind: ChangeAdded, Name: "a.", Type: "RRSIG", NewTTL: 86400,
			NewRData: rrsig("DS", "20260806170000", "20260724160000", "57780", "BBBB")},
		{Kind: ChangeModified, Name: ".", Type: "SOA", OldRData: "a. b. 1 1 1 1 1", NewRData: "a. b. 2 1 1 1 1"},
		{Kind: ChangeAdded, Name: "b.", Type: "NS", NewRData: "ns1.b."},
	}
	if got := CountMechanical(changes, "RRSIG"); got != 2 {
		t.Errorf("CountMechanical(RRSIG) = %d, want 2", got)
	}
	if got := CountMechanical(changes, "NS"); got != 0 {
		t.Errorf("CountMechanical(NS) = %d, want 0", got)
	}
}

func TestSubstantiveDropsResigningNoise(t *testing.T) {
	// 再署名だけの回: RRSIG の入れ替えと SOA serial / ZONEMD の更新のみ。
	resigning := []Change{
		{Kind: ChangeRemoved, Name: "example.", Type: "RRSIG", OldTTL: 86400,
			OldRData: rrsig("DS", "20260806050000", "20260724040000", "57780", "AAAA")},
		{Kind: ChangeAdded, Name: "example.", Type: "RRSIG", NewTTL: 86400,
			NewRData: rrsig("DS", "20260806170000", "20260724160000", "57780", "BBBB")},
		{Kind: ChangeModified, Name: ".", Type: "SOA", OldRData: "a. b. 1 1 1 1 1", NewRData: "a. b. 2 1 1 1 1"},
		{Kind: ChangeModified, Name: ".", Type: "ZONEMD", OldRData: "1 1 241 old", NewRData: "2 1 241 new"},
	}
	if got := Substantive(resigning); len(got) != 0 {
		t.Errorf("Substantive(re-signing only) = %+v, want empty", got)
	}

	withReal := append(append([]Change(nil), resigning...),
		Change{Kind: ChangeAdded, Name: "newgtld.", Type: "NS", NewRData: "ns1.newgtld."},
		Change{Kind: ChangeAdded, Name: "ns1.newgtld.", Type: "A", NewRData: "192.0.2.1"},
	)
	got := Substantive(withReal)
	if len(got) != 2 {
		t.Fatalf("Substantive() = %d changes, want 2: %+v", len(got), got)
	}
	if got[0].Type != "NS" || got[1].Type != "A" {
		t.Errorf("Substantive() kept %+v, want the NS and A changes", got)
	}
}

func TestCountByCategory(t *testing.T) {
	changes := []Change{
		{Kind: ChangeAdded, Name: "a.", Type: "NS"},
		{Kind: ChangeAdded, Name: "b.", Type: "NS"},
		{Kind: ChangeAdded, Name: "a.", Type: "DS"},
		{Kind: ChangeModified, Name: ".", Type: "RRSIG"},
	}
	counts := CountByCategory(changes)
	if counts[CategoryDelegation] != 2 {
		t.Errorf("delegation = %d, want 2", counts[CategoryDelegation])
	}
	if counts[CategoryDNSSEC] != 1 {
		t.Errorf("DNSSEC = %d, want 1", counts[CategoryDNSSEC])
	}
	if counts[CategorySignature] != 1 {
		t.Errorf("signature = %d, want 1", counts[CategorySignature])
	}
	if counts[CategoryGlue] != 0 {
		t.Errorf("glue = %d, want 0", counts[CategoryGlue])
	}
}

func TestCategoriesIsACopyAndCoversEveryCategory(t *testing.T) {
	cats := Categories()
	if len(cats) == 0 {
		t.Fatal("Categories() is empty")
	}
	cats[0] = CategoryOther
	if again := Categories(); again[0] == CategoryOther {
		t.Error("Categories() returned a slice sharing the package state")
	}

	// Categorize が返しうる全カテゴリが表示順に含まれていること。
	seen := make(map[Category]bool)
	for _, cat := range Categories() {
		if seen[cat] {
			t.Errorf("Categories() contains %v twice", cat)
		}
		seen[cat] = true
	}
	for _, rrType := range []string{"NS", "DS", "DNSKEY", "NSEC", "NSEC3", "NSEC3PARAM", "CDS", "CDNSKEY", "A", "AAAA", "RRSIG", "SOA", "ZONEMD", "TXT"} {
		cat := Categorize(Change{Type: rrType})
		if !seen[cat] {
			t.Errorf("Categories() is missing %v (from %s)", cat, rrType)
		}
	}
}

func TestSortChangesDeterministic(t *testing.T) {
	shuffled := []Change{
		{Kind: ChangeModified, Name: ".", Type: "SOA", OldRData: "serial 1", NewRData: "serial 2"},
		{Kind: ChangeAdded, Name: "bbb.", Type: "NS", NewRData: "b.nic.bbb."},
		{Kind: ChangeRemoved, Name: "aaa.", Type: "NS", OldRData: "a.nic.aaa."},
		{Kind: ChangeAdded, Name: "aaa.", Type: "DS", NewRData: "12345 8 2 ABCDEF"},
		{Kind: ChangeAdded, Name: "aaa.", Type: "NS", NewRData: "b.nic.aaa."},
		{Kind: ChangeRemoved, Name: "aaa.", Type: "NS", OldRData: "c.nic.aaa."},
	}
	want := []Change{
		{Kind: ChangeModified, Name: ".", Type: "SOA", OldRData: "serial 1", NewRData: "serial 2"},
		{Kind: ChangeAdded, Name: "aaa.", Type: "DS", NewRData: "12345 8 2 ABCDEF"},
		{Kind: ChangeAdded, Name: "aaa.", Type: "NS", NewRData: "b.nic.aaa."},
		{Kind: ChangeRemoved, Name: "aaa.", Type: "NS", OldRData: "a.nic.aaa."},
		{Kind: ChangeRemoved, Name: "aaa.", Type: "NS", OldRData: "c.nic.aaa."},
		{Kind: ChangeAdded, Name: "bbb.", Type: "NS", NewRData: "b.nic.bbb."},
	}

	SortChanges(shuffled)
	if len(shuffled) != len(want) {
		t.Fatalf("len = %d, want %d", len(shuffled), len(want))
	}
	for i := range want {
		if shuffled[i] != want[i] {
			t.Errorf("changes[%d] = %+v, want %+v", i, shuffled[i], want[i])
		}
	}
}

func TestSortChangesEmpty(t *testing.T) {
	SortChanges(nil)
	SortChanges([]Change{})
}

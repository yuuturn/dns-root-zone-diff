package diff

import (
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

func TestIsSubstantive(t *testing.T) {
	tests := []struct {
		cat  Category
		want bool
	}{
		{CategoryDelegation, true},
		{CategoryDNSSEC, true},
		{CategoryGlue, true},
		{CategoryOther, true},
		{CategorySignature, false},
		{CategoryZone, false},
	}
	for _, tt := range tests {
		if got := tt.cat.IsSubstantive(); got != tt.want {
			t.Errorf("%v.IsSubstantive() = %v, want %v", tt.cat, got, tt.want)
		}
	}
}

func TestSubstantiveDropsResigningNoise(t *testing.T) {
	// 再署名だけの回: RRSIG の入れ替えと SOA serial / ZONEMD の更新のみ。
	resigning := []Change{
		{Kind: ChangeRemoved, Name: "example.", Type: "RRSIG", OldRData: "DS 8 1 86400 ... old"},
		{Kind: ChangeAdded, Name: "example.", Type: "RRSIG", NewRData: "DS 8 1 86400 ... new"},
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

func TestSubstantiveCategoriesIsACopy(t *testing.T) {
	cats := SubstantiveCategories()
	if len(cats) == 0 {
		t.Fatal("SubstantiveCategories() is empty")
	}
	cats[0] = CategorySignature
	if again := SubstantiveCategories(); again[0] == CategorySignature {
		t.Error("SubstantiveCategories() returned a slice sharing the package state")
	}
	for _, cat := range SubstantiveCategories() {
		if !cat.IsSubstantive() {
			t.Errorf("SubstantiveCategories() contains non-substantive %v", cat)
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

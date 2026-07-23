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
			name:    "RRSIG modified is DNSSEC",
			change:  Change{Kind: ChangeModified, Name: ".", Type: "RRSIG"},
			wantCat: CategoryDNSSEC,
		},
		{
			name:    "SOA modified is other",
			change:  Change{Kind: ChangeModified, Name: ".", Type: "SOA"},
			wantCat: CategoryOther,
		},
		{
			name:    "A record added is other",
			change:  Change{Kind: ChangeAdded, Name: "example.", Type: "A", NewRData: "192.0.2.1"},
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
	if len(grouped[CategoryDNSSEC]) != 2 {
		t.Errorf("DNSSEC changes = %d, want 2", len(grouped[CategoryDNSSEC]))
	}
	if len(grouped[CategoryOther]) != 1 {
		t.Errorf("other changes = %d, want 1", len(grouped[CategoryOther]))
	}
}

package diff

import (
	"reflect"
	"testing"
)

func TestTLDOf(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{".", "."},
		{"example.", "example."},
		{"ns1.example.", "example."},
		{"a.b.c.example.", "example."},
		{"xn--p1ai.", "xn--p1ai."},
		{"example", "example."}, // 末尾ドットなしも FQDN 形に正規化する
	}
	for _, tt := range tests {
		if got := TLDOf(tt.name); got != tt.want {
			t.Errorf("TLDOf(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestSummarizeByTLD(t *testing.T) {
	changes := []Change{
		{Kind: ChangeAdded, Name: "example.", Type: "NS", NewRData: "a."},
		{Kind: ChangeRemoved, Name: "example.", Type: "NS", OldRData: "b."},
		{Kind: ChangeAdded, Name: "ns1.example.", Type: "A", NewRData: "192.0.2.1"},
		{Kind: ChangeModified, Name: "single.", Type: "DS", OldRData: "1", NewRData: "2"},
	}

	got := SummarizeByTLD(changes)
	want := []TLDGroup{
		{
			TLD: "example.",
			Counts: []TypeCount{
				{Type: "A", Added: 1},
				{Type: "NS", Added: 1, Removed: 1},
			},
			Total: 3,
		},
		{
			TLD:    "single.",
			Counts: []TypeCount{{Type: "DS", Modified: 1}},
			Total:  1,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("SummarizeByTLD() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestSummarizeByTLDDeterministicOrder(t *testing.T) {
	// 件数が同じ TLD は名前昇順。map 反復順に依存しないことを繰り返し確認する。
	changes := []Change{
		{Kind: ChangeAdded, Name: "ccc.", Type: "NS", NewRData: "a."},
		{Kind: ChangeAdded, Name: "aaa.", Type: "NS", NewRData: "a."},
		{Kind: ChangeAdded, Name: "bbb.", Type: "NS", NewRData: "a."},
		{Kind: ChangeAdded, Name: "zzz.", Type: "NS", NewRData: "a."},
		{Kind: ChangeRemoved, Name: "zzz.", Type: "NS", OldRData: "b."},
	}
	want := []string{"zzz.", "aaa.", "bbb.", "ccc."} // 件数降順 → 名前昇順
	for i := 0; i < 20; i++ {
		groups := SummarizeByTLD(changes)
		got := make([]string, 0, len(groups))
		for _, g := range groups {
			got = append(got, g.TLD)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("iteration %d: order = %v, want %v", i, got, want)
		}
	}
}

func TestTypeCountTotal(t *testing.T) {
	tc := TypeCount{Type: "NS", Added: 2, Removed: 1, Modified: 3}
	if got := tc.Total(); got != 6 {
		t.Errorf("Total() = %d, want 6", got)
	}
}

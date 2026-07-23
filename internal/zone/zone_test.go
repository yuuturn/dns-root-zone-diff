package zone

import (
	"testing"
)

func TestParseBasicRecords(t *testing.T) {
	input := ".\t\t\t86400\tIN\tSOA\ta.root-servers.net. nstld.verisign-grs.com. 2026072301 1800 900 604800 86400\n" +
		".\t\t\t86400\tIN\tNS\ta.root-servers.net.\n" +
		"aaa.\t\t\t172800\tIN\tNS\ta.nic.aaa.\n" +
		"aaa.\t\t\t172800\tIN\tDS\t12345 8 2 ABCDEF123456\n"

	records, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("Parse() returned %d records, want 4", len(records))
	}

	r := records[0]
	if r.Name != "." || r.TTL != 86400 || r.Class != "IN" || r.Type != "SOA" {
		t.Errorf("records[0] = %+v", r)
	}
	if r.RData != "a.root-servers.net. nstld.verisign-grs.com. 2026072301 1800 900 604800 86400" {
		t.Errorf("records[0].RData = %q", r.RData)
	}

	r = records[1]
	if r.Name != "." || r.Type != "NS" || r.RData != "a.root-servers.net." {
		t.Errorf("records[1] = %+v", r)
	}

	r = records[2]
	if r.Name != "aaa." || r.TTL != 172800 || r.Type != "NS" {
		t.Errorf("records[2] = %+v", r)
	}

	r = records[3]
	if r.Name != "aaa." || r.Type != "DS" || r.RData != "12345 8 2 ABCDEF123456" {
		t.Errorf("records[3] = %+v", r)
	}
}

func TestParseRRSIG(t *testing.T) {
	input := ".\t86400\tIN\tRRSIG\tSOA 8 0 86400 20260805050000 20260723040000 57780 . LlwnxeyrNOGgFSRRoWsVsIAkgnOczrtWuj17CBl7nyDa5kk2ze4ssjGuNpmcPlDdjXOCe09EsF0qCszXgPBdHTM4CTffeVkObQdIKOzc3Gc6qR8SRasEnz2OL/PzrW0oO1TkKOss4USsxuu9sNyuRkTPlfTiNxuW2X+4pBWSBoloP5pBl+/rCQ2oSBYOQR9boRqbEQi4FqgzGBF0Ep8DkRdNPPY3mGCWvyi4QpG9cIrnYWrVJPDS8jMZzXD75r9iUxdnFFnnoIlnNwRxEi9k4P837JEP0wOjoYgtTxOP0QulA1BOdyweAezK4JdeTucRl/7oG5cVi7Gq0JOiJFq+sg==\n"

	records, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	r := records[0]
	if r.Type != "RRSIG" {
		t.Errorf("Type = %q, want RRSIG", r.Type)
	}
	if r.RData == "" {
		t.Error("RData is empty")
	}
}

func TestParseSkipsComments(t *testing.T) {
	input := "; this is a comment\n" +
		".\t86400\tIN\tNS\ta.root-servers.net.\n" +
		"; another comment\n"

	records, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1 (comments skipped)", len(records))
	}
}

func TestParseEmpty(t *testing.T) {
	records, err := Parse([]byte(""))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 0 {
		t.Errorf("got %d records, want 0", len(records))
	}
}

func TestParseInheritedName(t *testing.T) {
	input := ".\t86400\tIN\tNS\ta.root-servers.net.\n" +
		"\t86400\tIN\tNS\tb.root-servers.net.\n"

	records, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[1].Name != "." {
		t.Errorf("records[1].Name = %q, want inherited \".\"", records[1].Name)
	}
	if records[1].RData != "b.root-servers.net." {
		t.Errorf("records[1].RData = %q", records[1].RData)
	}
}

func TestParseDNSKEY(t *testing.T) {
	input := ".\t172800\tIN\tDNSKEY\t256 3 8 AwEAAeCYD6Z7WWKVLeuWgowKP+3g+Gs1cnLKq7a3CaQxQpv8bfuFVI0WnG33qaSH/Mw9IBgifrdzf4XY/DQLnyBJ9MfaOyAWuEaEmYJ+GQPiwVVfstGwSA1McfFJUttTgq2Huu74KARhtA8wPo/N3XcyYQtNhz+qCM5NBb3ecx/naw6sYab9LxS6f2cU0q03++BP5Ks0Uef8WJCa/1izCYE+vMkwoltV+tENa3hpXiZ7jle/xdgaZrPi5ZGmyLVI34g1XVYrNlsCCTmNvFQIfzW5STFQFsQpizczyFn9r3LzSxxPCNwdlCG84bER0BmdwqbF6Tanv+FxMOavrahkj4wIy5k=\n"

	records, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records, want 1", len(records))
	}
	if records[0].Type != "DNSKEY" {
		t.Errorf("Type = %q, want DNSKEY", records[0].Type)
	}
}

func TestParseInvalidLine(t *testing.T) {
	input := "invalid line\n"
	_, err := Parse([]byte(input))
	if err == nil {
		t.Fatal("Parse() expected error for invalid line")
	}
}

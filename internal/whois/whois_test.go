package whois

import (
	"context"
	"net"
	"strings"
	"testing"
)

// helsinkiResponse は IANA WHOIS の実レスポンス (2026-10-28 時点) を模したもの。
const helsinkiResponse = `% IANA WHOIS server
% for more information on IANA, visit http://www.iana.org
% This query returned 1 object

domain:       HELSINKI

whois:        

status:       FORMER
remarks:      Registration information: 

created:      2015-12-10
changed:      2026-10-28
source:       IANA
`

func TestParse(t *testing.T) {
	info, err := Parse([]byte(helsinkiResponse))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	want := Info{
		Domain:  "HELSINKI",
		Status:  "FORMER",
		Remarks: "Registration information:",
		Created: "2015-12-10",
		Changed: "2026-10-28",
		Source:  "IANA",
	}
	if info != want {
		t.Errorf("Parse() = %+v, want %+v", info, want)
	}
}

func TestParseIDN(t *testing.T) {
	data := `% IANA WHOIS server
domain:       vermögensberatung
domain-ace:   XN--VERMGENSBERATUNG-PWB
status:       ACTIVE
created:      2014-03-05
changed:      2026-10-01
source:       IANA
`
	info, err := Parse([]byte(data))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if info.Domain != "vermögensberatung" {
		t.Errorf("Domain = %q", info.Domain)
	}
	if info.DomainACE != "XN--VERMGENSBERATUNG-PWB" {
		t.Errorf("DomainACE = %q", info.DomainACE)
	}
	if info.Status != "ACTIVE" {
		t.Errorf("Status = %q", info.Status)
	}
}

func TestParseNotFound(t *testing.T) {
	data := `% IANA WHOIS server
% This query returned 0 objects.
%
% You queried for notatld. but this server does not have
% any data for notatld..
`
	if _, err := Parse([]byte(data)); err == nil {
		t.Fatal("Parse() = nil, want error for a response without a domain")
	}
}

func TestParseEmpty(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Fatal("Parse() = nil, want error for an empty response")
	}
}

func TestLookup(t *testing.T) {
	addr, query := startWhoisServer(t, helsinkiResponse)

	info, err := lookup(context.Background(), addr, "helsinki")
	if err != nil {
		t.Fatalf("lookup() error = %v", err)
	}
	if info.Domain != "HELSINKI" || info.Status != "FORMER" {
		t.Errorf("lookup() = %+v", info)
	}
	if got := <-query; got != "helsinki.\r\n" {
		t.Errorf("query = %q, want %q", got, "helsinki.\r\n")
	}
}

func TestLookupIDNMatchesDomainACE(t *testing.T) {
	data := `domain:       vermögensberatung
domain-ace:   XN--VERMGENSBERATUNG-PWB
status:       ACTIVE
source:       IANA
`
	addr, _ := startWhoisServer(t, data)
	info, err := lookup(context.Background(), addr, "xn--vermgensberatung-pwb")
	if err != nil {
		t.Fatalf("lookup() error = %v", err)
	}
	if info.DomainACE != "XN--VERMGENSBERATUNG-PWB" {
		t.Errorf("DomainACE = %q", info.DomainACE)
	}
}

func TestLookupTrailingDot(t *testing.T) {
	addr, query := startWhoisServer(t, helsinkiResponse)
	if _, err := lookup(context.Background(), addr, "HELSINKI."); err != nil {
		t.Fatalf("lookup() error = %v", err)
	}
	if got := <-query; got != "helsinki.\r\n" {
		t.Errorf("query = %q, want normalized lowercase query", got)
	}
}

func TestLookupRejectsInvalidTLD(t *testing.T) {
	for _, tld := range []string{"", "helsinki\r\nQUIT", "bad_tld", "-bad", ".", "a.b"} {
		if _, err := Lookup(context.Background(), tld); err == nil {
			t.Errorf("Lookup(%q) = nil, want validation error", tld)
		}
	}
}

func TestLookupNotFound(t *testing.T) {
	addr, _ := startWhoisServer(t, "% This query returned 0 objects.\n")
	if _, err := lookup(context.Background(), addr, "notatld"); err == nil {
		t.Fatal("lookup() = nil, want error for missing domain")
	}
}

func TestLookupDomainMismatch(t *testing.T) {
	addr, _ := startWhoisServer(t, helsinkiResponse)
	if _, err := lookup(context.Background(), addr, "other"); err == nil {
		t.Fatal("lookup() = nil, want error for a mismatched domain")
	}
}

func TestLookupDialError(t *testing.T) {
	// closed port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	if _, err := lookup(context.Background(), addr, "helsinki"); err == nil {
		t.Fatal("lookup() = nil, want dial error")
	}
}

// startWhoisServer は 1 接続だけ応答する簡易 WHOIS サーバを起動し、
// 待ち受けアドレスと受信したクエリを返すチャネルを返す。
func startWhoisServer(t *testing.T, response string) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	query := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			query <- ""
			return
		}
		defer func() { _ = conn.Close() }()
		buf := make([]byte, 256)
		n, _ := conn.Read(buf)
		query <- string(buf[:n])
		_, _ = conn.Write([]byte(response))
	}()
	return ln.Addr().String(), query
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	data := strings.Join([]string{
		"domain:       COM",
		"organisation: Example",
		"nserver:      A.GTLD-SERVERS.NET 192.5.6.30",
		"status:       ACTIVE",
		"remarks:      Registration information: https://example.com",
		"created:      1985-01-01",
		"changed:      2026-03-10",
		"source:       IANA",
	}, "\n")
	info, err := Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if info.Domain != "COM" || info.Status != "ACTIVE" || info.Remarks != "Registration information: https://example.com" {
		t.Errorf("Parse() = %+v", info)
	}
}

// Package whois は IANA WHOIS サーバから TLD の登録情報を取得する。
// TLD の追加・削除通知に添える最小限の情報 (domain / status / remarks /
// created / changed / source) だけを扱う。
package whois

import (
	"context"
	"fmt"
	"io"
	"net"
	"regexp"
	"strings"
	"time"
)

const (
	// defaultServer は IANA の WHOIS サーバ。
	defaultServer = "whois.iana.org:43"
	// defaultTimeout は1回の問い合わせの上限時間。
	defaultTimeout = 10 * time.Second
	// maxResponseSize は応答の読み込み上限 (暴走したサーバ対策)。
	maxResponseSize = 1 << 20
)

// tldPattern は問い合わせる TLD ラベルの形式。CRLF インジェクションを防ぐため
// 英数字とハイフンだけを許可する。
var tldPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// Info は WHOIS 応答から取り出した TLD の登録情報。
// 値は IANA の応答をそのまま保持する (例: Domain="HELSINKI", Status="FORMER")。
type Info struct {
	Domain    string // domain (IDN では Unicode 表記)
	DomainACE string // domain-ace (A-label。無ければ空)
	Status    string // status (ACTIVE / FORMER など)
	Remarks   string // remarks (最初の非空行)
	Created   string // created (YYYY-MM-DD)
	Changed   string // changed (YYYY-MM-DD)
	Source    string // source (IANA)
}

// Lookup は IANA WHOIS に tld を問い合わせる。
// tld は "com" / "com." / "xn--..." のいずれの形式でもよい。
func Lookup(ctx context.Context, tld string) (Info, error) {
	return lookup(ctx, defaultServer, tld)
}

// lookup は server に問い合わせる。server を差し替えられるようにして
// テストではローカルの WHOIS サーバを使う。
func lookup(ctx context.Context, server, tld string) (Info, error) {
	tld = strings.ToLower(strings.TrimSuffix(tld, "."))
	if !tldPattern.MatchString(tld) {
		return Info{}, fmt.Errorf("whois: invalid TLD %q", tld)
	}

	dialer := net.Dialer{Timeout: defaultTimeout}
	conn, err := dialer.DialContext(ctx, "tcp", server)
	if err != nil {
		return Info{}, fmt.Errorf("whois: dial %s: %w", server, err)
	}
	defer func() { _ = conn.Close() }()

	deadline := time.Now().Add(defaultTimeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = conn.SetDeadline(deadline)

	if _, err := io.WriteString(conn, tld+".\r\n"); err != nil {
		return Info{}, fmt.Errorf("whois: send query: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(conn, maxResponseSize))
	if err != nil {
		return Info{}, fmt.Errorf("whois: read response: %w", err)
	}

	info, err := Parse(data)
	if err != nil {
		return Info{}, err
	}
	if !info.matches(tld) {
		return Info{}, fmt.Errorf("whois: response domain %q does not match %q", info.Domain, tld)
	}
	return info, nil
}

// matches は応答の domain (または domain-ace) が問い合わせた TLD と一致するかを返す。
func (i Info) matches(tld string) bool {
	return strings.EqualFold(i.Domain, tld) || (i.DomainACE != "" && strings.EqualFold(i.DomainACE, tld))
}

// Parse は IANA WHOIS の応答を Info に変換する。
// コメント行 (%) と空行、不要なフィールドは無視する。
func Parse(data []byte) (Info, error) {
	var info Info
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "%") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "domain":
			if info.Domain == "" {
				info.Domain = value
			}
		case "domain-ace":
			if info.DomainACE == "" {
				info.DomainACE = value
			}
		case "status":
			if info.Status == "" {
				info.Status = value
			}
		case "remarks":
			if info.Remarks == "" && value != "" {
				info.Remarks = value
			}
		case "created":
			if info.Created == "" {
				info.Created = value
			}
		case "changed":
			if info.Changed == "" {
				info.Changed = value
			}
		case "source":
			if info.Source == "" {
				info.Source = value
			}
		}
	}
	if info.Domain == "" {
		return Info{}, fmt.Errorf("whois: response has no domain")
	}
	return info, nil
}

package notify

import (
	"context"

	"github.com/yfujii/dns-root-diff/internal/diff"
	"github.com/yfujii/dns-root-diff/internal/whois"
)

// Notifier は変更通知の送信先インターフェース。
type Notifier interface {
	Notify(ctx context.Context, changes []diff.Change) error
	// NotifyAnchors は root anchors (DNSSEC トラストアンカー) の変更を通知する。
	// 投稿タイトルと RDATA の扱いが zone 通知と異なる。
	NotifyAnchors(ctx context.Context, changes []diff.Change) error
	// NotifyTLDs は IANA の TLD リストの変更を通知する。
	// 投稿タイトルが zone / root anchors 通知と異なる。infos は追加・削除された
	// TLD の WHOIS 情報で、X / BlueSky は通知へのリプライとして投稿する。
	NotifyTLDs(ctx context.Context, changes []diff.Change, infos map[string]whois.Info) error
	Name() string
}

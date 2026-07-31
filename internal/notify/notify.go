package notify

import (
	"context"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

// Notifier は変更通知の送信先インターフェース。
type Notifier interface {
	Notify(ctx context.Context, changes []diff.Change) error
	// NotifyAnchors は root anchors (DNSSEC トラストアンカー) の変更を通知する。
	// 投稿タイトルと RDATA の扱いが zone 通知と異なる。
	NotifyAnchors(ctx context.Context, changes []diff.Change) error
	Name() string
}

package notify

import (
	"context"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

// Notifier は変更通知の送信先インターフェース。
type Notifier interface {
	Notify(ctx context.Context, changes []diff.Change) error
	Name() string
}

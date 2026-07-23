package notify

import (
	"context"
	"fmt"
	"strings"

	"github.com/yfujii/dns-root-diff/internal/diff"
)

// Notifier は変更通知の送信先インターフェース。
type Notifier interface {
	Notify(ctx context.Context, changes []diff.Change) error
	Name() string
}

// FormatMessage は変更内容を人間が読めるテキストにフォーマットする。
func FormatMessage(changes []diff.Change) string {
	if len(changes) == 0 {
		return ""
	}

	grouped := diff.CategorizeChanges(changes)

	var sb strings.Builder
	fmt.Fprintf(&sb, "DNS Root Zone: %d change(s) detected\n", len(changes))

	catOrder := []diff.Category{diff.CategoryDelegation, diff.CategoryDNSSEC, diff.CategoryOther}
	for _, cat := range catOrder {
		catChanges, ok := grouped[cat]
		if !ok || len(catChanges) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\n[%s]\n", cat.String())
		for _, c := range catChanges {
			sb.WriteString(formatChange(c))
		}
	}

	return sb.String()
}

func formatChange(c diff.Change) string {
	switch c.Kind {
	case diff.ChangeAdded:
		return fmt.Sprintf("  + %s %s %s\n", c.Name, c.Type, truncate(c.NewRData, 60))
	case diff.ChangeRemoved:
		return fmt.Sprintf("  - %s %s %s\n", c.Name, c.Type, truncate(c.OldRData, 60))
	case diff.ChangeModified:
		return fmt.Sprintf("  ~ %s %s\n    old: %s\n    new: %s\n", c.Name, c.Type, truncate(c.OldRData, 60), truncate(c.NewRData, 60))
	default:
		return ""
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

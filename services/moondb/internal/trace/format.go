package trace

import (
	"fmt"
	"time"
)

// FormatDuration renders milliseconds for a person: 850ms, 4.2s, 3m12s,
// 1h02m.
func FormatDuration(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return fmt.Sprintf("%dms", ms)
	case d < time.Minute:
		return fmt.Sprintf("%.1fs", d.Seconds())
	case d < time.Hour:
		return fmt.Sprintf("%dm%02ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
}

// FormatCost renders a USD amount with the precision its size needs, or
// "unknown" for a cost no price covers.
func FormatCost(usd *float64) string {
	switch {
	case usd == nil:
		return "unknown"
	case *usd == 0:
		return "$0"
	case *usd < 0.01:
		return fmt.Sprintf("$%.4f", *usd)
	case *usd < 1:
		return fmt.Sprintf("$%.3f", *usd)
	}
	return fmt.Sprintf("$%.2f", *usd)
}

// FormatTokens renders a token count: 950, 12.3k, 4.5M.
func FormatTokens(n int64) string {
	switch {
	case n < 1000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
}

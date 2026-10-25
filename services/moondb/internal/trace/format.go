package trace

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/moondb"
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

// FormatTotalsCost renders a trace's total cost: "-" when it has none and
// names no unpriced model, since no call billed a token and there is
// nothing to price, else FormatCost.
func FormatTotalsCost(t Totals) string {
	if t.Cost == nil && len(t.Unpriced) == 0 {
		return "-"
	}
	return FormatCost(t.Cost)
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

// markup matches an XML-like tag: the harness wraps a teammate message, a
// slash command or a task notification in one, and a prompt line reads
// better without it.
var markup = regexp.MustCompile(`</?[A-Za-z][\w:-]*(\s[^<>]*)?/?>`)

// PromptLine is the line a prompt is named by: its first line that holds
// text once its tags are gone, with runs of space collapsed, cut to
// moondb.SessionPromptLineRunes with an ellipsis; "" for a prompt with no
// text. A session's prompt line and a trace's turn prompt both use it.
func PromptLine(prompt string) string {
	for line := range strings.Lines(prompt) {
		text := strings.Join(strings.Fields(markup.ReplaceAllString(line, " ")), " ")
		if text == "" {
			continue
		}
		if utf8.RuneCountInString(text) <= moondb.SessionPromptLineRunes {
			return text
		}
		return string([]rune(text)[:moondb.SessionPromptLineRunes]) + "…"
	}
	return ""
}

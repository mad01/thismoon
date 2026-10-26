package trace

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
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
		return cutLine(text)
	}
	return ""
}

// cutLine cuts text to moondb.SessionPromptLineRunes with an ellipsis.
func cutLine(text string) string {
	if utf8.RuneCountInString(text) <= moondb.SessionPromptLineRunes {
		return text
	}
	return string([]rune(text)[:moondb.SessionPromptLineRunes]) + "…"
}

// The parts of a harness-written prompt a label names.
var (
	taskSummary = regexp.MustCompile(`<summary>([^<]*)</summary>`)
	teammateTag = regexp.MustCompile(`<teammate-message\b[^>]*>`)
	messageFrom = regexp.MustCompile(`<(?:agent|cross-session)-message\b[^>]*\bfrom="([^"]*)"`)
	attribute   = regexp.MustCompile(`([\w-]+)="([^"]*)"`)
)

// PromptLabel names a turn by its prompt for a lane or a list: a prompt
// the harness wrote by what started the turn (store.PromptOrigin), such
// as "background task finished: <summary>", "teammate message from
// <id>: <summary>" or "continued after compaction", and a person's prompt
// by its PromptLine. It is cut as PromptLine is.
func PromptLabel(prompt string) string {
	switch store.PromptOrigin(prompt) {
	case store.OriginTask:
		return labelWith("background task finished", firstMatch(taskSummary, prompt))
	case store.OriginTeammate:
		attrs := attributes(teammateTag.FindString(prompt))
		return labelWith("teammate message"+from(attrs["teammate_id"]), attrs["summary"])
	case store.OriginAgent:
		return cutLine("agent message" + from(firstMatch(messageFrom, prompt)))
	case store.OriginContinuation:
		return "continued after compaction"
	}
	return PromptLine(prompt)
}

// labelWith joins a label and its detail with a colon, the label alone
// when the detail is empty.
func labelWith(label, detail string) string {
	detail = strings.Join(strings.Fields(detail), " ")
	if detail == "" {
		return cutLine(label)
	}
	return cutLine(label + ": " + detail)
}

// from is " from <who>", "" when who is empty.
func from(who string) string {
	if who == "" {
		return ""
	}
	return " from " + who
}

func firstMatch(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

// attributes reads the name="value" pairs of one tag.
func attributes(tag string) map[string]string {
	out := map[string]string{}
	for _, m := range attribute.FindAllStringSubmatch(tag, -1) {
		out[m[1]] = m[2]
	}
	return out
}

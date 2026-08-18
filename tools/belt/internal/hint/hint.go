// Package hint holds belt's advisory hints. A hint inspects one tool call
// after it ran and either says nothing (nil) or returns text the model reads
// alongside the tool result. Hints never block: there is no denial path in
// the interface, so an over-eager hint costs context and nothing else. Guards
// (internal/guard) are the blocking half of belt; see docs/adr/0008.
package hint

import (
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// Event names match `belt hint <event>` and the hook matcher they are
// registered under. Search and bash run on PostToolUse; session-start runs on
// SessionStart, where there is no tool call — the input carries only the
// session's cwd and id.
const (
	EventSearch       = "search"        // matcher: the csl search MCP tools
	EventBash         = "bash"          // matcher: Bash
	EventSessionStart = "session-start" // hook: SessionStart
)

// Input carries the fields extracted from a PostToolUse payload.
type Input struct {
	Event     string
	Command   string   // bash: the shell command that ran
	Cwd       string   // the session working directory
	Repo      string   // search: the repo filter the search was given
	Paths     []string // search: repo-relative file paths the search returned
	SessionID string   // used to suppress repeat advice within one session
}

// Advice is one hint's output: which hint spoke and what it said.
type Advice struct {
	Hint string
	Text string
}

// Hint checks one tool call and optionally advises. Returning nil is the
// common case and must stay cheap — a hint runs on every matching tool call.
type Hint interface {
	ID() string
	Event() string
	Check(in Input) *Advice
}

// All returns every registered hint, enabled or not, in registration order.
// Introspection (belt doctor) needs the disabled ones too.
func All(cfg config.Config) []Hint {
	return []Hint{
		NewKeepAssertions(cfg),
		NewKeepConsult(cfg),
		NewPreferCSL(cfg),
	}
}

// ForEvent returns the enabled hints for an event, in fixed order.
func ForEvent(event string, cfg config.Config) []Hint {
	var out []Hint
	for _, h := range All(cfg) {
		if h.Event() == event && cfg.HintEnabled(h.ID()) {
			out = append(out, h)
		}
	}
	return out
}

// Run collects advice from every enabled hint for the input's event. Unlike
// guards, where the first denial wins, every hint gets to speak: they advise
// on different things and suppressing the second because the first fired
// would drop information for no benefit.
func Run(in Input, cfg config.Config) []Advice {
	var out []Advice
	for _, h := range ForEvent(in.Event, cfg) {
		if a := h.Check(in); a != nil {
			out = append(out, *a)
		}
	}
	return out
}

// Render joins advice into the single block that goes into
// hookSpecificOutput.additionalContext. Returns "" when there is nothing to
// say, which the caller treats as "emit no JSON at all".
func Render(advice []Advice) string {
	if len(advice) == 0 {
		return ""
	}
	var b strings.Builder
	for i, a := range advice {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("belt[")
		b.WriteString(a.Hint)
		b.WriteString("]: ")
		b.WriteString(a.Text)
	}
	return b.String()
}

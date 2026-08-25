package hint

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// humanizerMarker is the synthetic seen-file id that makes the nudge fire at
// most once per session.
const humanizerMarker = "humanizer-check-nudge"

// humanizerNameKey is matched as a raw JSON key/value substring of the
// transcript: any humanizer MCP tool counts as the session having linted its
// own prose. The `"name":"mcp__humanizer__` adjacency only occurs in a real
// tool_use block, while the bare tool name also appears in prose — including
// in this hint's own advice, which the transcript records verbatim.
var humanizerNameKey = []byte(`"name":"mcp__humanizer__`)

// readOnlyVerbs name operations that fetch rather than publish. The consuming
// repo's hook matcher decides which MCP tools reach this hint at all
// (docs/adr/0006); this list is what keeps a deliberately broad matcher — a
// whole server rather than named tools — from nudging on plain reads.
var readOnlyVerbs = []string{"get", "list", "search", "read", "fetch"}

// Humanizer nudges a session that published text to an external system
// without linting it. The instruction prose covers PR descriptions and
// documentation, so those get linted; a Slack line or a one-paragraph issue
// comment gets skipped for feeling too small to bother with. Length is not
// what makes AI-writing tells visible, and the readers are the same people.
type Humanizer struct {
	cfg config.Config
}

func NewHumanizer(cfg config.Config) *Humanizer {
	return &Humanizer{cfg: cfg}
}

func (h *Humanizer) ID() string    { return "humanizer-check" }
func (h *Humanizer) Event() string { return EventExternalText }

func (h *Humanizer) Check(in Input) *Advice {
	if !publishesText(in.ToolName) {
		return nil
	}
	// Without a session id the once-per-session guarantee is impossible, and
	// advice on every external write is worse than none.
	path := seen.path(in.SessionID)
	if path == "" {
		return nil
	}
	if seen.load(path)[humanizerMarker] {
		return nil // already nudged this session; skip before the transcript read
	}
	if raw, err := os.ReadFile(in.TranscriptPath); err == nil && bytes.Contains(raw, humanizerNameKey) {
		return nil // the session already linted something on its own
	}
	seen.record(path, []string{humanizerMarker})
	return &Advice{Hint: h.ID(), Text: humanizerAdvice(in.ToolName, publishedText(in.ToolInput))}
}

// publishesText reports whether a tool name looks like an MCP call that puts
// text somewhere other people read. Non-MCP tools never reach here through a
// correct matcher; rejecting them anyway keeps a stray matcher entry (Bash,
// say) from spending a session's one nudge on a local command.
func publishesText(tool string) bool {
	op, ok := strings.CutPrefix(tool, "mcp__")
	if !ok {
		return false
	}
	// mcp__<server>__<operation>: only the operation decides, so drop the
	// server segment first.
	if i := strings.LastIndex(op, "__"); i >= 0 {
		op = op[i+2:]
	}
	op = strings.ToLower(op)
	for _, verb := range readOnlyVerbs {
		// Servers put the verb at either end (get_file_contents, issue_read),
		// so both orders have to be recognised.
		if op == verb || strings.HasPrefix(op, verb+"_") || strings.HasSuffix(op, "_"+verb) {
			return false
		}
	}
	return true
}

// humanizerExcerptLen caps how much of the published text the nudge quotes:
// enough to identify the text unambiguously, small enough not to bloat the
// context the nudge rides in.
const humanizerExcerptLen = 200

// bodyKeys are the top-level field names publishing tools put their prose
// under, checked before the longest-string fallback so a long identifier (a
// URL, a page of ids) does not shadow the actual message.
var bodyKeys = []string{"text", "body", "comment", "message", "description", "content"}

// publishedText pulls the prose a publishing tool call sent: a known body
// field when one is set, else the longest string anywhere in the input.
func publishedText(input map[string]any) string {
	var best string
	for _, key := range bodyKeys {
		if s, ok := input[key].(string); ok && len(s) > len(best) {
			best = s
		}
	}
	if best != "" {
		return best
	}
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			if len(t) > len(best) {
				best = t
			}
		case map[string]any:
			for _, val := range t {
				walk(val)
			}
		case []any:
			for _, item := range t {
				walk(item)
			}
		}
	}
	walk(input)
	return best
}

// humanizerAdvice names the exact text to check, quoting its head so the
// instruction cannot be read as generic advice for later. The earlier
// generic "run humanizer_detect over the wording" form was skipped in half
// the sessions it fired in (MAD-300 audit).
func humanizerAdvice(toolName, published string) string {
	var b strings.Builder
	b.WriteString("that call")
	if toolName != "" {
		b.WriteString(" (")
		b.WriteString(toolName)
		b.WriteString(")")
	}
	b.WriteString(" published text other people will read. Run humanizer_detect on it now, " +
		"before the next task, and fix what it flags — a posted comment, a PR body, and a " +
		"Slack draft are all still editable after the fact. ")
	if published != "" {
		b.WriteString("The text to check is the one just sent, beginning: ")
		b.WriteString(strconv.Quote(truncateRunes(published, humanizerExcerptLen)))
		b.WriteString(" — pass the full body as humanizer_detect(text: ...). ")
	}
	b.WriteString("Short operational notes carry the same tells as long-form docs; " +
		"brevity is not an exemption. This reminder fires once per session.")
	return b.String()
}

// truncateRunes cuts s at the last rune boundary at or below max bytes,
// appending an ellipsis when anything was cut.
func truncateRunes(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

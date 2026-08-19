package hint

import (
	"bytes"
	"os"
	"strings"

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
	return &Advice{Hint: h.ID(), Text: humanizerAdvice}
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

const humanizerAdvice = "that call published text other people will read. " +
	"Run humanizer_detect over the wording and fix what it flags — a posted comment, " +
	"a PR body, and a Slack draft are all still editable after the fact. Short operational " +
	"notes carry the same tells as long-form docs; brevity is not an exemption. " +
	"This reminder fires once per session."

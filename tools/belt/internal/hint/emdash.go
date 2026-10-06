package hint

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
	"github.com/mad01/thismoon/tools/belt/internal/mcptool"
)

// EmDashID identifies the em dash hint. One id is registered twice in All,
// once for external-text and once for bash, like publish-internal-names.
const EmDashID = "em-dash"

// emDash is U+2014, written as an escape so this file stays free of the
// character it hunts.
const emDash = "\u2014"

// EmDash flags published text that carries em dashes. The user reads them
// as an AI tell, and an audit found them in roughly one PR body in four;
// humanizer_detect does not flag them. The hint fires on every offending
// publish, with no session dedupe: each one is a separate piece of text
// that is still editable.
type EmDash struct {
	cfg   config.Config
	event string
	// readFile reads a body file a gh command names. Injectable for tests.
	readFile func(path string) ([]byte, error)
}

// NewEmDash builds the hint for one of its two events.
func NewEmDash(cfg config.Config, event string) *EmDash {
	return &EmDash{cfg: cfg, event: event, readFile: os.ReadFile}
}

func (h *EmDash) ID() string    { return EmDashID }
func (h *EmDash) Event() string { return h.event }

func (h *EmDash) Check(in Input) *Advice {
	var n int
	if h.event == EventExternalText {
		n = toolEmDashes(in)
	} else {
		n = h.bashEmDashes(in)
	}
	if n == 0 {
		return nil
	}
	return &Advice{Hint: EmDashID, Text: emDashAdvice(n)}
}

// toolEmDashes counts em dashes in every string of a publishing MCP call's
// input. Read verbs publish nothing (mcptool's shared rule).
func toolEmDashes(in Input) int {
	if !mcptool.Publishes(in.ToolName) {
		return 0
	}
	n := 0
	var walk func(v any)
	walk = func(v any) {
		switch t := v.(type) {
		case string:
			n += strings.Count(t, emDash)
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
	walk(in.ToolInput)
	return n
}

// emDashGhSubs are the gh subcommands whose text the hint reads. api is
// handled separately: only a call that sends fields publishes text.
var emDashGhSubs = map[string][]string{
	"pr":    {"create", "edit", "comment"},
	"issue": {"create", "edit", "comment"},
}

// ghFieldFlags are the gh api flags that send a request body.
var ghFieldFlags = []string{"-f", "-F", "--field", "--raw-field", "--input"}

// bashEmDashes counts em dashes in a command that publishes through gh:
// the command text once (heredoc bodies span segments, so the whole text is
// what carries the body), plus each body file the calls name.
func (h *EmDash) bashEmDashes(in Input) int {
	if !strings.Contains(in.Command, "gh") {
		return 0 // cheap exit: most bash calls never touch gh
	}
	publishing := false
	var files []string
	for _, c := range guard.GhCalls(in.Command, in.Cwd) {
		if !ghPublishesText(c) {
			continue
		}
		publishing = true
		for _, f := range c.Files() {
			if path, ok := guard.ReferencedPath(c.Dir, f); ok && !slices.Contains(files, path) {
				files = append(files, path)
			}
		}
	}
	if !publishing {
		return 0
	}
	n := strings.Count(in.Command, emDash)
	for _, path := range files {
		if body, err := h.readFile(path); err == nil {
			n += strings.Count(string(body), emDash)
		}
	}
	return n
}

// ghPublishesText reports whether a gh call is one the hint reads.
func ghPublishesText(c guard.GhCall) bool {
	if c.Group == "api" {
		return len(c.FlagValues(ghFieldFlags...)) > 0
	}
	return slices.Contains(emDashGhSubs[c.Group], c.Sub)
}

// emDashAdvice tells the model how many em dashes went out and how to fix
// them while the text is still editable.
func emDashAdvice(n int) string {
	noun := "em dashes"
	if n == 1 {
		noun = "em dash"
	}
	return fmt.Sprintf("the published text contains %d %s; the user reads them as an AI tell. "+
		"Edit it now (e.g. mcp__gh_com__update_pull_request / gh pr edit) replacing them "+
		"with a colon, comma, parentheses, or a new sentence.", n, noun)
}

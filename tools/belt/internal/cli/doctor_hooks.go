package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/hint"
)

// cslReadTool is csl_read under the MCP server name the hooks docs use.
// The consuming repo may register csl under another name.
const cslReadTool = "mcp__csl__csl_read"

// plainMatcher is a Claude Code matcher made only of tool names and |,
// which Claude Code compares as a list of exact names rather than as a
// regular expression.
var plainMatcher = regexp.MustCompile(`^[A-Za-z0-9_|]+$`)

// matcherServer finds the MCP server names a matcher spells out, so a
// regex naming its own server is tried on csl_read under that name.
var matcherServer = regexp.MustCompile(`mcp__([A-Za-z0-9_-]+?)__`)

// printHookWiring renders the PostToolUse entries in the Claude settings
// that run `belt hint read`, and warns when none of their matchers lets
// read-size see a csl_read. The warning stays in this section, out of the
// warnings list: read-size is advisory, so the gap costs a nudge and not a
// guard, and it must not fail the exit status a provisioning check gates
// on. With no such entry, or with the Claude settings read switched off,
// the check is skipped.
func printHookWiring(w io.Writer, cfg config.Config, p config.Paths) {
	fmt.Fprintln(
		w,
		"\nhook wiring, the PostToolUse entries in the Claude settings that run belt hint read:",
	)
	if !cfg.ClaudeSettings.ReadEnabled() {
		fmt.Fprintln(w, "  not checked (claude_settings.enabled: false)")
		return
	}
	var reads []config.ClaudeHook
	for _, h := range config.LoadClaudeHooks("PostToolUse", p.ClaudeSettings...) {
		if runsBeltHintRead(h.Command) {
			reads = append(reads, h)
		}
	}
	if len(reads) == 0 {
		fmt.Fprintln(w, "  (none: read-size is not wired)")
		return
	}
	for _, h := range reads {
		fmt.Fprintf(w, "  matcher %q  (%s)\n", h.Matcher, h.File)
	}
	if warn := readMatcherWarning(reads); warn != "" {
		fmt.Fprintf(w, "  WARNING: %s\n", warn)
	}
}

// readMatcherWarning names the matcher to set when no belt hint read entry
// lets a csl_read through, and is empty when one does.
func readMatcherWarning(reads []config.ClaudeHook) string {
	for _, h := range reads {
		if matcherSeesCSLRead(h.Matcher) {
			return ""
		}
	}
	matcher := reads[0].Matcher
	return fmt.Sprintf(
		"matcher %q lets no csl_read through, so read-size stays silent after a whole-file csl_read."+
			" Set it to %q, with your csl MCP server's registration name as the middle segment.",
		matcher,
		matcher+"|"+cslReadTool,
	)
}

// runsBeltHintRead reports whether a hook command runs `belt hint read`:
// a word whose base name is belt, followed by hint and read.
func runsBeltHintRead(command string) bool {
	words := strings.Fields(command)
	for i := 0; i+2 < len(words); i++ {
		if filepath.Base(words[i]) == "belt" && words[i+1] == "hint" && words[i+2] == "read" {
			return true
		}
	}
	return false
}

// matcherSeesCSLRead reports whether a Claude Code hook matcher matches
// csl_read under some MCP server name. It follows Claude Code's matcher
// rule: an empty matcher or "*" matches every tool, one made only of tool
// names and | is a list of exact names, and anything else is a regular
// expression searched for in the tool name. A regex is tried on csl_read
// under the usual server name and under each server name it spells out.
func matcherSeesCSLRead(matcher string) bool {
	switch {
	case matcher == "" || matcher == "*":
		return true
	case plainMatcher.MatchString(matcher):
		return slices.ContainsFunc(strings.Split(matcher, "|"), hint.IsCSLRead)
	}
	re, err := regexp.Compile(matcher)
	if err != nil {
		return false
	}
	if re.MatchString(cslReadTool) {
		return true
	}
	for _, m := range matcherServer.FindAllStringSubmatch(matcher, -1) {
		if re.MatchString("mcp__" + m[1] + "__csl_read") {
			return true
		}
	}
	return false
}

// Package guard holds belt's guard implementations. A guard inspects one
// tool-call input and either allows it (nil) or denies it with a reason the
// model reads. Guards for the same event run in registration order; the first
// denial wins.
package guard

import (
	"fmt"
	"maps"
	"slices"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// Event names match `belt hook <event>` and the settings.json matcher they
// are registered under. They are aliases of the config constants: the config
// file names the same events for custom guards, and validating them there
// means one source for what a legal event is.
const (
	EventBash  = config.EventBash  // matcher: Bash
	EventWrite = config.EventWrite // matcher: Write|Edit
	// matcher: the MCP tools that publish to a public code host (the
	// consuming repo picks them, docs/adr/0006).
	EventExternalText = config.EventExternalText
)

// Events lists every valid guard event. CLI validation and its error text
// read from here so a new event cannot be added above and then be silently
// rejected at the command line.
func Events() []string {
	return []string{EventBash, EventWrite, EventExternalText}
}

// Input carries the fields extracted from a PreToolUse payload.
type Input struct {
	Event    string
	Command  string // bash: the shell command about to run
	Cwd      string // bash: the session working directory
	FilePath string // write: target file
	Content  string // write: content being written (Write content / Edit new_string)
	ToolName string // external-text: the MCP tool about to run
	// ToolInput carries the external-text tool call's input fields: every
	// string in it is text that is about to leave the machine.
	ToolInput map[string]any
}

// Denial is a blocked tool call: which guard fired and why.
type Denial struct {
	Guard  string
	Reason string
}

// Reasonf builds a denial with the guard id prefixed so a block is always
// attributable to the guard that fired.
func Reasonf(id, format string, args ...any) *Denial {
	return &Denial{Guard: id, Reason: fmt.Sprintf("belt[%s]: ", id) + fmt.Sprintf(format, args...)}
}

// Guard checks one input against one rule.
type Guard interface {
	ID() string
	Event() string
	Check(in Input) *Denial
}

// All returns every registered guard, enabled or not, in registration order.
// Introspection (belt doctor) needs the disabled ones too: a guard filtered
// out of ForEvent looks identical to one that never existed. Built-in guards
// run first in fixed order; custom guards follow, alphabetical by name so
// the order is deterministic across runs.
func All(cfg config.Config) []Guard {
	guards := []Guard{
		NewGitPushMain(cfg),
		NewGitIdentity(cfg),
		NewCommitGuard(cfg),
		NewScriptDenyList(cfg),
		NewWriteInternalNames(cfg),
		// One guard id on two events: the bash half watches git push, git
		// commit, branch and tag creation, and gh; the external-text half
		// watches the gh MCP tools. ForEvent keeps them apart, GuardEnabled
		// switches both.
		NewPublishInternalNames(cfg, EventBash),
		NewPublishInternalNames(cfg, EventExternalText),
	}
	for _, name := range slices.Sorted(maps.Keys(cfg.CustomGuards)) {
		guards = append(guards, NewCustom(name, cfg.CustomGuards[name]))
	}
	return guards
}

// ForEvent returns the enabled guards for an event, in fixed order.
func ForEvent(event string, cfg config.Config) []Guard {
	var out []Guard
	for _, g := range All(cfg) {
		if g.Event() == event && cfg.GuardEnabled(g.ID()) {
			out = append(out, g)
		}
	}
	return out
}

// Run checks the input against every enabled guard for its event and returns
// the first denial, or nil when all guards allow.
func Run(in Input, cfg config.Config) *Denial {
	for _, g := range ForEvent(in.Event, cfg) {
		if d := g.Check(in); d != nil {
			return d
		}
	}
	return nil
}

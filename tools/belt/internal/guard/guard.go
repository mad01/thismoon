// Package guard holds belt's guard implementations. A guard inspects one
// tool-call input and either allows it (nil) or denies it with a reason the
// model reads. Guards for the same event run in registration order; the first
// denial wins.
package guard

import (
	"fmt"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// Event names match `belt hook <event>` and the settings.json matcher they
// are registered under.
const (
	EventBash  = "bash"  // matcher: Bash
	EventWrite = "write" // matcher: Write|Edit
)

// Input carries the fields extracted from a PreToolUse payload.
type Input struct {
	Event    string
	Command  string // bash: the shell command about to run
	Cwd      string // bash: the session working directory
	FilePath string // write: target file
	Content  string // write: content being written (Write content / Edit new_string)
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

// ForEvent returns the enabled guards for an event, in fixed order.
func ForEvent(event string, cfg config.Config) []Guard {
	all := []Guard{
		NewGitPushMain(cfg),
		NewScriptDenyList(cfg),
		NewWriteInternalNames(cfg),
	}
	var out []Guard
	for _, g := range all {
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

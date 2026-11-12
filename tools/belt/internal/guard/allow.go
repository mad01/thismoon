package guard

import (
	"fmt"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// An allow is belt's fourth concept (docs/adr/0032). It sits at PreToolUse
// beside the guards and may answer allow for one narrow call shape, so the
// call runs without a permission prompt. It has no denial path: a call it
// stays silent on goes on to Claude Code's own permission check. The hook
// runs the allows only after RunClean reported a clean pass (no guard
// denied, failed open, or warned in soft mode), and an unreadable payload
// or config denies before either runs, so a deny always wins.

// Approval is a tool call an allow answers allow for: which allow and why.
type Approval struct {
	Allow  string
	Reason string
}

// approvalf builds an approval with the allow id prefixed, in the same
// belt[<id>]: form a denial carries, so an answer is attributable too.
func approvalf(id, format string, args ...any) *Approval {
	return &Approval{
		Allow:  id,
		Reason: fmt.Sprintf("belt[%s]: ", id) + fmt.Sprintf(format, args...),
	}
}

// Allow approves one input or returns nil.
type Allow interface {
	ID() string
	Event() string
	Approve(in Input) *Approval
}

// Allows returns every registered allow, enabled or not, in registration
// order. Introspection (belt doctor, belt config) needs the disabled ones,
// which is every allow on a machine that never turned one on.
func Allows(cfg config.Config) []Allow {
	return []Allow{NewGitCRead(cfg)}
}

// Approve returns the first approval an enabled allow for the input's
// event gives, or nil. Callers run it only after RunClean returned a clean
// pass: an approval never overrides a denial or a guard's warning.
func Approve(in Input, cfg config.Config) *Approval {
	for _, a := range Allows(cfg) {
		if a.Event() != in.Event || !cfg.AllowEnabled(a.ID()) {
			continue
		}
		if ap := a.Approve(in); ap != nil {
			return ap
		}
	}
	return nil
}

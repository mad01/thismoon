package guard

import "github.com/mad01/thismoon/tools/belt/internal/config"

// A guard can let a call through in two ways. It can find nothing to deny,
// or it can find something and let the call through anyway with a warn
// event: a custom guard that timed out, failed to start or exited with
// neither 0 nor 1 (it fails open), and any guard in soft mode that would
// have denied. Both return nil from Check. The second kind is a pass only
// for the permission prompt that follows, never for an allow, so the hook
// asks RunClean which kind it got (docs/adr/0032).

// RunClean is Run for a caller that acts on a pass. It returns the first
// denial, as Run does, and clean, which is true only when no guard denied
// and none emitted a warning while it checked the input.
func RunClean(in Input, cfg config.Config) (d *Denial, clean bool) {
	warned := false
	for _, g := range ForEvent(in.Event, cfg) {
		if w, ok := g.(warner); ok {
			w.noteWarnings(func() { warned = true })
		}
		if d := g.Check(in); d != nil {
			return d, false
		}
	}
	return nil, !warned
}

// emitFunc is the events-service emitter every warning guard holds.
type emitFunc = func(source, level, title, message string, tags map[string]string)

// warner is a guard that can let a call through with a warning. Every
// guard that holds an emit func is one: the guard emits only to warn, and
// TestEveryEmittingGuardIsAWarner holds new guards to it.
type warner interface {
	// noteWarnings makes the guard call note before each event it emits.
	noteWarnings(note func())
}

// noting wraps emit so that every event it sends calls note first.
func noting(emit emitFunc, note func()) emitFunc {
	return func(source, level, title, message string, tags map[string]string) {
		note()
		emit(source, level, title, message, tags)
	}
}

func (g *CommitGuard) noteWarnings(note func())          { g.emit = noting(g.emit, note) }
func (c *Custom) noteWarnings(note func())               { c.emit = noting(c.emit, note) }
func (g *GitDiscard) noteWarnings(note func())           { g.emit = noting(g.emit, note) }
func (g *GitIdentity) noteWarnings(note func())          { g.emit = noting(g.emit, note) }
func (g *PublishInternalNames) noteWarnings(note func()) { g.emit = noting(g.emit, note) }
func (g *ScriptDenyList) noteWarnings(note func())       { g.emit = noting(g.emit, note) }

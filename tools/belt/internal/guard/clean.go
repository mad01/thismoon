package guard

import (
	"fmt"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// A guard can let a call through in two ways. It can find nothing to deny,
// or it can find something and let the call through anyway with a warn
// event: a custom guard that timed out, failed to start or exited with
// neither 0 nor 1 (it fails open), and any guard in soft mode that would
// have denied. Both return nil from Check. The second kind is a pass only
// for the permission prompt that follows, never for an allow, so the hook
// asks RunClean which kind it got (docs/adr/0032).

// A guard in hint mode (config.HintModeGuards) has a third way: it lets
// the call through and gives advice the model reads beside it. Advice is
// not a pass an allow may act on either, and a denial from any guard drops
// it, since the call does not run.

// RunClean is Run for a caller that acts on a pass. It returns the first
// denial, as Run does, and clean, which is true only when no guard denied,
// none emitted a warning and none gave advice while it checked the input.
func RunClean(in Input, cfg config.Config) (d *Denial, clean bool) {
	d, clean, _ = RunAdvised(in, cfg)
	return d, clean
}

// RunAdvised is RunClean with the advice the guards in hint mode gave, in
// guard order. advice is nil whenever d is set.
func RunAdvised(in Input, cfg config.Config) (d *Denial, clean bool, advice []Advice) {
	warned := false
	for _, g := range ForEvent(in.Event, cfg) {
		if w, ok := g.(warner); ok {
			w.noteWarnings(func() { warned = true })
		}
		d, given := Verdict(g, in)
		if d != nil {
			return d, false, nil
		}
		advice = append(advice, given...)
	}
	return nil, !warned && len(advice) == 0, advice
}

// Advice is what a guard in hint mode tells the model about a call it lets
// through: which guard and the text, prefixed belt[<guard-id>]: as a
// denial's reason is.
type Advice struct {
	Guard string
	Text  string
}

// advicef builds advice with the guard id prefixed, the way Reasonf builds
// a denial.
func advicef(id, format string, args ...any) Advice {
	return Advice{Guard: id, Text: fmt.Sprintf("belt[%s]: ", id) + fmt.Sprintf(format, args...)}
}

// adviser is a guard that can let a call through with advice instead of a
// denial.
type adviser interface {
	// noteAdvice makes the guard hand each piece of advice to note.
	noteAdvice(note func(Advice))
}

// Verdict runs one guard on the input and returns its denial, or the
// advice it gave while letting the call through. belt check prints both.
func Verdict(g Guard, in Input) (*Denial, []Advice) {
	var advice []Advice
	if a, ok := g.(adviser); ok {
		a.noteAdvice(func(x Advice) { advice = append(advice, x) })
	}
	if d := g.Check(in); d != nil {
		return d, nil
	}
	return nil, advice
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
func (g *DirectDenyList) noteWarnings(note func())       { g.emit = noting(g.emit, note) }
func (c *Custom) noteWarnings(note func())               { c.emit = noting(c.emit, note) }
func (g *GitDiscard) noteWarnings(note func())           { g.emit = noting(g.emit, note) }
func (g *GitIdentity) noteWarnings(note func())          { g.emit = noting(g.emit, note) }
func (g *PublishInternalNames) noteWarnings(note func()) { g.emit = noting(g.emit, note) }
func (g *ScriptDenyList) noteWarnings(note func())       { g.emit = noting(g.emit, note) }

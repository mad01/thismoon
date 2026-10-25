// Package record is the capture side of moondb: the logic behind `moondb
// record`, which a harness hook runs with one event payload on stdin. It
// turns the payload into spool envelopes and returns; serve does the rest.
// It copies belt's record contract (docs/adr/0024): nothing on stdout, exit
// 0 always, and a session that never waits long. Every git call is bounded
// by a deadline, and when the budget runs out an edit is written with what
// was gathered so far. A file a Bash command changed that the budget never
// reached is counted on the turn instead.
//
// Run keeps a little state per session under <workdir>/sessions, read back
// by later events of the same session and by nothing else:
//
//	<session>.turn     "<prompt_id> <offset>": the open turn and the
//	                   transcript size when it opened
//	<session>.paths    "<worktree>\t<path>" per line: the files the session
//	                   edited, which every snapshot of the worktree covers
//	<session>.touched  one worktree per line: where the session edited
//	<session>.heads    "<worktree>\t<head>" per line, last line wins: the
//	                   HEAD last seen in each touched worktree
//	<session>.snap     JSON, per worktree: the state hash of the last
//	                   snapshot taken there, the size, time, blob and chunk
//	                   of each file it read, and the chunk of each HEAD blob
//	                   it shipped, so unchanged content is not read again
package record

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Harness names the agent runtime whose hook payloads Run reads and whose
// transcripts `moondb import` reads; every envelope carries it.
const Harness = "claude-code"

// Hook events Run records; every other event is ignored.
const (
	eventPrompt   = "UserPromptSubmit"
	eventToolUsed = "PostToolUse"
	eventStop     = "Stop"
)

// Options configures one Run.
type Options struct {
	// Workdir is the moondb state directory. The spool is <Workdir>/spool,
	// the store's objects <Workdir>/objects, the session state
	// <Workdir>/sessions.
	Workdir string
	// Now stamps the envelopes; time.Now when nil.
	Now func() time.Time
	// Excludes are the globs whose contents are never captured;
	// moondb.DefaultExcludes when nil.
	Excludes []string
	// Budget is how long Run gathers before it writes what it has;
	// moondb.RecordBudget when zero.
	Budget time.Duration
	// BashBudget is Budget for a Bash payload, whose hook is async;
	// moondb.BashBudget when zero.
	BashBudget time.Duration
	// SnapshotBudget is how long the turn-edge events spend on snapshots,
	// after their own envelope; moondb.SnapshotBudget when zero.
	SnapshotBudget time.Duration
	// Logf receives one line for each Bash payload whose files Run left
	// out, with the counts; nil discards it.
	Logf func(format string, args ...any)
}

// Run records one hook payload read from in. It returns an error only for
// a payload it cannot decode (or that lacks the ids its event needs) and
// for a spool it cannot write; everything else it fails to gather is left
// out of the envelope. The caller recovers panics and exits 0 regardless.
func Run(ctx context.Context, in io.Reader, opts Options) error {
	if opts.Workdir == "" {
		return errors.New("record: no workdir")
	}
	p, err := decodePayload(in)
	if err != nil {
		return err
	}
	opts = opts.withDefaults()
	budgeted, cancel := context.WithTimeout(ctx, opts.budgetFor(p))
	defer cancel()
	rec := &recorder{
		base:  ctx,
		ctx:   budgeted,
		start: opts.Now(),
		p:     p,
		opts:  opts,
		locs:  map[string]store.Location{},
		spool: &spool.Writer{
			Dir:     filepath.Join(opts.Workdir, moondb.SpoolDirName),
			Objects: filepath.Join(opts.Workdir, moondb.ObjectsDirName),
		},
		state: sessionState{
			dir: filepath.Join(opts.Workdir, moondb.SessionsDirName),
			id:  p.sessionID,
		},
	}
	switch p.event {
	case eventPrompt:
		return rec.require(rec.openTurn, "prompt_id", p.promptID)
	case eventToolUsed:
		return rec.require(rec.toolUsed, "tool_use_id", p.toolUseID)
	case eventStop:
		return rec.require(rec.closeTurn, "prompt_id", p.promptID)
	default:
		return nil
	}
}

func (o Options) withDefaults() Options {
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Excludes == nil {
		o.Excludes = moondb.DefaultExcludes
	}
	if o.Budget <= 0 {
		o.Budget = moondb.RecordBudget
	}
	if o.BashBudget <= 0 {
		o.BashBudget = moondb.BashBudget
	}
	if o.SnapshotBudget <= 0 {
		o.SnapshotBudget = moondb.SnapshotBudget
	}
	if o.Logf == nil {
		o.Logf = func(string, ...any) {}
	}
	return o
}

// budgetFor is how long Run gathers for p: BashBudget for a Bash payload,
// whose async hook nobody waits on, Budget for every other.
func (o Options) budgetFor(p payload) time.Duration {
	if p.event == eventToolUsed && p.tool == toolBash {
		return o.BashBudget
	}
	return o.Budget
}

// recorder is one Run in progress. ctx ends when the budget does; base is
// the context Run was given, which a snapshot takes its own budget from.
// start is when the hook began, before it read anything.
type recorder struct {
	base  context.Context
	ctx   context.Context
	start time.Time
	p     payload
	opts  Options
	spool *spool.Writer
	state sessionState
	// locs caches the git location of each directory placed in this Run.
	locs map[string]store.Location
}

// within returns a copy of rec that works under ctx instead of the budget
// Run set, sharing everything else.
func (rec *recorder) within(ctx context.Context) *recorder {
	c := *rec
	c.ctx = ctx
	return &c
}

// require runs handle once the session id and the event's own id are set:
// without them nothing written could be told apart from the next payload.
func (rec *recorder) require(handle func() error, key, id string) error {
	if rec.p.sessionID == "" {
		return fmt.Errorf("record: %s payload has no session_id", rec.p.event)
	}
	if id == "" {
		return fmt.Errorf("record: %s payload has no %s", rec.p.event, key)
	}
	return handle()
}

// envelope starts an envelope of kind with what every payload carries,
// stamped when the hook began. The Bash hook is async, so an Edit of a file
// the command changed can land before the hook reads it; stamped before
// the read, the Bash edit sorts before that Edit. A snapshot restamps its
// envelope once its reads are done.
func (rec *recorder) envelope(kind spool.Kind, id string) spool.Envelope {
	p := rec.p
	return spool.Envelope{
		Version: spool.Version,
		Kind:    kind,
		ID:      id,
		At:      rec.start,
		Harness: Harness,
		Session: spool.SessionRef{
			ID: p.sessionID, Cwd: p.cwd, Title: p.title, TranscriptPath: p.transcriptPath,
		},
		Turn:  spool.TurnRef{ID: p.promptID},
		Agent: spool.AgentRef{ID: p.agentID, Type: p.agentType},
	}
}

// spent reports whether the budget has run out.
func (rec *recorder) spent() bool {
	return rec.ctx.Err() != nil
}

// gitCtx bounds one git call by moondb.GitBudget and by what is left of
// the budget, whichever ends first.
func (rec *recorder) gitCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(rec.ctx, moondb.GitBudget)
}

// locate places dir in git; the zero Location when git cannot say in time.
// A placed directory is asked once per Run; a failure is asked again.
func (rec *recorder) locate(dir string) store.Location {
	loc, _ := rec.locateAnswered(dir)
	return loc
}

// locateAnswered is locate that also reports whether git answered: false
// when the zero Location comes from a deadline, not from dir being outside
// any repository.
func (rec *recorder) locateAnswered(dir string) (store.Location, bool) {
	if loc, ok := rec.locs[dir]; ok {
		return loc, true
	}
	ctx, cancel := rec.gitCtx()
	defer cancel()
	loc := Locate(ctx, dir)
	if loc.Worktree != "" {
		rec.locs[dir] = loc
		return loc, true
	}
	return loc, ctx.Err() == nil
}

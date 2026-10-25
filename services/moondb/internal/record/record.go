// Package record is the capture side of moondb: the logic behind `moondb
// record`, which a harness hook runs with one event payload on stdin. It
// turns the payload into spool envelopes and returns; serve does the rest.
// It copies belt's record contract (docs/adr/0024): nothing on stdout, exit
// 0 always, and a session that never waits long. Every git call is bounded
// by a deadline, and when the budget runs out an edit is written with what
// was gathered so far.
//
// Run keeps a little state per session under <workdir>/sessions, read back
// by later events of the same session and by nothing else:
//
//	<session>.turn     "<prompt_id> <offset>": the open turn and the
//	                   transcript size when it opened
//	<session>.paths    "<worktree>\t<path>" per line: the files the session
//	                   edited; a path's first edit ships its HEAD content
//	<session>.touched  one worktree per line: where the session edited
//	<session>.heads    "<worktree>\t<head>" per line, last line wins: the
//	                   HEAD last seen in each touched worktree
package record

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// harness names the agent runtime whose hook payloads Run reads.
const harness = "claude-code"

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
	ctx, cancel := context.WithTimeout(ctx, opts.Budget)
	defer cancel()
	rec := &recorder{
		ctx:  ctx,
		p:    p,
		opts: opts,
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
	return o
}

// recorder is one Run in progress. ctx ends when the budget does.
type recorder struct {
	ctx   context.Context
	p     payload
	opts  Options
	spool *spool.Writer
	state sessionState
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

// envelope starts an envelope of kind with what every payload carries.
func (rec *recorder) envelope(kind spool.Kind, id string) spool.Envelope {
	p := rec.p
	return spool.Envelope{
		Version: spool.Version,
		Kind:    kind,
		ID:      id,
		At:      rec.opts.Now(),
		Harness: harness,
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
func (rec *recorder) locate(dir string) store.Location {
	ctx, cancel := rec.gitCtx()
	defer cancel()
	l := gitloc.LocationAt(ctx, dir)
	return store.Location{
		Repo: l.Repo, Worktree: l.Toplevel, Canonical: l.Canonical, Branch: l.Branch, Head: l.Head,
	}
}

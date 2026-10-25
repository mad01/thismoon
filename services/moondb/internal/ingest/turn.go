package ingest

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// turnOpen records a submitted prompt. A turn the store already holds (a
// placeholder an early edit left, or the same envelope replayed) keeps its
// fields; the open only fills what is empty and moves StartedAt back to the
// prompt's time, never forward. An import moves it too: an imported prompt
// can predate the live edit that left the placeholder. A session without
// a prompt line takes the one its first prompt gives.
func (in *Ingester) turnOpen(
	ctx context.Context,
	env spool.Envelope,
	missing map[string]bool,
) error {
	if env.TurnOpen == nil {
		return errNoPayload(env)
	}
	if err := requireTurn(env); err != nil {
		return err
	}
	if err := in.upsertSession(ctx, env); err != nil {
		return err
	}
	prompt := in.kept(env, missing, env.TurnOpen.PromptChunk)
	if err := in.openTurn(ctx, env, prompt); err != nil {
		return err
	}
	return in.fillSession(ctx, env.Session.ID, titleOffer{})
}

// openTurn puts the turn a turn-open names, or fills the one the store
// holds. A closed turn whose start moved back gets its totals again, since
// its wall time grew.
func (in *Ingester) openTurn(ctx context.Context, env spool.Envelope, prompt string) error {
	td, found, err := in.loadTurn(ctx, env.Turn.ID)
	if err != nil {
		return err
	}
	if !found {
		t := newTurn(env, store.TurnOpen)
		t.PromptChunk = prompt
		return in.putTurn(ctx, t)
	}
	t := td.Turn
	changed := fill(&t.PromptChunk, prompt)
	if t.TranscriptOffset == 0 && env.Turn.TranscriptOffset != 0 {
		t.TranscriptOffset, changed = env.Turn.TranscriptOffset, true
	}
	changed = earliest(&t.StartedAt, env.At) || changed // a stored StartedAt is never empty
	if !changed {
		return nil
	}
	in.withTotals(ctx, &t)
	return in.putTurn(ctx, t)
}

// turnClose records the end of a turn: its last message, its transcript
// lines, its final status and the totals of its trace. A close whose open
// never arrived creates the turn, and the session with it, so the turn's
// records do not dangle. An import only fills: it closes a turn still open
// and sets fields the turn has empty. The session is offered the title the
// turn's lines give and, without a prompt line, the one its first prompt
// gives, even when the close changes nothing on the turn: a replayed close
// makes up a fill an earlier attempt lost to a failed write.
func (in *Ingester) turnClose(
	ctx context.Context,
	env spool.Envelope,
	missing map[string]bool,
) error {
	p := env.TurnClose
	if p == nil {
		return errNoPayload(env)
	}
	if err := requireTurn(env); err != nil {
		return err
	}
	if err := in.upsertSession(ctx, env); err != nil {
		return err
	}
	td, found, err := in.loadTurn(ctx, env.Turn.ID)
	if err != nil {
		return err
	}
	t := td.Turn
	if !found {
		t = newTurn(env, store.TurnClosed)
	}
	set := setter(env)
	changed := !found
	changed = set(&t.FinalChunk, in.kept(env, missing, p.FinalChunk)) || changed
	changed = set(&t.TranscriptChunk, in.kept(env, missing, p.TranscriptChunk)) || changed
	changed = closeTurn(&t, env, closedStatus(p.Status)) || changed
	lines, err := in.closedLines(ctx, &t, changed)
	if err != nil {
		return err
	}
	return in.fillSession(ctx, t.SessionID, transcriptTitle(lines))
}

// closedLines returns a closed turn's transcript lines. A changed turn gets
// its totals from them and is written; an unchanged one is only read, and
// a chunk it cannot read is logged and offers no title.
func (in *Ingester) closedLines(ctx context.Context, t *store.Turn, changed bool) ([]byte, error) {
	if changed {
		lines := in.withTotals(ctx, t)
		return lines, in.putTurn(ctx, *t)
	}
	lines, err := in.transcript(ctx, *t)
	if err != nil {
		in.logf("%v; offering its session no title from it", err)
	}
	return lines, nil
}

// closeTurn sets the turn's final status and end time and reports whether
// either changed. An import changes the status of an open turn only and
// sets an end time only where there is none.
func closeTurn(t *store.Turn, env spool.Envelope, status store.TurnStatus) bool {
	changed := false
	if t.Status != status && (!env.Backfill || t.Status == store.TurnOpen) {
		t.Status, changed = status, true
	}
	if t.EndedAt == nil || (!env.Backfill && !t.EndedAt.Equal(env.At)) {
		at := env.At
		t.EndedAt, changed = &at, true
	}
	return changed
}

// closedStatus is the status a close leaves: failed when the payload says
// so, closed for anything else, since a close cannot leave a turn open.
func closedStatus(s store.TurnStatus) store.TurnStatus {
	if s == store.TurnFailed {
		return store.TurnFailed
	}
	return store.TurnClosed
}

// newTurn is the turn an envelope implies when the store has none yet.
// Seq stays 0 so the store assigns the next one in the session.
func newTurn(env spool.Envelope, status store.TurnStatus) store.Turn {
	return store.Turn{
		ID:               env.Turn.ID,
		SessionID:        env.Session.ID,
		TranscriptOffset: env.Turn.TranscriptOffset,
		Status:           status,
		StartedAt:        env.At,
	}
}

func requireTurn(env spool.Envelope) error {
	if env.Turn.ID == "" {
		return fmt.Errorf("ingest: %s envelope %s has no turn id", env.Kind, env.ID)
	}
	return nil
}

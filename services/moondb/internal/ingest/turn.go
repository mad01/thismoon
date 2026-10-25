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
// prompt's time.
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
	changed = earliest(&t.StartedAt, env.At) || changed
	if !changed {
		return nil
	}
	return in.putTurn(ctx, t)
}

// turnClose records the end of a turn: its last message, its transcript
// lines and its final status. A close whose open never arrived creates the
// turn, and the session with it, so the turn's records do not dangle.
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
	changed := !found
	changed = replace(&t.FinalChunk, in.kept(env, missing, p.FinalChunk)) || changed
	changed = replace(&t.TranscriptChunk, in.kept(env, missing, p.TranscriptChunk)) || changed
	if status := closedStatus(p.Status); t.Status != status {
		t.Status, changed = status, true
	}
	if t.EndedAt == nil || !t.EndedAt.Equal(env.At) {
		at := env.At
		t.EndedAt, changed = &at, true
	}
	if !changed {
		return nil
	}
	return in.putTurn(ctx, t)
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

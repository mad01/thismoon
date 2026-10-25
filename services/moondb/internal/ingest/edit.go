package ingest

import (
	"context"
	"fmt"
	"slices"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// edit derives the Edit for an edit envelope and appends it, with the
// unattributed delta when the disk read disagreed. The delta goes first:
// a crash between the two appends leaves the edit unrecorded, and the
// replay derives it again from the same chain head instead of from itself.
// An edit already recorded under its turn is a replay and writes nothing.
func (in *Ingester) edit(ctx context.Context, env spool.Envelope) error {
	if env.Edit == nil {
		return errNoPayload(env)
	}
	if err := in.upsertSession(ctx, env); err != nil {
		return err
	}
	recorded, err := in.ensureTurn(ctx, env)
	if err != nil || recorded {
		return err
	}
	e, delta, err := in.deriver().Edit(ctx, env)
	if err != nil {
		return fmt.Errorf("ingest: edit %s: %w", env.ID, err)
	}
	if delta != nil {
		if delta.Ambiguous, err = in.ambiguous(ctx, delta.Worktree, env.Session.ID); err != nil {
			return err
		}
		if err := in.Store.AppendDelta(ctx, *delta); err != nil {
			return fmt.Errorf("ingest: append delta %s: %w", delta.ID, err)
		}
	}
	if err := in.Store.AppendEdit(ctx, e); err != nil {
		return fmt.Errorf("ingest: append edit %s: %w", e.ID, err)
	}
	return nil
}

// ensureTurn makes sure the edit's turn exists, putting an open
// placeholder when the edit arrived before its turn-open, and reports
// whether the turn already lists the edit. An envelope without a turn id
// has no turn to place and is never taken for a replay.
func (in *Ingester) ensureTurn(ctx context.Context, env spool.Envelope) (recorded bool, err error) {
	if env.Turn.ID == "" {
		return false, nil
	}
	td, found, err := in.loadTurn(ctx, env.Turn.ID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, in.putTurn(ctx, newTurn(env, store.TurnOpen))
	}
	id := editID(env.Edit)
	return slices.ContainsFunc(td.Edits, func(e store.Edit) bool { return e.ID == id }), nil
}

// editID is the id derive gives the Edit of a payload: the tool call plus
// the file's index within it.
func editID(p *spool.EditPayload) string {
	return fmt.Sprintf("%s:%d", p.ToolUseID, p.Index)
}

// ambiguous reports whether a session other than sessionID has an open
// turn in the worktree, so a change no tool call explains may be theirs.
// Files outside any repository share the worktree "", which says nothing
// about who else was working on them, so they are never ambiguous.
func (in *Ingester) ambiguous(ctx context.Context, worktree, sessionID string) (bool, error) {
	if worktree == "" {
		return false, nil
	}
	turns, err := in.Store.OpenTurns(ctx, worktree)
	if err != nil {
		return false, fmt.Errorf("ingest: open turns in %s: %w", worktree, err)
	}
	return slices.ContainsFunc(turns, func(t store.Turn) bool {
		return t.SessionID != sessionID
	}), nil
}

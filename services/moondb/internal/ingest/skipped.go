package ingest

import (
	"context"
	"slices"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// skipped puts the count of files a tool call changed and the hook skipped
// on the call's turn, so a turn whose Bash command pulled or
// checked out files says so instead of looking like it changed nothing. A
// turn the store does not hold yet gets an open placeholder, as for an
// edit. A call the turn already lists is a replay and changes nothing. An
// envelope without a turn id has no turn to put the count on, so it is
// logged and dropped.
func (in *Ingester) skipped(ctx context.Context, env spool.Envelope) error {
	if env.Skipped == nil {
		return errNoPayload(env)
	}
	if env.Turn.ID == "" {
		in.logf("ingest: skipped %s has no turn id; dropping its count", env.ID)
		return nil
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
		t = newTurn(env, store.TurnOpen)
	}
	s := *env.Skipped
	if slices.ContainsFunc(t.Skipped, func(k store.SkippedFiles) bool {
		return k.ToolUseID == s.ToolUseID
	}) {
		return nil
	}
	s.At = env.At
	t.Skipped = append(slices.Clone(t.Skipped), s)
	return in.putTurn(ctx, t)
}

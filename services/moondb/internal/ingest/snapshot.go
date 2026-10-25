package ingest

import (
	"context"
	"fmt"
	"slices"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// snapshot records a turn-boundary snapshot with the unattributed deltas it
// reveals. The deltas go first, as for an edit: a crash between the
// appends leaves the snapshot unrecorded, and the replay finds the same
// previous snapshot again instead of itself. A snapshot its turn already
// lists is a replay and writes nothing. A file whose chunk was lost keeps
// its entry without content.
func (in *Ingester) snapshot(
	ctx context.Context,
	env spool.Envelope,
	missing map[string]bool,
) error {
	if env.Snapshot == nil {
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
	if !found {
		if err := in.putTurn(ctx, newTurn(env, store.TurnOpen)); err != nil {
			return err
		}
	}
	if slices.ContainsFunc(td.Snapshots, func(s store.Snapshot) bool { return s.ID == env.ID }) {
		return nil
	}
	snap, deltas, err := in.deriver().Snapshot(ctx, in.withoutLost(env, missing))
	if err != nil {
		return fmt.Errorf("ingest: snapshot %s: %w", env.ID, err)
	}
	ambiguous, err := in.ambiguous(ctx, snap.Worktree, env.Session.ID)
	if err != nil {
		return err
	}
	for _, d := range deltas {
		d.Ambiguous = ambiguous
		if err := in.Store.AppendDelta(ctx, d); err != nil {
			return fmt.Errorf("ingest: append delta %s: %w", d.ID, err)
		}
	}
	if err := in.Store.AppendSnapshot(ctx, snap); err != nil {
		return fmt.Errorf("ingest: append snapshot %s: %w", snap.ID, err)
	}
	return nil
}

// withoutLost returns env with every file chunk and HEAD chunk that is in
// neither spool nor store cleared, so no record points at content the
// store does not have.
func (in *Ingester) withoutLost(env spool.Envelope, missing map[string]bool) spool.Envelope {
	if len(missing) == 0 {
		return env
	}
	p := *env.Snapshot
	p.Files = slices.Clone(p.Files)
	for i, f := range p.Files {
		p.Files[i].Chunk = in.kept(env, missing, f.Chunk)
		p.Files[i].HeadChunk = in.kept(env, missing, f.HeadChunk)
	}
	env.Snapshot = &p
	return env
}

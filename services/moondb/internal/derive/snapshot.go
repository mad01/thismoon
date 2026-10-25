package derive

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/spool"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Snapshot derives the Snapshot record for a snapshot envelope, storing its
// file map as one chunk, and the unattributed deltas it reveals: each file
// whose content differs from the latest content known of the path before
// the snapshot. That is the newer of two: the latest earlier edit's disk
// read, or its after-content when it has none, and the path in the
// worktree's previous snapshot, which for a path a whole previous snapshot
// lacked is HEAD's content. A formatter, a person, or a Bash command
// without a diff changed such a file between the two, and no recorded
// tool call explains it. Errors come from the chunk store and the store
// lookups only.
func (d *Deriver) Snapshot(
	ctx context.Context,
	env spool.Envelope,
) (store.Snapshot, []store.UnattributedDelta, error) {
	p := env.Snapshot
	if p == nil {
		return store.Snapshot{}, nil, fmt.Errorf(
			"derive: envelope %s (%s) has no snapshot payload", env.ID, env.Kind)
	}
	files := slices.Clone(p.Files)
	if files == nil {
		files = []store.SnapshotFile{}
	}
	slices.SortFunc(files, func(a, b store.SnapshotFile) int { return cmp.Compare(a.Path, b.Path) })
	raw, err := json.Marshal(files)
	if err != nil {
		return store.Snapshot{}, nil, fmt.Errorf("derive: encode file map of %s: %w", env.ID, err)
	}
	key, err := d.Chunks.Put(ctx, raw)
	if err != nil {
		return store.Snapshot{}, nil, fmt.Errorf("derive: store file map of %s: %w", env.ID, err)
	}
	snap := store.Snapshot{
		ID: env.ID, SessionID: env.Session.ID, TurnID: env.Turn.ID, Phase: p.Phase,
		Started: p.Started, At: env.At, Location: p.Location, Scope: p.Scope, MapChunk: key,
		FileCount: len(files), Truncated: p.Truncated, Skipped: p.Skipped,
	}
	deltas, err := d.snapshotDeltas(ctx, snap, files)
	if err != nil {
		return store.Snapshot{}, nil, err
	}
	return snap, deltas, nil
}

// snapshotDeltas compares every file the snapshot read, and every file it
// found deleted, with the latest content known of its path before.
func (d *Deriver) snapshotDeltas(
	ctx context.Context,
	snap store.Snapshot,
	files []store.SnapshotFile,
) ([]store.UnattributedDelta, error) {
	prev, prevFiles, err := d.snapshotAt(ctx, store.SnapshotQuery{
		Worktree: snap.Worktree, At: snap.At, ExcludeID: snap.ID,
	})
	if err != nil {
		return nil, err
	}
	var out []store.UnattributedDelta
	for _, f := range files {
		if f.Chunk == "" && !f.Deleted {
			continue
		}
		from, err := d.knownBefore(ctx, snap, f, prev, prevFiles)
		if err != nil {
			return nil, err
		}
		if from == "" || from == f.Chunk {
			continue
		}
		delta, err := d.snapshotDelta(ctx, snap, f, from)
		if err != nil {
			return nil, err
		}
		out = append(out, delta)
	}
	return out, nil
}

// knownBefore returns the chunk of the latest content known of f's path
// before snap, or "" when it is unknown: an edit landed while snap read,
// neither source has it, the newer source holds the path without content
// or as deleted, or an edit since left no after-content to compare with.
// A path a whole previous snapshot lacked was clean then, so it held the
// content HEAD had; snap ships that content, which counts when both
// snapshots sat on one HEAD.
func (d *Deriver) knownBefore(
	ctx context.Context,
	snap store.Snapshot,
	f store.SnapshotFile,
	prev *store.Snapshot,
	prevFiles map[string]store.SnapshotFile,
) (string, error) {
	edit, err := d.latestEdit(ctx, snap, f.Path)
	if err != nil {
		return "", err
	}
	if edit != nil && edit.At.After(began(snap)) {
		return "", nil // the read may have come before the edit or after it
	}
	prevFile, held := prevFiles[f.Path]
	var since time.Time
	var key string
	switch {
	case prev != nil && (edit == nil || began(*prev).After(edit.At)) && (held || !prev.Truncated):
		since, key = began(*prev), prevFile.Chunk
		if !held && prev.Head != "" && prev.Head == snap.Head {
			key = f.HeadChunk
		}
	case edit != nil:
		since, key = edit.At, cmp.Or(edit.ObservedChunk, edit.AfterChunk)
	}
	if key == "" {
		return "", nil
	}
	contentless, err := d.contentlessEditSince(ctx, snap, f.Path, since)
	if err != nil || contentless {
		return "", err
	}
	return key, nil
}

// began is when snap's capture started: Started, or At when it has none.
// Only an edit stamped before it is surely in what snap read.
func began(snap store.Snapshot) time.Time {
	if snap.Started.IsZero() {
		return snap.At
	}
	return snap.Started
}

// latestEdit is the latest edit of path at or before snap that has
// after-content; nil when there is none.
func (d *Deriver) latestEdit(
	ctx context.Context,
	snap store.Snapshot,
	path string,
) (*store.Edit, error) {
	if d.LatestAfter == nil {
		return nil, nil
	}
	e, err := d.LatestAfter(ctx, store.ChainQuery{Worktree: snap.Worktree, Path: path, At: snap.At})
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("derive: latest edit of %s: %w", path, err)
	}
	return &e, nil
}

// contentlessEditSince reports an edit of path after since and at or
// before snap that has no after-content: a recorded tool call changed the
// file in a way moondb could not derive, so a change found since is not
// unattributed.
func (d *Deriver) contentlessEditSince(
	ctx context.Context,
	snap store.Snapshot,
	path string,
	since time.Time,
) (bool, error) {
	if d.Edits == nil {
		return false, nil
	}
	edits, err := d.Edits(ctx, store.EditQuery{Worktree: snap.Worktree, Path: path, Since: since})
	if err != nil {
		return false, fmt.Errorf("derive: edits of %s: %w", path, err)
	}
	return slices.ContainsFunc(edits, func(e store.Edit) bool {
		return e.AfterChunk == "" && e.At.After(since) && !e.At.After(snap.At)
	}), nil
}

// snapshotDelta records how f, as the snapshot found it, differs from the
// content in from. A deleted file differs by every line.
func (d *Deriver) snapshotDelta(
	ctx context.Context,
	snap store.Snapshot,
	f store.SnapshotFile,
	from string,
) (store.UnattributedDelta, error) {
	old, err := d.get(ctx, from)
	if err != nil {
		return store.UnattributedDelta{}, err
	}
	cur, err := d.get(ctx, f.Chunk)
	if err != nil {
		return store.UnattributedDelta{}, err
	}
	key, err := d.putDiff(ctx, f.Path, old.data, cur.data)
	if err != nil {
		return store.UnattributedDelta{}, err
	}
	return store.UnattributedDelta{
		ID: snap.ID + ":" + f.Path, SessionID: snap.SessionID, TurnID: snap.TurnID, At: snap.At,
		Location: snap.Location, Path: f.Path, FromChunk: from, ToChunk: f.Chunk,
		PatchChunk: key, Cause: store.DeltaSnapshot,
	}, nil
}

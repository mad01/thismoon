package derive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// history is what the store knew of an edit's path before the edit: the
// latest earlier edit with after-content, and the worktree's latest
// snapshot with its entry for the path. recall loads it once, on first
// use.
type history struct {
	loaded bool
	edit   *store.Edit
	snap   *store.Snapshot
	// file is the snapshot's entry for the path; nil when it has none.
	file *store.SnapshotFile
}

// chainStale reports a snapshot taken after the chain's edit that speaks
// for the path: it holds the path, or it is whole and the path was clean.
// Either way the edit's after-content is no longer what the file held.
func (h *history) chainStale() bool {
	return h.snap != nil && h.edit != nil && h.snap.At.After(h.edit.At) &&
		(h.file != nil || !h.snap.Truncated)
}

// recall loads h for e unless it is loaded already.
func (d *Deriver) recall(ctx context.Context, e store.Edit, h *history) error {
	if h.loaded {
		return nil
	}
	h.loaded = true
	if d.LatestAfter != nil {
		prev, err := d.LatestAfter(ctx, store.ChainQuery{
			Worktree: e.Worktree, Path: e.Path, At: e.At, ExcludeID: e.ID,
		})
		switch {
		case err == nil:
			h.edit = &prev
		case !errors.Is(err, store.ErrNotFound):
			return fmt.Errorf("derive: latest edit of %s: %w", e.Path, err)
		}
	}
	snap, files, err := d.snapshotAt(ctx, store.SnapshotQuery{Worktree: e.Worktree, At: e.At})
	if err != nil || snap == nil {
		return err
	}
	h.snap = snap
	if f, ok := files[e.Path]; ok {
		h.file = &f
	}
	return nil
}

// snapshotAt returns the snapshot q asks for with its file map by path,
// or nil when there is none. A file outside any worktree has none.
func (d *Deriver) snapshotAt(
	ctx context.Context,
	q store.SnapshotQuery,
) (*store.Snapshot, map[string]store.SnapshotFile, error) {
	if d.LatestSnapshot == nil || q.Worktree == "" {
		return nil, nil, nil
	}
	snap, err := d.LatestSnapshot(ctx, q)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("derive: latest snapshot of %s: %w", q.Worktree, err)
	}
	files, err := d.snapshotFiles(ctx, snap)
	if err != nil {
		return nil, nil, err
	}
	return &snap, files, nil
}

// snapshotFiles reads a snapshot's file map, keyed by path.
func (d *Deriver) snapshotFiles(
	ctx context.Context,
	snap store.Snapshot,
) (map[string]store.SnapshotFile, error) {
	raw, err := d.get(ctx, snap.MapChunk)
	if err != nil || !raw.ok {
		return nil, err
	}
	var list []store.SnapshotFile
	if err := json.Unmarshal(raw.data, &list); err != nil {
		return nil, fmt.Errorf("derive: file map of snapshot %s: %w", snap.ID, err)
	}
	files := make(map[string]store.SnapshotFile, len(list))
	for _, f := range list {
		files[f.Path] = f
	}
	return files, nil
}

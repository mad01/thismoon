package local

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// The read side of the index. Every method here only reads, so the store
// serves them under a read lock, and every listing is a fresh slice.

// sessionList walks every session newest first by start: Since selects by
// last activity, which a session started long ago can still have, so no
// start time ends the walk early.
func (ix *index) sessionList(f store.SessionFilter) []store.SessionSummary {
	out := []store.SessionSummary{}
	for i := len(ix.sessionOrder) - 1; i >= 0; i-- {
		s := ix.sessionOrder[i]
		if _, ok := ix.sessionRepos[s.ID][f.Repo]; f.Repo != "" && !ok {
			continue // cheaper than a summary, and f.Keeps would drop it
		}
		sum := ix.summary(s)
		if !f.Keeps(sum) {
			continue
		}
		out = append(out, sum)
		if f.Limit > 0 && len(out) == f.Limit {
			break
		}
	}
	return out
}

func (ix *index) sessionDetail(id string) (store.SessionDetail, error) {
	s, ok := ix.sessions[id]
	if !ok {
		return store.SessionDetail{}, fmt.Errorf("session %s: %w", id, store.ErrNotFound)
	}
	sum := ix.summary(s)
	return store.SessionDetail{SessionSummary: sum, Turns: ix.sessionTurns(id)}, nil
}

// summary adds up a session's records in the index; it reads no chunk.
func (ix *index) summary(s *store.Session) store.SessionSummary {
	edits := ix.editsBySession[s.ID]
	return store.Summarize(*s, ix.sessionTurns(s.ID), func(yield func(*store.Edit) bool) {
		for _, r := range edits {
			if !yield(&r.v) {
				return
			}
		}
	})
}

// sessionTurns copies a session's turns out in Seq order.
func (ix *index) sessionTurns(id string) []store.Turn {
	turns := ix.turnsBySession[id]
	out := make([]store.Turn, 0, len(turns))
	for _, t := range turns {
		out = append(out, *t)
	}
	return out
}

// turnsWithoutTotals scans every turn: the totals backfill asks once a
// tick, and a scan of the in-memory turns costs less than an index kept in
// step with every put.
func (ix *index) turnsWithoutTotals(version, limit int) []store.Turn {
	out := []store.Turn{}
	for _, t := range ix.turns {
		if store.NeedsTotals(*t, version) {
			out = append(out, *t)
		}
	}
	store.SortForTotals(out, func(id string) time.Time {
		if s, ok := ix.sessions[id]; ok {
			return s.StartedAt
		}
		return time.Time{}
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func (ix *index) turnDetail(id string) (store.TurnDetail, error) {
	t, ok := ix.turns[id]
	if !ok {
		return store.TurnDetail{}, fmt.Errorf("turn %s: %w", id, store.ErrNotFound)
	}
	return store.TurnDetail{
		Turn:        *t,
		Edits:       newestFirst(ix.edits.byTurn[id]),
		Deltas:      newestFirst(ix.deltas.byTurn[id]),
		Snapshots:   newestFirst(ix.snapshots.byTurn[id]),
		CommitLinks: newestFirst(ix.links.byTurn[id]),
	}, nil
}

func (ix *index) editList(q store.EditQuery) []store.Edit {
	list := ix.editCandidates(q)
	out := []store.Edit{}
	for i := len(list) - 1; i >= 0; i-- {
		r := list[i]
		if r.at.Before(q.Since) {
			break
		}
		if !editMatches(q, &r.v) {
			continue
		}
		out = append(out, r.v)
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out
}

// editCandidates picks the shortest index list that holds every edit q can
// match; editMatches then applies the whole query to it.
func (ix *index) editCandidates(q store.EditQuery) []*rec[store.Edit] {
	best := ix.allEdits
	narrow := func(set bool, list []*rec[store.Edit]) {
		if set && len(list) < len(best) {
			best = list
		}
	}
	narrow(q.TurnID != "", ix.edits.byTurn[q.TurnID])
	narrow(q.SessionID != "", ix.editsBySession[q.SessionID])
	narrow(q.AfterBlob != "", ix.editsByBlob[q.AfterBlob])
	narrow(q.Repo != "" && q.Path != "", ix.editsByRepoPath[pairKey(q.Repo, q.Path)])
	narrow(q.Worktree != "" && q.Path != "", ix.editsByWorktreePath[pairKey(q.Worktree, q.Path)])
	return best
}

func editMatches(q store.EditQuery, e *store.Edit) bool {
	return (q.Repo == "" || e.Repo == q.Repo) &&
		(q.Path == "" || e.Path == q.Path) &&
		(q.Worktree == "" || e.Worktree == q.Worktree) &&
		(q.SessionID == "" || e.SessionID == q.SessionID) &&
		(q.TurnID == "" || e.TurnID == q.TurnID) &&
		(q.AfterBlob == "" || e.AfterBlob == q.AfterBlob)
}

// latestAfter walks the path's edits newest first, by At and then log
// position, past those q rules out.
func (ix *index) latestAfter(q store.ChainQuery) (store.Edit, error) {
	list := ix.editsByWorktreePath[pairKey(q.Worktree, q.Path)]
	for i := len(list) - 1; i >= 0; i-- {
		if e := list[i].v; chainHead(q, e) {
			return e, nil
		}
	}
	return store.Edit{}, fmt.Errorf(
		"after-content of %s in %q: %w", q.Path, q.Worktree, store.ErrNotFound)
}

// chainHead reports whether e, an edit of q's worktree and path, can head
// the chain q asks for.
func chainHead(q store.ChainQuery, e store.Edit) bool {
	return e.AfterChunk != "" && e.ID != q.ExcludeID && (q.At.IsZero() || !e.At.After(q.At))
}

// latestSnapshot walks the worktree's snapshots newest first, by At and
// then log position, past those q rules out.
func (ix *index) latestSnapshot(q store.SnapshotQuery) (store.Snapshot, error) {
	list := ix.snapshotsByWorktree[q.Worktree]
	for i := len(list) - 1; i >= 0; i-- {
		s := list[i].v
		if s.ID != q.ExcludeID && (q.At.IsZero() || !s.At.After(q.At)) {
			return s, nil
		}
	}
	return store.Snapshot{}, fmt.Errorf("snapshot of %q: %w", q.Worktree, store.ErrNotFound)
}

func (ix *index) openTurns(worktree string) []store.Turn {
	out := []store.Turn{}
	for id := range ix.openByWorktree[worktree] {
		if t, ok := ix.turns[id]; ok {
			out = append(out, *t)
		}
	}
	slices.SortFunc(out, func(a, b store.Turn) int {
		if c := b.StartedAt.Compare(a.StartedAt); c != 0 {
			return c
		}
		return cmp.Compare(b.ID, a.ID)
	})
	return out
}

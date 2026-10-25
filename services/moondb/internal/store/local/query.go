package local

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// The read side of the index. Every method here only reads, so the store
// serves them under a read lock, and every listing is a fresh slice.

func (ix *index) sessionList(f store.SessionFilter) []store.Session {
	out := []store.Session{}
	for i := len(ix.sessionOrder) - 1; i >= 0; i-- {
		s := ix.sessionOrder[i]
		if s.StartedAt.Before(f.Since) {
			break
		}
		if _, ok := ix.sessionRepos[s.ID][f.Repo]; f.Repo != "" && !ok {
			continue
		}
		out = append(out, *s)
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
	turns := ix.turnsBySession[id]
	d := store.SessionDetail{
		Session:   *s,
		Turns:     make([]store.Turn, 0, len(turns)),
		Worktrees: ix.sessionWorktrees[id].sorted(),
		EditCount: len(ix.editsBySession[id]),
	}
	for _, t := range turns {
		d.Turns = append(d.Turns, *t)
	}
	return d, nil
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

func (ix *index) latestAfter(worktree, path string) (store.Edit, error) {
	list := ix.editsByWorktreePath[pairKey(worktree, path)]
	for i := len(list) - 1; i >= 0; i-- {
		if list[i].v.AfterChunk != "" {
			return list[i].v, nil
		}
	}
	return store.Edit{}, fmt.Errorf(
		"after-content of %s in %q: %w", path, worktree, store.ErrNotFound)
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

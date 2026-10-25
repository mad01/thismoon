package local

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Record kinds, the "t" of a log line.
const (
	kindSession    = "session"
	kindTurn       = "turn"
	kindEdit       = "edit"
	kindDelta      = "delta"
	kindSnapshot   = "snapshot"
	kindCommitLink = "commitlink"
)

// errUnknownKind marks a log line of a kind this build does not know, left
// by a newer build. Replay skips such a line rather than refuse to open.
var errUnknownKind = errors.New("local: unknown record kind")

// logLine is one line of the log: a record and its kind.
type logLine struct {
	T string          `json:"t"`
	R json.RawMessage `json:"r"`
}

// index is the in-memory view of the log that every read is served from.
// It is rebuilt from the log on open and changed only through apply, so a
// reopened store holds exactly what the live one did. Every list of records
// is kept oldest first.
type index struct {
	// n counts applied lines; it orders records whose times are equal.
	n uint64

	sessions map[string]*store.Session
	// sessionOrder holds every session by StartedAt, then ID.
	sessionOrder []*store.Session
	turns        map[string]*store.Turn
	// turnsBySession holds each session's turns by Seq.
	turnsBySession map[string][]*store.Turn

	edits     facts[store.Edit]
	deltas    facts[store.UnattributedDelta]
	snapshots facts[store.Snapshot]
	links     facts[store.CommitLink]

	allEdits            []*rec[store.Edit]
	editsBySession      map[string][]*rec[store.Edit]
	editsByRepoPath     map[string][]*rec[store.Edit]
	editsByWorktreePath map[string][]*rec[store.Edit]
	editsByBlob         map[string][]*rec[store.Edit]
	snapshotsByWorktree map[string][]*rec[store.Snapshot]

	sessionRepos map[string]set
	// turnWorktrees holds the worktrees each turn has an edit in, and
	// openByWorktree the open turns among them, keyed by worktree.
	turnWorktrees  map[string]set
	openByWorktree map[string]set
}

func newIndex() *index {
	return &index{
		sessions:            map[string]*store.Session{},
		turns:               map[string]*store.Turn{},
		turnsBySession:      map[string][]*store.Turn{},
		edits:               newFacts[store.Edit](),
		deltas:              newFacts[store.UnattributedDelta](),
		snapshots:           newFacts[store.Snapshot](),
		links:               newFacts[store.CommitLink](),
		editsBySession:      map[string][]*rec[store.Edit]{},
		editsByRepoPath:     map[string][]*rec[store.Edit]{},
		editsByWorktreePath: map[string][]*rec[store.Edit]{},
		editsByBlob:         map[string][]*rec[store.Edit]{},
		snapshotsByWorktree: map[string][]*rec[store.Snapshot]{},
		sessionRepos:        map[string]set{},
		turnWorktrees:       map[string]set{},
		openByWorktree:      map[string]set{},
	}
}

// apply folds one record into the index. Replay and the write path both
// go through it.
func (ix *index) apply(kind string, raw json.RawMessage) error {
	ix.n++
	switch kind {
	case kindSession:
		return decodeInto(raw, ix.putSession)
	case kindTurn:
		return decodeInto(raw, ix.putTurn)
	case kindEdit:
		return decodeInto(raw, ix.addEdit)
	case kindDelta:
		return decodeInto(raw, func(d store.UnattributedDelta) {
			ix.deltas.add(d.ID, d.TurnID, newRec(d, d.At, ix.n))
		})
	case kindSnapshot:
		return decodeInto(raw, ix.addSnapshot)
	case kindCommitLink:
		return decodeInto(raw, func(l store.CommitLink) {
			ix.links.add(l.ID, l.TurnID, newRec(l, l.At, ix.n))
		})
	}
	return fmt.Errorf("%w %q", errUnknownKind, kind)
}

func decodeInto[T any](raw json.RawMessage, fn func(T)) error {
	var v T
	if err := json.Unmarshal(raw, &v); err != nil {
		return fmt.Errorf("decode %T: %w", v, err)
	}
	fn(v)
	return nil
}

// putSession replaces the session with the same id, or adds it.
func (ix *index) putSession(s store.Session) {
	cur, ok := ix.sessions[s.ID]
	if ok {
		ix.sessionOrder = removeSorted(ix.sessionOrder, cur, cmpSession)
		*cur = s
	} else {
		cur = &s
		ix.sessions[s.ID] = cur
	}
	ix.sessionOrder = insertSorted(ix.sessionOrder, cur, cmpSession)
}

// settledTurn applies the Seq rule to an incoming turn: a known turn keeps
// the session and Seq it was first recorded with, and a new turn without a
// Seq takes the next one in its session.
func (ix *index) settledTurn(t store.Turn) store.Turn {
	if cur, ok := ix.turns[t.ID]; ok {
		t.SessionID, t.Seq = cur.SessionID, cur.Seq
		return t
	}
	if t.Seq == 0 {
		t.Seq = 1
		if ts := ix.turnsBySession[t.SessionID]; len(ts) > 0 {
			t.Seq = ts[len(ts)-1].Seq + 1
		}
	}
	return t
}

// putTurn replaces the turn with the same id, or adds it, and keeps the
// open-turn index in step with its status.
func (ix *index) putTurn(t store.Turn) {
	t = ix.settledTurn(t)
	cur, ok := ix.turns[t.ID]
	if ok {
		*cur = t
	} else {
		cur = &t
		ix.turns[t.ID] = cur
		ix.turnsBySession[t.SessionID] = insertSorted(
			ix.turnsBySession[t.SessionID], cur, cmpTurnSeq)
	}
	for wt := range ix.turnWorktrees[t.ID] {
		if cur.Status == store.TurnOpen {
			addTo(ix.openByWorktree, wt, t.ID)
		} else {
			removeFrom(ix.openByWorktree, wt, t.ID)
		}
	}
}

// addEdit records e under every key a query can narrow by, unless an edit
// with its id is already recorded. An edit may arrive before its turn; the
// turn joins the open-turn index when it is put.
func (ix *index) addEdit(e store.Edit) {
	r := newRec(e, e.At, ix.n)
	if !ix.edits.add(e.ID, e.TurnID, r) {
		return
	}
	ix.allEdits = insertRec(ix.allEdits, r)
	indexRec(ix.editsBySession, e.SessionID, r)
	indexRec(ix.editsByRepoPath, pairKey(e.Repo, e.Path), r)
	indexRec(ix.editsByWorktreePath, pairKey(e.Worktree, e.Path), r)
	if e.AfterBlob != "" {
		indexRec(ix.editsByBlob, e.AfterBlob, r)
	}
	if e.Repo != "" {
		addTo(ix.sessionRepos, e.SessionID, e.Repo)
	}
	addTo(ix.turnWorktrees, e.TurnID, e.Worktree)
	if t, ok := ix.turns[e.TurnID]; ok && t.Status == store.TurnOpen {
		addTo(ix.openByWorktree, e.Worktree, e.TurnID)
	}
}

// addSnapshot records s under its turn and its worktree, unless a
// snapshot with its id is already recorded.
func (ix *index) addSnapshot(s store.Snapshot) {
	r := newRec(s, s.At, ix.n)
	if ix.snapshots.add(s.ID, s.TurnID, r) {
		indexRec(ix.snapshotsByWorktree, s.Worktree, r)
	}
}

func cmpSession(a, b *store.Session) int {
	if c := a.StartedAt.Compare(b.StartedAt); c != 0 {
		return c
	}
	return cmp.Compare(a.ID, b.ID)
}

func cmpTurnSeq(a, b *store.Turn) int {
	if c := cmp.Compare(a.Seq, b.Seq); c != 0 {
		return c
	}
	return cmp.Compare(a.ID, b.ID)
}

func insertSorted[T any](list []T, v T, cmp func(a, b T) int) []T {
	i, _ := slices.BinarySearchFunc(list, v, cmp)
	return slices.Insert(list, i, v)
}

func removeSorted[T any](list []T, v T, cmp func(a, b T) int) []T {
	if i, ok := slices.BinarySearchFunc(list, v, cmp); ok {
		return slices.Delete(list, i, i+1)
	}
	return list
}

// pairKey joins two strings into one map key; NUL appears in neither a
// path nor a repo identity.
func pairKey(a, b string) string { return a + "\x00" + b }

// rec is an append-only record in an index list, with the time it sorts by
// and its position in the log, which breaks ties between equal times.
type rec[T any] struct {
	v  T
	at time.Time
	n  uint64
}

func newRec[T any](v T, at time.Time, n uint64) *rec[T] {
	return &rec[T]{v: v, at: at, n: n}
}

func cmpRec[T any](a, b *rec[T]) int {
	if c := a.at.Compare(b.at); c != 0 {
		return c
	}
	return cmp.Compare(a.n, b.n)
}

func insertRec[T any](list []*rec[T], r *rec[T]) []*rec[T] {
	return insertSorted(list, r, cmpRec[T])
}

func indexRec[T any](m map[string][]*rec[T], key string, r *rec[T]) {
	m[key] = insertRec(m[key], r)
}

// newestFirst copies the records of a list out, newest first.
func newestFirst[T any](list []*rec[T]) []T {
	out := make([]T, 0, len(list))
	for i := len(list) - 1; i >= 0; i-- {
		out = append(out, list[i].v)
	}
	return out
}

// facts indexes one kind of append-only record: the ids already recorded,
// which make an append idempotent, and the records under each turn.
type facts[T any] struct {
	ids    map[string]struct{}
	byTurn map[string][]*rec[T]
}

func newFacts[T any]() facts[T] {
	return facts[T]{ids: map[string]struct{}{}, byTurn: map[string][]*rec[T]{}}
}

func (f *facts[T]) has(id string) bool {
	_, ok := f.ids[id]
	return ok
}

// add records r under turn unless id is already recorded, and reports
// whether it did. The first record with an id wins.
func (f *facts[T]) add(id, turn string, r *rec[T]) bool {
	if f.has(id) {
		return false
	}
	f.ids[id] = struct{}{}
	indexRec(f.byTurn, turn, r)
	return true
}

// set is a set of strings.
type set map[string]struct{}

func addTo(m map[string]set, key, v string) {
	s, ok := m[key]
	if !ok {
		s = set{}
		m[key] = s
	}
	s[v] = struct{}{}
}

func removeFrom(m map[string]set, key, v string) {
	delete(m[key], v)
	if len(m[key]) == 0 {
		delete(m, key)
	}
}

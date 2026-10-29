// Package faketest is an in-memory store.Store for the ingest and query
// tests. It keeps the contract the local backend keeps: Put upserts with
// the newest record winning whole, a known turn keeps its session and Seq,
// Append is idempotent on the id, and listings come newest first. It keeps
// nothing on disk and is safe for concurrent use.
package faketest

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Store is the in-memory store. The zero value is not usable; call New.
type Store struct {
	mu        sync.Mutex
	sessions  map[string]store.Session
	turns     map[string]store.Turn
	agents    map[string]store.Agent
	edits     []store.Edit
	deltas    []store.UnattributedDelta
	snapshots []store.Snapshot
	links     []store.CommitLink
	chunks    *Chunks
	writes    int
}

// New returns an empty Store.
func New() *Store {
	return &Store{
		sessions: map[string]store.Session{},
		turns:    map[string]store.Turn{},
		agents:   map[string]store.Agent{},
		chunks:   &Chunks{data: map[string][]byte{}},
	}
}

// Writes counts the calls that wrote a record: every Put, and every Append
// of an id not recorded yet. A replay that changes nothing leaves it alone.
func (s *Store) Writes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writes
}

// PutSession upserts a session.
func (s *Store) PutSession(_ context.Context, v store.Session) error {
	if v.ID == "" {
		return errors.New("faketest: session has no id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[v.ID] = v
	s.writes++
	return nil
}

// PutTurn upserts a turn. A new turn with Seq 0 takes the next Seq in its
// session; a known turn keeps its Seq and cannot move to another session.
func (s *Store) PutTurn(_ context.Context, t store.Turn) error {
	if t.ID == "" {
		return errors.New("faketest: turn has no id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.turns[t.ID]; ok {
		if cur.SessionID != t.SessionID {
			return fmt.Errorf("faketest: turn %s belongs to session %s, not %s",
				t.ID, cur.SessionID, t.SessionID)
		}
		t.Seq = cur.Seq
	} else if t.Seq == 0 {
		for _, o := range s.turns {
			if o.SessionID == t.SessionID {
				t.Seq = max(t.Seq, o.Seq)
			}
		}
		t.Seq++
	}
	s.turns[t.ID] = t
	s.writes++
	return nil
}

// PutAgent upserts an agent; a known agent cannot move to another
// session.
func (s *Store) PutAgent(_ context.Context, a store.Agent) error {
	if a.ID == "" {
		return errors.New("faketest: agent has no id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.agents[a.ID]; ok && cur.SessionID != a.SessionID {
		return fmt.Errorf("faketest: agent %s belongs to session %s, not %s",
			a.ID, cur.SessionID, a.SessionID)
	}
	s.agents[a.ID] = a
	s.writes++
	return nil
}

// Agents lists a session's agents in store.SortAgents order.
func (s *Store) Agents(_ context.Context, sessionID string) ([]store.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessionAgents(sessionID), nil
}

// Agent loads one agent.
func (s *Store) Agent(_ context.Context, id string) (store.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	a, ok := s.agents[id]
	if !ok {
		return store.Agent{}, fmt.Errorf("faketest: agent %s: %w", id, store.ErrNotFound)
	}
	return a, nil
}

// AgentsWithoutTotals lists up to n agents whose totals are missing or
// older than version, newest session first.
func (s *Store) AgentsWithoutTotals(_ context.Context, version, n int) ([]store.Agent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Agent
	for _, a := range s.agents {
		if store.NeedsAgentTotals(a, version) {
			out = append(out, a)
		}
	}
	slices.SortFunc(out, func(a, b store.Agent) int {
		return cmp.Or(
			s.sessions[b.SessionID].StartedAt.Compare(s.sessions[a.SessionID].StartedAt),
			cmp.Compare(b.SessionID, a.SessionID),
			a.StartedAt.Compare(b.StartedAt),
			cmp.Compare(a.ID, b.ID),
		)
	})
	return limit(out, n), nil
}

// CountAgentsToFill counts the agents store.NeedsFill keeps for
// since.
func (s *Store) CountAgentsToFill(_ context.Context, since time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for _, a := range s.agents {
		if store.NeedsFill(a, since) {
			n++
		}
	}
	return n, nil
}

// sessionAgents lists one session's agents; the caller holds mu.
func (s *Store) sessionAgents(id string) []store.Agent {
	out := []store.Agent{}
	for _, a := range s.agents {
		if a.SessionID == id {
			out = append(out, a)
		}
	}
	store.SortAgents(out)
	return out
}

// AppendEdit records an edit unless its id is known.
func (s *Store) AppendEdit(_ context.Context, e store.Edit) error {
	return appendNew(s, &s.edits, e, func(x store.Edit) string { return x.ID })
}

// AppendDelta records an unattributed delta unless its id is known.
func (s *Store) AppendDelta(_ context.Context, d store.UnattributedDelta) error {
	return appendNew(s, &s.deltas, d, func(x store.UnattributedDelta) string { return x.ID })
}

// AppendSnapshot records a snapshot unless its id is known.
func (s *Store) AppendSnapshot(_ context.Context, v store.Snapshot) error {
	return appendNew(s, &s.snapshots, v, func(x store.Snapshot) string { return x.ID })
}

// AppendCommitLink records a commit link unless its id is known.
func (s *Store) AppendCommitLink(_ context.Context, l store.CommitLink) error {
	return appendNew(s, &s.links, l, func(x store.CommitLink) string { return x.ID })
}

// appendNew adds v to list unless a record with its id is in it already.
func appendNew[T any](s *Store, list *[]T, v T, id func(T) string) error {
	if id(v) == "" {
		return errors.New("faketest: record has no id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if slices.ContainsFunc(*list, func(x T) bool { return id(x) == id(v) }) {
		return nil
	}
	*list = append(*list, v)
	s.writes++
	return nil
}

// Sessions lists sessions most recently active first, each summarized and
// kept when f keeps it.
func (s *Store) Sessions(_ context.Context, f store.SessionFilter) ([]store.SessionSummary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []store.SessionSummary{}
	for _, v := range s.sessions {
		if sum := s.summary(v); f.Keeps(sum) {
			out = append(out, sum)
		}
	}
	store.SortSummaries(out)
	return limit(out, f.Limit), nil
}

// Session loads a session with its summary and its turns in Seq order.
func (s *Store) Session(_ context.Context, id string) (store.SessionDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.sessions[id]
	if !ok {
		return store.SessionDetail{}, fmt.Errorf("faketest: session %s: %w", id, store.ErrNotFound)
	}
	return store.SessionDetail{
		SessionSummary: s.summary(v),
		Turns:          s.sessionTurns(id),
		Agents:         s.sessionAgents(id),
	}, nil
}

// TurnsWithoutTotals lists up to limit closed or failed turns whose totals
// are missing or older than version, in store.SortForTotals order.
func (s *Store) TurnsWithoutTotals(_ context.Context, version, n int) ([]store.Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Turn
	for _, t := range s.turns {
		if store.NeedsTotals(t, version) {
			out = append(out, t)
		}
	}
	store.SortForTotals(out, func(id string) time.Time { return s.sessions[id].StartedAt })
	return limit(out, n), nil
}

// summary adds up one session's records; the caller holds mu.
func (s *Store) summary(v store.Session) store.SessionSummary {
	turns, agents := s.sessionTurns(v.ID), s.sessionAgents(v.ID)
	return store.Summarize(v, turns, agents, func(yield func(*store.Edit) bool) {
		for i := range s.edits {
			if s.edits[i].SessionID == v.ID && !yield(&s.edits[i]) {
				return
			}
		}
	})
}

// sessionTurns lists one session's turns in Seq order; the caller holds mu.
func (s *Store) sessionTurns(id string) []store.Turn {
	var out []store.Turn
	for _, t := range s.turns {
		if t.SessionID == id {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b store.Turn) int { return cmp.Compare(a.Seq, b.Seq) })
	return out
}

// Turn loads a turn with what was recorded under it, each newest first.
func (s *Store) Turn(_ context.Context, id string) (store.TurnDetail, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.turns[id]
	if !ok {
		return store.TurnDetail{}, fmt.Errorf("faketest: turn %s: %w", id, store.ErrNotFound)
	}
	return store.TurnDetail{
		Turn: t,
		Edits: newest(
			s.edits,
			func(e store.Edit) (bool, time.Time) { return e.TurnID == id, e.At },
		),
		Deltas: newest(
			s.deltas,
			func(d store.UnattributedDelta) (bool, time.Time) { return d.TurnID == id, d.At },
		),
		Snapshots: newest(
			s.snapshots,
			func(v store.Snapshot) (bool, time.Time) { return v.TurnID == id, v.At },
		),
		CommitLinks: newest(
			s.links,
			func(l store.CommitLink) (bool, time.Time) { return l.TurnID == id, l.At },
		),
	}, nil
}

// Edits lists the edits matching every set field of q, newest first.
func (s *Store) Edits(_ context.Context, q store.EditQuery) ([]store.Edit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := newest(s.edits, func(e store.Edit) (bool, time.Time) { return matches(e, q), e.At })
	return limit(out, q.Limit), nil
}

func matches(e store.Edit, q store.EditQuery) bool {
	return field(q.Repo, e.Repo) && field(q.Path, e.Path) && field(q.Worktree, e.Worktree) &&
		field(q.SessionID, e.SessionID) && field(q.TurnID, e.TurnID) &&
		field(q.AfterBlob, e.AfterBlob) && !e.At.Before(q.Since)
}

func field(want, got string) bool { return want == "" || want == got }

// LatestAfter returns the newest edit q allows with an AfterChunk.
func (s *Store) LatestAfter(_ context.Context, q store.ChainQuery) (store.Edit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := newest(s.edits, func(e store.Edit) (bool, time.Time) {
		return e.Worktree == q.Worktree && e.Path == q.Path && e.AfterChunk != "" &&
			e.ID != q.ExcludeID && (q.At.IsZero() || !e.At.After(q.At)), e.At
	})
	if len(out) == 0 {
		return store.Edit{}, fmt.Errorf("faketest: latest after %s: %w", q.Path, store.ErrNotFound)
	}
	return out[0], nil
}

// LatestSnapshot returns the newest snapshot q allows.
func (s *Store) LatestSnapshot(_ context.Context, q store.SnapshotQuery) (store.Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := newest(s.snapshots, func(v store.Snapshot) (bool, time.Time) {
		return v.Worktree == q.Worktree && v.ID != q.ExcludeID &&
			(q.At.IsZero() || !v.At.After(q.At)), v.At
	})
	if len(out) == 0 {
		return store.Snapshot{}, fmt.Errorf(
			"faketest: snapshot of %s: %w", q.Worktree, store.ErrNotFound)
	}
	return out[0], nil
}

// OpenTurns lists the open turns with an edit in worktree, newest first by
// StartedAt.
func (s *Store) OpenTurns(_ context.Context, worktree string) ([]store.Turn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Turn
	for _, t := range s.turns {
		if t.Status == store.TurnOpen && slices.ContainsFunc(s.edits, func(e store.Edit) bool {
			return e.TurnID == t.ID && e.Worktree == worktree
		}) {
			out = append(out, t)
		}
	}
	slices.SortFunc(out, func(a, b store.Turn) int { return b.StartedAt.Compare(a.StartedAt) })
	return out, nil
}

// Chunks returns the in-memory chunk store.
func (s *Store) Chunks() store.Chunks { return s.chunks }

// Ping always succeeds.
func (s *Store) Ping(context.Context) error { return nil }

// Close does nothing.
func (s *Store) Close() error { return nil }

// newest returns the items keep accepts, latest time first; items with the
// same time come latest-appended first.
func newest[T any](items []T, keep func(T) (bool, time.Time)) []T {
	var out []T
	for i := len(items) - 1; i >= 0; i-- {
		if ok, _ := keep(items[i]); ok {
			out = append(out, items[i])
		}
	}
	slices.SortStableFunc(out, func(a, b T) int {
		_, ta := keep(a)
		_, tb := keep(b)
		return tb.Compare(ta)
	})
	return out
}

func limit[T any](items []T, n int) []T {
	if n > 0 && len(items) > n {
		return items[:n]
	}
	return items
}

// Chunks is an in-memory store.Chunks keyed by the SHA-256 of the bytes.
type Chunks struct {
	mu   sync.Mutex
	data map[string][]byte
}

// Put stores data and returns its key.
func (c *Chunks) Put(_ context.Context, data []byte) (string, error) {
	sum := sha256.Sum256(data)
	key := hex.EncodeToString(sum[:])
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[key] = slices.Clone(data)
	return key, nil
}

// Get returns the bytes stored under key; a missing key wraps
// store.ErrNotFound.
func (c *Chunks) Get(_ context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.data[key]
	if !ok {
		return nil, fmt.Errorf("faketest: chunk %s: %w", key, store.ErrNotFound)
	}
	return slices.Clone(data), nil
}

// Has reports whether key is stored.
func (c *Chunks) Has(_ context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.data[key]
	return ok, nil
}

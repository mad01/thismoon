package store

import "context"

// Store is what a backend implements. Every method takes a context because
// a backend may sit behind a network or a bucket; the local backend ignores
// it. Put methods upsert (newest record per id wins); Append methods add an
// immutable fact and are idempotent on ID, so a replayed spool envelope
// changes nothing. Listings come newest first, sessions by their last
// activity. There is one writer per
// store, the serve process; readers reach it over HTTP.
type Store interface {
	PutSession(ctx context.Context, s Session) error
	PutTurn(ctx context.Context, t Turn) error
	// PutAgent upserts an agent: the newest record per id wins whole. An
	// agent cannot move to another session.
	PutAgent(ctx context.Context, a Agent) error
	AppendEdit(ctx context.Context, e Edit) error
	AppendDelta(ctx context.Context, d UnattributedDelta) error
	AppendSnapshot(ctx context.Context, s Snapshot) error
	AppendCommitLink(ctx context.Context, l CommitLink) error

	// Sessions lists sessions most recently active first (SortSummaries),
	// each summarized from its records (Summarize) and kept when f keeps
	// it (SessionFilter.Keeps), so Since and the order both go by last
	// activity.
	Sessions(ctx context.Context, f SessionFilter) ([]SessionSummary, error)
	// Session loads one session with its summary and its turns in Seq
	// order. Returns ErrNotFound.
	Session(ctx context.Context, id string) (SessionDetail, error)
	// TurnsWithoutTotals lists up to limit closed or failed turns whose
	// Totals are missing or older than version, in SortForTotals order:
	// the newest session first, a session's turns by Seq. serve's totals
	// backfill computes them.
	TurnsWithoutTotals(ctx context.Context, version, limit int) ([]Turn, error)
	// Turn loads one turn with everything recorded under it. Returns
	// ErrNotFound.
	Turn(ctx context.Context, id string) (TurnDetail, error)
	// Agents lists a session's agents in SortAgents order; none for an
	// unknown session.
	Agents(ctx context.Context, sessionID string) ([]Agent, error)
	// Agent loads one agent. Returns ErrNotFound.
	Agent(ctx context.Context, id string) (Agent, error)
	// AgentsWithoutTotals lists up to limit agents whose Totals are
	// missing or older than version, the newest session first and a
	// session's agents in SortAgents order. serve's totals backfill
	// computes them.
	AgentsWithoutTotals(ctx context.Context, version, limit int) ([]Agent, error)
	// Edits lists edits newest first, narrowed by q.
	Edits(ctx context.Context, q EditQuery) ([]Edit, error)
	// LatestAfter returns the most recent Edit q allows that has an
	// AfterChunk, the head of the before-derivation chain: same worktree
	// and path, At at or before q.At, ID other than q.ExcludeID. Returns
	// ErrNotFound when the path has no such edit.
	LatestAfter(ctx context.Context, q ChainQuery) (Edit, error)
	// LatestSnapshot returns the newest snapshot q allows: same worktree,
	// At at or before q.At, ID other than q.ExcludeID. Snapshots with
	// equal times come latest-appended first. Returns ErrNotFound when
	// the worktree has no such snapshot.
	LatestSnapshot(ctx context.Context, q SnapshotQuery) (Snapshot, error)
	// OpenTurns lists the turns still open in the worktree, across sessions.
	// Two sessions with open turns in one worktree make a snapshot delta
	// ambiguous.
	OpenTurns(ctx context.Context, worktree string) ([]Turn, error)

	// Chunks is the content store the records point into.
	Chunks() Chunks
	// Ping reports whether the store is reachable and readable.
	Ping(ctx context.Context) error
	// Close flushes and releases the store.
	Close() error
}

// Chunks is content-addressed storage. The key is the lowercase hex SHA-256
// of the uncompressed bytes; a backend may compress at rest. Put is
// idempotent: storing bytes already present is a no-op that returns the
// same key.
type Chunks interface {
	Put(ctx context.Context, data []byte) (sha string, err error)
	Get(ctx context.Context, sha string) ([]byte, error)
	Has(ctx context.Context, sha string) (bool, error)
}

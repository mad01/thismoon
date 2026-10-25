package store

import "context"

// Store is what a backend implements. Every method takes a context because
// a backend may sit behind a network or a bucket; the local backend ignores
// it. Put methods upsert (newest record per id wins); Append methods add an
// immutable fact and are idempotent on ID, so a replayed spool envelope
// changes nothing. Listings come newest first. There is one writer per
// store, the serve process; readers reach it over HTTP.
type Store interface {
	PutSession(ctx context.Context, s Session) error
	PutTurn(ctx context.Context, t Turn) error
	AppendEdit(ctx context.Context, e Edit) error
	AppendDelta(ctx context.Context, d UnattributedDelta) error
	AppendSnapshot(ctx context.Context, s Snapshot) error
	AppendCommitLink(ctx context.Context, l CommitLink) error

	// Sessions lists sessions newest first by StartedAt, narrowed by f,
	// each summarized from its records (Summarize).
	Sessions(ctx context.Context, f SessionFilter) ([]SessionSummary, error)
	// Session loads one session with its summary and its turns in Seq
	// order. Returns ErrNotFound.
	Session(ctx context.Context, id string) (SessionDetail, error)
	// TurnsWithoutTotals lists up to limit closed or failed turns whose
	// Totals are missing or older than version, newest first by
	// StartedAt, then by ID. serve's backfill computes them.
	TurnsWithoutTotals(ctx context.Context, version, limit int) ([]Turn, error)
	// Turn loads one turn with everything recorded under it. Returns
	// ErrNotFound.
	Turn(ctx context.Context, id string) (TurnDetail, error)
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

// Package local is the moondb store on local disk: an append-only JSONL log
// for the records and a content-addressed object directory for the chunks,
// both under one state directory.
//
// Layout under the directory Open is given:
//
//	log/000001.jsonl        records, one {"t":kind,"r":record} per line
//	log/000002.jsonl        the next segment, started once a line would
//	                        push the last one past moondb.SegmentBytes
//	objects/<ab>/<sha>.gz   a chunk, gzipped, named by the SHA-256 of its
//	                        uncompressed bytes
//
// Open replays every segment in order into an in-memory index that serves
// every read. A write appends one line, syncs the segment, and only then
// updates the index, so a reader never sees a record that is not on disk.
// The serve process is the only writer; readers run concurrently with it
// under a read lock. Files are 0600 and directories 0700.
package local

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

const (
	logDirName     = moondb.LogDirName
	objectsDirName = moondb.ObjectsDirName
	dirPerm        = 0o700
	filePerm       = 0o600
)

// logf reports non-fatal store conditions (a torn final line, a record kind
// from a newer build); tests swap it to capture warnings.
var logf = log.Printf

// errClosed is returned by writes after Close.
var errClosed = errors.New("local: store closed")

// Store is the local backend of store.Store.
type Store struct {
	mu  sync.RWMutex
	dir string
	ix  *index
	// seg is the open segment, number segNum, holding segSize bytes. It
	// is nil once broken is set.
	seg      *os.File
	segNum   int
	segSize  int64
	segLimit int64
	// broken is set by Close, or by a failed append that could not be cut
	// back off the log; every later write returns it.
	broken error
	chunks *chunkStore
}

// Open loads the store under dir, creating the directory when it does not
// exist, and replays the log into memory. A torn final line, the remains of
// an append cut short by a crash, is dropped with a warning; any other line
// that does not parse is an error.
func Open(dir string) (*Store, error) {
	return open(dir, moondb.SegmentBytes)
}

// open is Open with the segment size as a parameter, so tests can roll
// segments without writing megabytes.
func open(dir string, segLimit int64) (*Store, error) {
	s := &Store{
		dir:      dir,
		ix:       newIndex(),
		segLimit: segLimit,
		chunks:   &chunkStore{dir: filepath.Join(dir, objectsDirName)},
	}
	for _, d := range []string{s.logDir(), s.chunks.dir} {
		if err := os.MkdirAll(d, dirPerm); err != nil {
			return nil, fmt.Errorf("create %s: %w", d, err)
		}
	}
	nums, err := listSegments(s.logDir())
	if err != nil {
		return nil, err
	}
	for i, n := range nums {
		if err := s.replay(n, i == len(nums)-1); err != nil {
			return nil, err
		}
	}
	last := 1
	if len(nums) > 0 {
		last = nums[len(nums)-1]
	}
	if err := s.openSegment(last); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) logDir() string { return filepath.Join(s.dir, logDirName) }

func (s *Store) segmentPath(n int) string {
	return filepath.Join(s.logDir(), segmentName(n))
}

// openSegment makes segment n the one appends go to, creating it if needed.
func (s *Store) openSegment(n int) error {
	f, err := os.OpenFile(s.segmentPath(n), os.O_APPEND|os.O_CREATE|os.O_WRONLY, filePerm)
	if err != nil {
		return fmt.Errorf("open segment: %w", err)
	}
	info, err := f.Stat()
	if err == nil {
		err = syncDir(s.logDir())
	}
	if err != nil {
		f.Close()
		return fmt.Errorf("open segment: %w", err)
	}
	s.seg, s.segNum, s.segSize = f, n, info.Size()
	return nil
}

// syncDir makes a new directory entry durable.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

// commit writes one record to the log and folds it into the index. Callers
// hold s.mu for writing.
func (s *Store) commit(kind string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", kind, err)
	}
	line, err := json.Marshal(logLine{T: kind, R: raw})
	if err != nil {
		return fmt.Errorf("encode %s: %w", kind, err)
	}
	if err := s.write(append(line, '\n')); err != nil {
		return err
	}
	return s.ix.apply(kind, raw)
}

// write appends one line to the open segment and syncs it, first rolling to
// a new segment when the line would push the open one past its limit. A
// failed write is cut back off, so the next line starts on a clean edge.
func (s *Store) write(line []byte) error {
	if s.broken != nil {
		return s.broken
	}
	if s.segSize > 0 && s.segSize+int64(len(line)) > s.segLimit {
		if err := s.roll(); err != nil {
			return fmt.Errorf("roll segment: %w", err)
		}
	}
	if _, err := s.seg.Write(line); err != nil {
		return s.rewind(fmt.Errorf("append record: %w", err))
	}
	if err := s.seg.Sync(); err != nil {
		return s.rewind(fmt.Errorf("sync segment: %w", err))
	}
	s.segSize += int64(len(line))
	return nil
}

// roll opens the next segment and closes the full one, which every write
// has already synced.
func (s *Store) roll() error {
	full := s.seg
	if err := s.openSegment(s.segNum + 1); err != nil {
		return err
	}
	full.Close()
	return nil
}

// rewind truncates the open segment back to its last good size after a
// failed write and returns cause. When even that fails the log can no
// longer be appended to safely, and the store refuses further writes.
func (s *Store) rewind(cause error) error {
	if err := s.seg.Truncate(s.segSize); err != nil {
		s.seg.Close()
		s.seg = nil
		s.broken = fmt.Errorf("local: log unwritable after a failed append: %w",
			errors.Join(cause, err))
		return s.broken
	}
	return cause
}

// PutSession upserts a session: the newest record for an id wins whole.
func (s *Store) PutSession(ctx context.Context, v store.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if v.ID == "" {
		return errors.New("local: session has no id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commit(kindSession, v)
}

// PutTurn upserts a turn. A new turn with Seq 0 takes the next Seq in its
// session; a turn already recorded keeps its Seq. A turn cannot move to
// another session.
func (s *Store) PutTurn(ctx context.Context, t store.Turn) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.ID == "" {
		return errors.New("local: turn has no id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if cur, ok := s.ix.turns[t.ID]; ok && cur.SessionID != t.SessionID {
		return fmt.Errorf("local: turn %s belongs to session %s, not %s",
			t.ID, cur.SessionID, t.SessionID)
	}
	return s.commit(kindTurn, s.ix.settledTurn(t))
}

// AppendEdit records an edit; an id already recorded is a no-op.
func (s *Store) AppendEdit(ctx context.Context, e store.Edit) error {
	return s.appendFact(ctx, kindEdit, e.ID, s.ix.edits.has, e)
}

// AppendDelta records an unattributed delta; an id already recorded is a
// no-op.
func (s *Store) AppendDelta(ctx context.Context, d store.UnattributedDelta) error {
	return s.appendFact(ctx, kindDelta, d.ID, s.ix.deltas.has, d)
}

// AppendSnapshot records a snapshot; an id already recorded is a no-op.
func (s *Store) AppendSnapshot(ctx context.Context, v store.Snapshot) error {
	return s.appendFact(ctx, kindSnapshot, v.ID, s.ix.snapshots.has, v)
}

// AppendCommitLink records a commit link; an id already recorded is a
// no-op.
func (s *Store) AppendCommitLink(ctx context.Context, l store.CommitLink) error {
	return s.appendFact(ctx, kindCommitLink, l.ID, s.ix.links.has, l)
}

// appendFact writes an immutable record unless has reports its id is
// already recorded.
func (s *Store) appendFact(
	ctx context.Context, kind, id string, has func(string) bool, v any,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if id == "" {
		return fmt.Errorf("local: %s has no id", kind)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if has(id) {
		return nil
	}
	return s.commit(kind, v)
}

// Sessions lists sessions newest first by StartedAt. Since keeps sessions
// started at or after it; Repo keeps sessions with an edit in the repo.
func (s *Store) Sessions(ctx context.Context, f store.SessionFilter) ([]store.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ix.sessionList(f), nil
}

// Session loads a session with its turns in Seq order, the worktrees it
// edited in (sorted; edits outside any repository add none), and its edit
// count. Returns store.ErrNotFound.
func (s *Store) Session(ctx context.Context, id string) (store.SessionDetail, error) {
	if err := ctx.Err(); err != nil {
		return store.SessionDetail{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ix.sessionDetail(id)
}

// Turn loads a turn with its edits, deltas, snapshots and commit links,
// each newest first. Returns store.ErrNotFound.
func (s *Store) Turn(ctx context.Context, id string) (store.TurnDetail, error) {
	if err := ctx.Err(); err != nil {
		return store.TurnDetail{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ix.turnDetail(id)
}

// Edits lists the edits matching every set field of q, newest first.
func (s *Store) Edits(ctx context.Context, q store.EditQuery) ([]store.Edit, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ix.editList(q), nil
}

// LatestAfter returns the newest edit q allows that has an AfterChunk.
// Returns store.ErrNotFound.
func (s *Store) LatestAfter(ctx context.Context, q store.ChainQuery) (store.Edit, error) {
	if err := ctx.Err(); err != nil {
		return store.Edit{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ix.latestAfter(q)
}

// OpenTurns lists the open turns, across sessions, that have an edit in
// worktree, newest first by StartedAt.
func (s *Store) OpenTurns(ctx context.Context, worktree string) ([]store.Turn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ix.openTurns(worktree), nil
}

// Chunks returns the object directory the records point into.
func (s *Store) Chunks() store.Chunks { return s.chunks }

// Ping reports whether the store is open, writable and its log directory
// readable.
func (s *Store) Ping(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.broken != nil {
		return s.broken
	}
	if _, err := os.Stat(s.logDir()); err != nil {
		return fmt.Errorf("stat log: %w", err)
	}
	return nil
}

// Close syncs and closes the open segment. Writes after Close fail; Close
// itself may be called again.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.seg == nil {
		return nil
	}
	seg := s.seg
	s.seg, s.broken = nil, errClosed
	if err := seg.Sync(); err != nil {
		seg.Close()
		return fmt.Errorf("sync segment: %w", err)
	}
	if err := seg.Close(); err != nil {
		return fmt.Errorf("close segment: %w", err)
	}
	return nil
}

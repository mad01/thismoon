// Package cloud is the moondb store of a shared instance. The records are
// the local backend's: the same append-only JSONL log and in-memory index,
// on the persistent volume of the one pod that serves them. The chunks live
// in a Cloud Storage bucket at objects/<sha>, each created only when absent
// (ifGenerationMatch=0), and a copy of the log sits beside them at
// log/<segment>: a segment once the log has moved past it, the open one
// every moondb.SegmentCopyEvery and once more on Close.
//
// The bucket is reached over its JSON API with application default
// credentials (Bucket); there is no SDK. The empty objects/ directory the
// local backend makes under the data directory stays unused.
package cloud

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// Config says where a cloud store keeps its records and chunks.
type Config struct {
	// Dir is the data directory on the persistent volume: the log.
	Dir string
	// Bucket holds the chunks and the copy of the log.
	Bucket *Bucket
	// Logf reports a copy of the log that failed; log.Printf when nil.
	Logf func(format string, args ...any)
	// PollEvery and CopyEvery pace the copy of the log; zero takes
	// moondb.SegmentPollEvery and moondb.SegmentCopyEvery.
	PollEvery time.Duration
	CopyEvery time.Duration
}

// Store is the cloud backend of store.Store: the local backend's records
// with the chunks in a bucket. Every record method is the local one.
type Store struct {
	*local.Store
	bucket *Bucket
	chunks *chunkStore
	copier *segmentCopier
	stop   context.CancelFunc
	done   chan struct{}
}

// Open loads the records under cfg.Dir, as the local backend does, and
// starts copying the log to the bucket. It does not reach the bucket
// before it returns; Ping does.
func Open(cfg Config) (*Store, error) {
	if cfg.Bucket == nil {
		return nil, errors.New("cloud: no bucket")
	}
	ls, err := local.Open(cfg.Dir)
	if err != nil {
		return nil, err
	}
	logf := cfg.Logf
	if logf == nil {
		logf = log.Printf
	}
	ctx, stop := context.WithCancel(context.Background())
	s := &Store{
		Store:  ls,
		bucket: cfg.Bucket,
		chunks: newChunkStore(cfg.Bucket),
		copier: newSegmentCopier(cfg.Dir, cfg.Bucket, logf),
		stop:   stop,
		done:   make(chan struct{}),
	}
	poll, every := orDefault(cfg.PollEvery, moondb.SegmentPollEvery),
		orDefault(cfg.CopyEvery, moondb.SegmentCopyEvery)
	go func() {
		defer close(s.done)
		s.copier.run(ctx, poll, every)
	}()
	return s, nil
}

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// Chunks returns the bucket the records point into.
func (s *Store) Chunks() store.Chunks { return s.chunks }

// Ping reports whether the log is writable and the bucket reachable.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.Store.Ping(ctx); err != nil {
		return err
	}
	if err := s.bucket.check(ctx); err != nil {
		return fmt.Errorf("bucket: %w", err)
	}
	return nil
}

// Close stops the copy loop, closes the log, and copies what the bucket
// does not hold of it yet, within moondb.FinalCopyTimeout. Writes after
// Close fail; Close itself may be called again.
func (s *Store) Close() error {
	s.stop()
	<-s.done
	err := s.Store.Close()
	ctx, cancel := context.WithTimeout(context.Background(), moondb.FinalCopyTimeout)
	defer cancel()
	if cerr := s.copier.copyAll(ctx); cerr != nil {
		err = errors.Join(err, fmt.Errorf("copy log to bucket %s: %w", s.bucket.Name(), cerr))
	}
	return err
}

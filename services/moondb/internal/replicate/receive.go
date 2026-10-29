// Package replicate copies a local moondb store to a shared instance. A
// local serve runs a Replicator: it reads its own log from a saved cursor,
// sends the chunks those records point at that the instance lacks, then the
// records in log order, and saves the cursor once the instance has taken
// them. The shared serve takes them with Missing, PutChunks and Apply,
// which write through the same store.Store methods ingest uses, so the
// shared store keeps one writer too. Records keep their ids, so a batch
// sent twice changes nothing the second time.
package replicate

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// ErrInvalid marks a batch the instance cannot take as sent: a chunk key
// that is not one, or a record that does not decode. A server maps it to
// a client error.
var ErrInvalid = errors.New("replicate: invalid batch")

// Batch is the body of POST /api/sync: chunks, then records in log order.
// A replicator sends the chunks before the records that point at them.
type Batch struct {
	Chunks  [][]byte     `json:"chunks,omitempty"`
	Records []local.Line `json:"records,omitempty"`
}

// BatchResult answers a Batch: the chunks stored, the records written, and
// the records of a kind the instance does not know, skipped as replay
// skips them.
type BatchResult struct {
	Chunks  int `json:"chunks"`
	Records int `json:"records"`
	Skipped int `json:"skipped,omitempty"`
}

// MissingRequest is the body of POST /api/sync/missing: chunk keys the
// next records point at.
type MissingRequest struct {
	Chunks []string `json:"chunks"`
}

// MissingResult names the keys of a MissingRequest the instance lacks, in
// the order asked.
type MissingResult struct {
	Missing []string `json:"missing"`
}

// Missing returns the keys chunks lacks, in the order given, asking about
// up to moondb.SyncParallelism keys at once.
func Missing(ctx context.Context, chunks store.Chunks, keys []string) ([]string, error) {
	for _, k := range keys {
		if !store.ValidChunkKey(k) {
			return nil, fmt.Errorf("%w: chunk key %q", ErrInvalid, k)
		}
	}
	lacks := make([]bool, len(keys))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(moondb.SyncParallelism)
	for i, k := range keys {
		g.Go(func() error {
			ok, err := chunks.Has(gctx, k)
			lacks[i] = !ok
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("replicate: look up chunks: %w", err)
	}
	var out []string
	for i, k := range keys {
		if lacks[i] {
			out = append(out, k)
		}
	}
	return out, nil
}

// PutChunks stores every chunk in data, up to moondb.SyncParallelism at
// once, and returns how many it stored.
func PutChunks(ctx context.Context, chunks store.Chunks, data [][]byte) (int, error) {
	var mu sync.Mutex
	stored := 0
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(moondb.SyncParallelism)
	for _, d := range data {
		g.Go(func() error {
			if _, err := chunks.Put(gctx, d); err != nil {
				return err
			}
			mu.Lock()
			stored++
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return stored, fmt.Errorf("replicate: store chunks: %w", err)
	}
	return stored, nil
}

// Apply writes lines to st in order, each through the method its record
// calls for: Put for sessions, turns and agents, Append for the facts. A
// line of a kind this build does not know is skipped and counted. Apply
// stops at the first line it cannot write and reports how many it wrote
// before it.
func Apply(
	ctx context.Context,
	st store.Store,
	lines []local.Line,
) (applied, skipped int, err error) {
	for i, l := range lines {
		rec, err := l.Decode()
		if errors.Is(err, local.ErrUnknownKind) {
			skipped++
			continue
		}
		if err != nil {
			return applied, skipped, fmt.Errorf("%w: record %d: %w", ErrInvalid, i, err)
		}
		if err := write(ctx, st, rec); err != nil {
			return applied, skipped, fmt.Errorf("replicate: record %d: %w", i, err)
		}
		applied++
	}
	return applied, skipped, nil
}

func write(ctx context.Context, st store.Store, rec any) error {
	switch v := rec.(type) {
	case store.Session:
		return st.PutSession(ctx, v)
	case store.Turn:
		return st.PutTurn(ctx, v)
	case store.Agent:
		return st.PutAgent(ctx, v)
	case store.Edit:
		return st.AppendEdit(ctx, v)
	case store.UnattributedDelta:
		return st.AppendDelta(ctx, v)
	case store.Snapshot:
		return st.AppendSnapshot(ctx, v)
	case store.CommitLink:
		return st.AppendCommitLink(ctx, v)
	}
	return fmt.Errorf("replicate: no store method for %T", rec)
}

package ingest

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/spool"
)

// Run pacing. A failed envelope waits retryDelay before its next attempt
// and moves to <spool>/failed/ after maxAttempts, so one poison envelope
// cannot stall the spool; sweepEvery is the longest the spool goes without
// a drain and a sweep, which is also what retries a failure when no new
// envelope arrives.
const (
	maxAttempts = 3
	retryDelay  = 10 * time.Second
	sweepEvery  = time.Minute
	failedDir   = "failed"
	failedPerm  = 0o700
)

// Run drains the spool into the store until ctx ends. It ingests every
// envelope present, then each one Spool.Watch reports, in name order; each
// ingested envelope is removed and every drained batch ends with a
// Spool.Sweep, as does a drain at least every minute. While the spool is
// empty, the totals backfill computes the totals of stored turns that lack
// them and fills the prompt lines of sessions that lack one, a few each
// tick (moondb.TotalsBackfillTurns every moondb.TotalsBackfillEvery), in
// the same goroutine, so it never races an envelope for a record. An
// envelope that fails to decode or ingest, or panics while ingesting, is
// logged and left for a later attempt without holding up the ones behind
// it. After maxAttempts it is moved to <spool>/failed/ with the envelope
// file only; its chunks go with the next sweep. A failure while Store.Ping also fails is the store's, not the
// envelope's: the batch stops and nothing is counted against it. Run
// returns nil when ctx ends and an error only when the spool cannot be
// watched.
func (in *Ingester) Run(ctx context.Context) error {
	r := &runner{
		in:       in,
		attempts: map[string]int{},
		retryAt:  map[string]time.Time{},
		skip:     map[string]bool{},
	}
	r.drain(ctx)
	ring := make(chan struct{}, 1)
	watched := make(chan error, 1)
	go func() {
		watched <- in.Spool.Watch(ctx, func(string) {
			select {
			case ring <- struct{}{}:
			default: // a drain is already due; it lists everything
			}
		})
	}()
	tick := time.NewTicker(sweepEvery)
	defer tick.Stop()
	backfill := time.NewTicker(moondb.TotalsBackfillEvery)
	defer backfill.Stop()
	for {
		select {
		case <-ctx.Done():
			return <-watched // nil: Watch ends cleanly with ctx
		case err := <-watched:
			return err
		case <-ring:
			r.drain(ctx)
		case <-tick.C:
			r.drain(ctx)
		case <-backfill.C:
			r.backfill(ctx)
		}
	}
}

// runner is the state one Run keeps: failed envelopes keyed by path, and
// the totals backfill's progress.
type runner struct {
	in       *Ingester
	attempts map[string]int
	retryAt  map[string]time.Time
	// skip holds the turns whose totals the totals backfill could not
	// compute; backfilled counts the turns computed since the backlog last
	// ran out.
	skip       map[string]bool
	backfilled int
	// promptLines holds the sessions still to get a prompt line, listed
	// once by the first batch (listed): a session recorded later gets its
	// prompt line at a turn open or close.
	promptLines []string
	listed      bool
}

// backfill runs one totals backfill batch, but only while the spool is
// empty: an envelope always goes first.
func (r *runner) backfill(ctx context.Context) {
	pending, err := r.in.Spool.List()
	if err != nil || len(pending) > 0 {
		return
	}
	r.backfillTotals(ctx)
	r.backfillPromptLines(ctx)
}

// backfillTotals computes one batch of missing turn totals. When a batch
// finds nothing left to try after earlier ones did work, it logs how many
// turns they computed.
func (r *runner) backfillTotals(ctx context.Context) {
	b, err := r.in.backfillTotals(ctx, moondb.TotalsBackfillTurns, r.skip)
	if err != nil {
		r.in.logf("%v", err)
		return
	}
	r.backfilled += b.computed
	if b.tried == 0 && r.backfilled > 0 {
		r.in.logf("ingest: totals backfill done: computed the totals of %d stored turns",
			r.backfilled)
		r.backfilled = 0
	}
}

// backfillPromptLines fills the prompt lines of up to
// moondb.TotalsBackfillTurns of the sessions that had none when Run
// started. A store failure leaves the rest for the next batch.
func (r *runner) backfillPromptLines(ctx context.Context) {
	if !r.listed {
		ids, err := r.in.sessionsWithoutPromptLine(ctx)
		if err != nil {
			r.in.logf("%v", err)
			return
		}
		r.promptLines, r.listed = ids, true
	}
	batch := r.promptLines[:min(moondb.TotalsBackfillTurns, len(r.promptLines))]
	n, err := r.in.fillPromptLines(ctx, batch)
	r.promptLines = r.promptLines[n:]
	if err != nil && ctx.Err() == nil {
		r.in.logf("ingest: totals backfill: %v; pausing the prompt lines until the next batch", err)
	}
}

// drain ingests every pending envelope not waiting out a retry delay, in
// name order, then sweeps the spool.
func (r *runner) drain(ctx context.Context) {
	paths, err := r.in.Spool.List()
	if err != nil {
		r.in.logf("ingest: list spool: %v", err)
		return
	}
	r.forget(paths)
	now := r.in.now()
	for _, path := range paths {
		if ctx.Err() != nil {
			return
		}
		if now.Before(r.retryAt[path]) {
			continue
		}
		if !r.ingest(ctx, path) {
			break
		}
	}
	if err := r.in.Spool.Sweep(); err != nil {
		r.in.logf("ingest: sweep: %v", err)
	}
}

// ingest reads, records and removes one envelope. It reports whether the
// batch should go on: false when ctx ended or the store is down.
func (r *runner) ingest(ctx context.Context, path string) bool {
	env, err := r.in.Spool.Read(path)
	if errors.Is(err, fs.ErrNotExist) {
		return true // removed since the listing
	}
	if err == nil {
		err = r.guarded(ctx, env)
	}
	if err == nil {
		r.clear(path)
		if err := r.in.Spool.Remove(path); err != nil {
			r.in.logf("ingest: %v", err)
		}
		return true
	}
	if ctx.Err() != nil {
		return false
	}
	if perr := r.in.Store.Ping(ctx); perr != nil {
		r.in.logf("ingest: store unavailable, pausing the spool: %v (while ingesting %s: %v)",
			perr, filepath.Base(path), err)
		return false
	}
	r.fail(path, err)
	return true
}

// guarded runs Ingest with a panic turned into an error, so an envelope
// that trips a bug uses up its attempts and moves aside like any other
// failure instead of crashing serve on every restart.
func (r *runner) guarded(ctx context.Context, env spool.Envelope) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("ingest: %s panicked: %v\n%s", env.ID, p, debug.Stack())
		}
	}()
	return r.in.Ingest(ctx, env)
}

// fail counts a failed attempt and moves the envelope aside once it has
// used them all.
func (r *runner) fail(path string, err error) {
	r.attempts[path]++
	n := r.attempts[path]
	name := filepath.Base(path)
	if n < maxAttempts {
		r.retryAt[path] = r.in.now().Add(retryDelay)
		r.in.logf("ingest: %s failed (attempt %d of %d): %v", name, n, maxAttempts, err)
		return
	}
	if merr := r.moveAside(path); merr != nil {
		r.retryAt[path] = r.in.now().Add(retryDelay)
		r.in.logf("ingest: %s failed %d times and could not be moved aside: %v: %v",
			name, n, merr, err)
		return
	}
	r.clear(path)
	r.in.logf("ingest: %s failed %d times, moved to %s/: %v", name, n, failedDir, err)
}

// moveAside moves an envelope into <spool>/failed/, where List does not
// look.
func (r *runner) moveAside(path string) error {
	dir := filepath.Join(r.in.Spool.Dir, failedDir)
	if err := os.MkdirAll(dir, failedPerm); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}
	return os.Rename(path, filepath.Join(dir, filepath.Base(path)))
}

func (r *runner) clear(path string) {
	delete(r.attempts, path)
	delete(r.retryAt, path)
}

// forget drops the state of envelopes no longer in the spool.
func (r *runner) forget(paths []string) {
	present := make(map[string]bool, len(paths))
	for _, p := range paths {
		present[p] = true
	}
	for p := range r.attempts {
		if !present[p] {
			r.clear(p)
		}
	}
	for p := range r.retryAt {
		if !present[p] {
			r.clear(p)
		}
	}
}

package k8sstore

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SweepExpired deletes every ephemeral page whose expiry has passed at now
// and reports how many it removed. It is idempotent across replicas: a
// page another replica already deleted counts as gone.
func (s *Store) SweepExpired(ctx context.Context, now time.Time) (int, error) {
	recs, err := s.list(ctx, LabelEphemeral+"=true")
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, rec := range recs {
		if !rec.Page.Expired(now) {
			continue
		}
		err := s.pages().Delete(ctx, rec.Page.ID, metav1.DeleteOptions{})
		if err != nil && !apierrors.IsNotFound(err) {
			return deleted, fmt.Errorf("delete expired page %s: %w", rec.Page.ID, err)
		}
		deleted++
	}
	return deleted, nil
}

// RunSweeper announces itself, sweeps once immediately, and then sweeps
// every interval until ctx is done. Each sweep logs its result, so the
// absence of a line is itself the signal that the goroutine is gone. A
// failed sweep is logged and retried next tick; it never stops the server,
// because serving pages matters more than purging old ones on time. A
// cancelled context ends the loop before the next sweep, so shutdown never
// logs a sweep that failed on the way out.
func RunSweeper(ctx context.Context, s *Store, interval time.Duration, logf func(string, ...any)) {
	logf("present: sweeping expired pages every %s", interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		s.sweepOnce(ctx, interval, logf)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// sweepOnce runs one sweep and logs how it went. Every completed sweep
// says so, deletions or not: at the default interval that is a line every
// ten minutes, which is what tells an operator the goroutine is still
// alive. A sweep that failed says so on its own line instead.
func (s *Store) sweepOnce(ctx context.Context, interval time.Duration, logf func(string, ...any)) {
	defer func() {
		if r := recover(); r != nil {
			logf("present: sweep panicked: %v", r)
		}
	}()
	n, err := s.SweepExpired(ctx, s.now())
	if err != nil {
		logf("present: sweep deleted=%d err=%v", n, err)
		return
	}
	logf("present: sweep deleted=%d next in %s", n, interval)
}

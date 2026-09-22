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

// RunSweeper sweeps once immediately and then every interval until ctx is
// done. A failed sweep is logged and retried next tick; it never stops the
// server, because serving pages matters more than purging old ones on time.
func RunSweeper(ctx context.Context, s *Store, interval time.Duration, logf func(string, ...any)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.sweepOnce(ctx, logf)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Store) sweepOnce(ctx context.Context, logf func(string, ...any)) {
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
	if n > 0 {
		logf("present: sweep deleted=%d", n)
	}
}

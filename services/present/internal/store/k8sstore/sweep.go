package k8sstore

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// SweepExpired deletes every ephemeral page whose expiry has passed at now
// and reports how many it removed. It judges pages from the page cache once
// that has synced, else from a list against the API server, and deletes
// each one only at the resourceVersion it was judged at. It is idempotent
// across replicas: a page another replica already deleted counts as gone.
func (s *Store) SweepExpired(ctx context.Context, now time.Time) (int, error) {
	pages, err := s.ephemeralPages(ctx)
	if err != nil {
		return 0, err
	}
	deleted := 0
	for _, cp := range pages {
		if !cp.page.Expired(now) {
			continue
		}
		gone, err := s.deleteAsSeen(ctx, cp)
		if err != nil {
			return deleted, err
		}
		if gone {
			deleted++
		}
	}
	return deleted, nil
}

// ephemeralPages returns the ephemeral pages, from the cache once it has
// synced and from the API server before that.
func (s *Store) ephemeralPages(ctx context.Context) ([]*cachedPage, error) {
	if s.cache.synced() {
		return s.cache.ephemeral()
	}
	var out []*cachedPage
	err := s.eachObject(ctx, LabelEphemeral+"=true", func(u *unstructured.Unstructured) error {
		cp := newCachedPage(u)
		if cp.err != nil {
			return cp.err
		}
		out = append(out, cp)
		return nil
	})
	return out, err
}

// deleteAsSeen deletes a page under a resourceVersion precondition, so the
// API server refuses when the page changed after the sweeper looked at it,
// which a cache trailing the watch could otherwise miss. It reports whether
// the page is gone; a refused delete is left for the next sweep to judge.
func (s *Store) deleteAsSeen(ctx context.Context, cp *cachedPage) (bool, error) {
	rv := cp.ResourceVersion
	err := s.pages().Delete(ctx, cp.Name, metav1.DeleteOptions{
		Preconditions: &metav1.Preconditions{ResourceVersion: &rv},
	})
	switch {
	case err == nil, apierrors.IsNotFound(err):
		return true, nil
	case apierrors.IsConflict(err):
		return false, nil
	default:
		return false, fmt.Errorf("delete expired page %s: %w", cp.Name, err)
	}
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

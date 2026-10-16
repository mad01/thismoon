package store

import (
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// A writer reaches the page lock within microseconds of starting; the
// timeout only bounds a broken run, and the poll keeps the wait cheap.
const (
	lockWaitTimeout = 10 * time.Second
	lockWaitPoll    = time.Millisecond
	stackBufBytes   = 1 << 20
)

// TestWritesThatWaitedOutADeleteFindNoPage pins the probe that runs once the
// lock is granted: a writer that opened the page directory and then waited
// while the page was removed gets ErrNotFound, and nothing comes back at the
// page's path.
func TestWritesThatWaitedOutADeleteFindNoPage(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "T", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	title := "x"
	writes := map[string]func() error{
		"Edit": func() error {
			_, err := s.Edit(ctx, p.ID, PatchEdit(Patch{Title: &title}))
			return err
		},
		"Update": func() error {
			_, err := s.Update(ctx, p.ID, Patch{Title: &title})
			return err
		},
		"SetShared": func() error { return s.SetShared(ctx, p.ID, nil) },
		"Delete":    func() error { return s.Delete(ctx, p.ID) },
	}
	results := writeWhileLocked(t, s, p.ID, writes, func() error {
		return os.RemoveAll(s.pageDir(p.ID))
	})
	for name, err := range results {
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s that waited out a delete: err = %v, want ErrNotFound", name, err)
		}
	}
	if _, err := os.Stat(s.pageDir(p.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("page dir after the waiting writes: err = %v, want it gone", err)
	}
}

// TestAWriterThatWaitedOutARecreateLeavesTheNewPage pins the other half of
// the probe: a page removed and created again under the same id while a
// writer waited is a new directory the writer's lock does not cover, so the
// writer gets ErrNotFound and the new page stays as it was created.
func TestAWriterThatWaitedOutARecreateLeavesTheNewPage(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p, err := s.Create(ctx, Draft{Title: "old", Content: "<p>x</p>"})
	if err != nil {
		t.Fatal(err)
	}
	title := "from the waiting writer"
	writes := map[string]func() error{
		"Edit": func() error {
			_, err := s.Edit(ctx, p.ID, PatchEdit(Patch{Title: &title}))
			return err
		},
	}
	results := writeWhileLocked(t, s, p.ID, writes, func() error {
		if err := os.RemoveAll(s.pageDir(p.ID)); err != nil {
			return err
		}
		_, err := s.Create(ctx, Draft{ID: p.ID, Title: "new", Content: "<p>y</p>"})
		return err
	})
	if !errors.Is(results["Edit"], ErrNotFound) {
		t.Fatalf("Edit that waited out a recreate: err = %v, want ErrNotFound", results["Edit"])
	}
	got, err := s.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "new" || got.Version != 1 {
		t.Fatalf("recreated page = %q v%d, want \"new\" v1", got.Title, got.Version)
	}
}

// writeWhileLocked holds the page lock, starts every write, waits until each
// one has opened the page directory and blocks on the lock, runs change,
// then releases the lock and returns what each write returned.
func writeWhileLocked(
	t *testing.T, s *FS, id string, writes map[string]func() error, change func() error,
) map[string]error {
	t.Helper()
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		results = make(map[string]error, len(writes))
	)
	err := s.withPageLock(id, func() error {
		for name, write := range writes {
			wg.Go(func() {
				err := write()
				mu.Lock()
				defer mu.Unlock()
				results[name] = err
			})
		}
		waitForLockWaiters(t, len(writes))
		return change()
	})
	wg.Wait()
	if err != nil {
		t.Fatalf("change under the held lock: %v", err)
	}
	return results
}

// waitForLockWaiters blocks until n goroutines sit in lockExclusive. A
// writer gets there only after opening the page directory, so one counted
// here has already passed the existence check that runs before the lock.
func waitForLockWaiters(t *testing.T, n int) {
	t.Helper()
	buf := make([]byte, stackBufBytes)
	for deadline := time.Now().Add(lockWaitTimeout); time.Now().Before(deadline); {
		size := runtime.Stack(buf, true)
		if strings.Count(string(buf[:size]), "store.lockExclusive(") >= n {
			return
		}
		time.Sleep(lockWaitPoll)
	}
	t.Fatalf("%d writers never reached the page lock", n)
}

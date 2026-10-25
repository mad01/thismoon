package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/store/storetest"
)

func TestConformance(t *testing.T) {
	storetest.Run(t, func(t *testing.T, dir string) store.Store {
		t.Helper()
		return mustOpen(t, dir, 0)
	})
}

// mustOpen opens the store under dir, with segments of segLimit bytes when
// it is not 0.
func mustOpen(t *testing.T, dir string, segLimit int64) *Store {
	t.Helper()
	if segLimit == 0 {
		s, err := Open(dir)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		return s
	}
	s, err := open(dir, segLimit)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return s
}

func mustClose(t *testing.T, s *Store) {
	t.Helper()
	if err := s.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
}

// captureLog swaps logf for the test and returns the lines it logged.
func captureLog(t *testing.T) *[]string {
	t.Helper()
	var lines []string
	prev := logf
	logf = func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) }
	t.Cleanup(func() { logf = prev })
	return &lines
}

func testEdit(i int) store.Edit {
	return store.Edit{
		ID:        fmt.Sprintf("e%03d", i),
		SessionID: "s1",
		TurnID:    "t1",
		At:        time.Date(2026, 10, 1, 9, 0, i, 0, time.UTC),
		Location:  store.Location{Repo: "github.com/o/a", Worktree: "/wt/a"},
		Path:      "x.go",
	}
}

func appendEdits(t *testing.T, s *Store, from, to int) {
	t.Helper()
	for i := from; i < to; i++ {
		if err := s.AppendEdit(context.Background(), testEdit(i)); err != nil {
			t.Fatalf("AppendEdit(%d): %v", i, err)
		}
	}
}

func editCount(t *testing.T, s *Store) int {
	t.Helper()
	edits, err := s.Edits(context.Background(), store.EditQuery{})
	if err != nil {
		t.Fatalf("Edits: %v", err)
	}
	return len(edits)
}

func TestPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	s := mustOpen(t, dir, 0)
	appendEdits(t, s, 0, 1)
	sha, err := s.Chunks().Put(context.Background(), []byte("secret"))
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	cases := []struct {
		path string
		want os.FileMode
	}{
		{dir, dirPerm},
		{filepath.Join(dir, logDirName), dirPerm},
		{filepath.Join(dir, objectsDirName), dirPerm},
		{filepath.Join(dir, objectsDirName, sha[:2]), dirPerm},
		{filepath.Join(dir, logDirName, segmentName(1)), filePerm},
		{filepath.Join(dir, objectsDirName, sha[:2], sha+".gz"), filePerm},
	}
	for _, c := range cases {
		info, err := os.Stat(c.path)
		if err != nil {
			t.Fatalf("stat: %v", err)
		}
		if got := info.Mode().Perm(); got != c.want {
			t.Errorf("%s mode = %o, want %o", c.path, got, c.want)
		}
	}
}

func TestClosedStoreRefusesWrites(t *testing.T) {
	s := mustOpen(t, t.TempDir(), 0)
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
	if err := s.AppendEdit(context.Background(), testEdit(0)); !errors.Is(err, errClosed) {
		t.Errorf("AppendEdit after Close: err = %v, want errClosed", err)
	}
	if err := s.Ping(context.Background()); !errors.Is(err, errClosed) {
		t.Errorf("Ping after Close: err = %v, want errClosed", err)
	}
}

func TestPutTurnRefusesMovingSessions(t *testing.T) {
	s := mustOpen(t, t.TempDir(), 0)
	t.Cleanup(func() { mustClose(t, s) })
	ctx := context.Background()
	if err := s.PutTurn(ctx, store.Turn{ID: "t1", SessionID: "s1"}); err != nil {
		t.Fatalf("PutTurn: %v", err)
	}
	if err := s.PutTurn(ctx, store.Turn{ID: "t1", SessionID: "s2"}); err == nil {
		t.Error("PutTurn moving t1 to s2 succeeded, want an error")
	}
}

func TestCanceledContext(t *testing.T) {
	s := mustOpen(t, t.TempDir(), 0)
	t.Cleanup(func() { mustClose(t, s) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.AppendEdit(ctx, testEdit(0)); !errors.Is(err, context.Canceled) {
		t.Errorf("AppendEdit: err = %v, want context.Canceled", err)
	}
	if n := editCount(t, s); n != 0 {
		t.Errorf("edits after a canceled append = %d, want 0", n)
	}
}

func TestReadsDuringWrites(t *testing.T) {
	s := mustOpen(t, t.TempDir(), 0)
	t.Cleanup(func() { mustClose(t, s) })
	ctx := context.Background()
	if err := s.PutTurn(ctx, store.Turn{ID: "t1", SessionID: "s1", Status: store.TurnOpen}); err != nil {
		t.Fatalf("PutTurn: %v", err)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for range 50 {
				if _, err := s.Edits(ctx, store.EditQuery{Worktree: "/wt/a"}); err != nil {
					t.Errorf("Edits: %v", err)
					return
				}
				if _, err := s.OpenTurns(ctx, "/wt/a"); err != nil {
					t.Errorf("OpenTurns: %v", err)
					return
				}
			}
		})
	}
	appendEdits(t, s, 0, 50)
	wg.Wait()
	if n := editCount(t, s); n != 50 {
		t.Errorf("edits = %d, want 50", n)
	}
}

package refresh

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

// testRefresher builds a Refresher with an injected clock and run func, never
// touching the real config, filesystem, or syncer.
func testRefresher(run runFunc, now func() time.Time) *Refresher {
	return &Refresher{
		cfg:       &config.Config{},
		enabled:   true,
		interval:  15 * time.Minute,
		heartbeat: time.Minute,
		run:       run,
		now:       now,
		kick:      make(chan string, 1),
		results:   map[string]outcome{},
	}
}

func fixedNow() time.Time {
	return time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
}

func TestCycleSuccessRecordsOutcomes(t *testing.T) {
	rep := &syncer.Report{
		Results: []syncer.PullResult{
			{Repo: finder.Repo{Name: "org/a", Path: "/repos/a"}, Status: "updated"},
			{
				Repo:    finder.Repo{Name: "org/b", Path: "/repos/b"},
				Status:  "dirty",
				Message: "on main, uncommitted changes",
			},
		},
		Indexed: []finder.Repo{{Name: "org/a", Path: "/repos/a"}},
	}
	r := testRefresher(func(context.Context, string) (*syncer.Report, error) {
		return rep, nil
	}, fixedNow)

	r.cycle(context.Background(), "")

	if r.lastRun != fixedNow() {
		t.Errorf("lastRun = %v, want %v", r.lastRun, fixedNow())
	}
	if r.lastErr != "" {
		t.Errorf("lastErr = %q, want empty", r.lastErr)
	}
	a := r.results["/repos/a"]
	if a.status != "updated" || !a.indexed {
		t.Errorf("outcome for /repos/a = %+v, want updated+indexed", a)
	}
	b := r.results["/repos/b"]
	if b.status != "dirty" || b.indexed {
		t.Errorf("outcome for /repos/b = %+v, want dirty, not indexed", b)
	}
}

func TestCycleSingleRepoDoesNotResetClock(t *testing.T) {
	r := testRefresher(func(context.Context, string) (*syncer.Report, error) {
		return &syncer.Report{}, nil
	}, fixedNow)

	r.cycle(context.Background(), "org/a")

	if !r.lastRun.IsZero() {
		t.Errorf("single-repo cycle set lastRun = %v, want zero", r.lastRun)
	}
}

func TestCycleLockedSkipsWithoutBackoff(t *testing.T) {
	r := testRefresher(func(context.Context, string) (*syncer.Report, error) {
		return nil, fmt.Errorf("wrapped: %w", syncer.ErrLocked)
	}, fixedNow)

	r.cycle(context.Background(), "")

	if r.failures != 0 {
		t.Errorf("failures = %d, want 0 for a locked skip", r.failures)
	}
	if !r.nextAttempt.IsZero() {
		t.Errorf("nextAttempt = %v, want zero (no backoff)", r.nextAttempt)
	}
	if r.lastErr == "" {
		t.Error("lastErr should record the skip")
	}
	if !r.lastRun.IsZero() {
		t.Error("a skipped cycle should not count as a run")
	}
}

func TestCycleFailureBacksOffExponentially(t *testing.T) {
	r := testRefresher(func(context.Context, string) (*syncer.Report, error) {
		return nil, errors.New("boom")
	}, fixedNow)

	r.cycle(context.Background(), "")
	first := r.nextAttempt
	if want := fixedNow().Add(time.Minute); first != want {
		t.Errorf("first backoff until %v, want %v", first, want)
	}

	r.cycle(context.Background(), "")
	if want := fixedNow().Add(2 * time.Minute); r.nextAttempt != want {
		t.Errorf("second backoff until %v, want %v", r.nextAttempt, want)
	}
	if r.lastErr != "boom" {
		t.Errorf("lastErr = %q, want boom", r.lastErr)
	}
}

func TestDue(t *testing.T) {
	now := fixedNow()
	tests := []struct {
		name   string
		mutate func(r *Refresher)
		want   bool
	}{
		{"never ran", func(r *Refresher) {}, true},
		{"fresh", func(r *Refresher) { r.lastRun = now.Add(-time.Minute) }, false},
		{"stale", func(r *Refresher) { r.lastRun = now.Add(-16 * time.Minute) }, true},
		{"backing off", func(r *Refresher) { r.nextAttempt = now.Add(time.Minute) }, false},
		{"disabled", func(r *Refresher) { r.enabled = false }, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testRefresher(nil, func() time.Time { return now })
			tt.mutate(r)
			if got := r.due(); got != tt.want {
				t.Errorf("due() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestKickBusy(t *testing.T) {
	r := testRefresher(nil, fixedNow)

	if err := r.Kick(""); err != nil {
		t.Fatalf("first kick: %v", err)
	}
	if err := r.Kick(""); !errors.Is(err, ErrBusy) {
		t.Errorf("second kick while queued = %v, want ErrBusy", err)
	}

	<-r.kick // drain the queue
	r.mu.Lock()
	r.running = true
	r.mu.Unlock()
	if err := r.Kick(""); !errors.Is(err, ErrBusy) {
		t.Errorf("kick while running = %v, want ErrBusy", err)
	}
}

func TestStatusReportsLoopState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".local", "state"))
	r := testRefresher(nil, fixedNow)
	r.lastRun = fixedNow().Add(-5 * time.Minute)
	r.lastErr = "boom"
	r.results["/repos/a"] = outcome{status: "updated", indexed: true, at: fixedNow()}

	st := r.Status()

	if !st.Enabled || st.IntervalMinutes != 15 {
		t.Errorf("status = %+v, want enabled with 15m interval", st)
	}
	if st.LastRun == "" || st.NextRun == "" {
		t.Errorf("status times missing: last=%q next=%q", st.LastRun, st.NextRun)
	}
	if st.LastError != "boom" {
		t.Errorf("LastError = %q, want boom", st.LastError)
	}
	if st.Repos == nil {
		t.Error("Repos must be non-nil so it marshals as []")
	}
}

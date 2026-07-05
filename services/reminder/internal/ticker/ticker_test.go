package ticker

import (
	"errors"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/reminder/internal/store"
)

// fakeNotifier records each Notify call and can be set to fail.
type fakeNotifier struct {
	calls []string
	fail  bool
}

func (f *fakeNotifier) Notify(title, body string) error {
	if f.fail {
		return errors.New("boom")
	}
	f.calls = append(f.calls, title)
	return nil
}

func newStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return s
}

func TestCycleFiresOneShotExactlyOnce(t *testing.T) {
	s := newStore(t)
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	if _, err := s.Create(store.CreateInput{Title: "ping", Due: now.Add(-time.Minute)}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	n := &fakeNotifier{}

	Cycle(s, n, clock)
	Cycle(s, n, clock) // second cycle must not re-fire

	if len(n.calls) != 1 || n.calls[0] != "ping" {
		t.Fatalf("want one fire of 'ping', got %v", n.calls)
	}
	got := s.List("")[0]
	if got.Status != store.StatusFired || got.FiredAt == nil {
		t.Errorf("one-shot should be fired with FiredAt set: %+v", got)
	}
}

func TestCycleRecurringReschedules(t *testing.T) {
	s := newStore(t)
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r, _ := s.Create(
		store.CreateInput{Title: "standup", Due: now.Add(-time.Minute), Repeat: "daily"},
	)
	n := &fakeNotifier{}

	Cycle(s, n, clock)
	Cycle(s, n, clock) // not due again at the same now

	if len(n.calls) != 1 {
		t.Fatalf("recurring should fire once per due window, got %d", len(n.calls))
	}
	got, _ := s.Get(r.ID)
	if got.Status != store.StatusPending {
		t.Errorf("recurring should stay pending, got %s", got.Status)
	}
	if !got.Due.After(now) {
		t.Errorf("recurring due should advance to the future, got %v", got.Due)
	}
}

func TestCycleSkipsNonPending(t *testing.T) {
	s := newStore(t)
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r, _ := s.Create(store.CreateInput{Title: "x", Due: now.Add(-time.Minute)})
	if _, err := s.Cancel(r.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	n := &fakeNotifier{}

	Cycle(s, n, clock)

	if len(n.calls) != 0 {
		t.Errorf("cancelled reminder must not fire, got %v", n.calls)
	}
}

func TestCycleNotifyFailureLeavesPending(t *testing.T) {
	s := newStore(t)
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	r, _ := s.Create(store.CreateInput{Title: "x", Due: now.Add(-time.Minute)})
	n := &fakeNotifier{fail: true}

	Cycle(s, n, clock)

	got, _ := s.Get(r.ID)
	if got.Status != store.StatusPending {
		t.Errorf("failed delivery should leave reminder pending for retry, got %s", got.Status)
	}
}

package store

import (
	"testing"
	"time"
)

// newTestStore returns a Store backed by a temp dir with a fixed clock, so
// tests never touch the real ~/.local/share/reminder and times are
// deterministic.
func newTestStore(t *testing.T) (*Store, time.Time) {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	clock := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return clock }
	return s, clock
}

func TestCreateValidates(t *testing.T) {
	s, clock := newTestStore(t)
	if _, err := s.Create(CreateInput{Title: "", Due: clock.Add(time.Hour)}); err == nil {
		t.Error("empty title should fail")
	}
	if _, err := s.Create(CreateInput{Title: "x"}); err == nil {
		t.Error("zero due should fail")
	}
	if _, err := s.Create(CreateInput{Title: "x", Due: clock.Add(time.Hour), Repeat: "banana"}); err == nil {
		t.Error("bad repeat should fail")
	}
}

func TestCreateGetRoundTrips(t *testing.T) {
	s, clock := newTestStore(t)
	r, err := s.Create(
		CreateInput{
			Title:  "ship it",
			Body:   "the PR",
			Due:    clock.Add(2 * time.Hour),
			Repeat: "daily",
		},
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.ID == "" || r.Status != StatusPending {
		t.Fatalf("unexpected created reminder: %+v", r)
	}

	got, err := s.Get(r.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != "ship it" || got.Body != "the PR" || got.Repeat != "daily" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if _, err := s.Get("nope"); err != ErrNotFound {
		t.Errorf("Get missing: want ErrNotFound, got %v", err)
	}
}

func TestPersistenceReload(t *testing.T) {
	dir := t.TempDir()
	s1, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	due := time.Date(2026, 6, 26, 9, 0, 0, 0, time.UTC)
	r, err := s1.Create(CreateInput{Title: "standup", Due: due})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// A fresh Store over the same dir sees the persisted reminder.
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := s2.Get(r.ID)
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if got.Title != "standup" {
		t.Errorf("reloaded title = %q, want standup", got.Title)
	}
}

func TestListSortsByDueAndFilters(t *testing.T) {
	s, clock := newTestStore(t)
	late, _ := s.Create(CreateInput{Title: "late", Due: clock.Add(3 * time.Hour)})
	early, _ := s.Create(CreateInput{Title: "early", Due: clock.Add(1 * time.Hour)})

	all := s.List("")
	if len(all) != 2 || all[0].ID != early.ID || all[1].ID != late.ID {
		t.Fatalf("List not sorted by due: %+v", all)
	}

	if _, err := s.Cancel(late.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	pending := s.List(StatusPending)
	if len(pending) != 1 || pending[0].ID != early.ID {
		t.Errorf("status filter wrong: %+v", pending)
	}
}

func TestUpdateRearmsFired(t *testing.T) {
	s, clock := newTestStore(t)
	r, _ := s.Create(CreateInput{Title: "x", Due: clock.Add(time.Hour)})

	// Force it to fired, then move due into the future via Update.
	if _, err := s.Trigger(r.ID, clock.Add(2*time.Hour)); err != nil {
		t.Fatalf("Trigger: %v", err)
	}
	if got, _ := s.Get(r.ID); got.Status != StatusFired {
		t.Fatalf("precondition: want fired, got %s", got.Status)
	}

	newDue := clock.Add(24 * time.Hour)
	updated, err := s.Update(r.ID, Patch{Due: &newDue})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Status != StatusPending || updated.FiredAt != nil {
		t.Errorf("re-arm failed: status=%s firedAt=%v", updated.Status, updated.FiredAt)
	}
}

func TestCancelStopsFiring(t *testing.T) {
	s, clock := newTestStore(t)
	r, _ := s.Create(CreateInput{Title: "x", Due: clock.Add(-time.Hour)}) // already due
	if _, err := s.Cancel(r.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	due := s.DueReminders(clock)
	if len(due) != 0 {
		t.Errorf("cancelled reminder should not be due: %+v", due)
	}
}

func TestDeleteRemoves(t *testing.T) {
	s, clock := newTestStore(t)
	r, _ := s.Create(CreateInput{Title: "x", Due: clock.Add(time.Hour)})
	if err := s.Delete(r.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := s.Get(r.ID); err != ErrNotFound {
		t.Errorf("Get after delete: want ErrNotFound, got %v", err)
	}
	if err := s.Delete(r.ID); err != ErrNotFound {
		t.Errorf("Delete missing: want ErrNotFound, got %v", err)
	}
}

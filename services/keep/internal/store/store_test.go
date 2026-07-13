package store

import (
	"testing"
	"time"

	"github.com/mad01/thismoon/services/keep/internal/pin"
)

// zeroReader yields deterministic bytes so generated ids differ only by their
// timestamp, making ordering assertions stable.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// incrClock returns a clock that advances 1ms per call, so each store mutation
// gets a distinct, monotonically increasing timestamp (and id).
func incrClock(start time.Time) func() time.Time {
	cur := start
	return func() time.Time {
		t := cur
		cur = cur.Add(time.Millisecond)
		return t
	}
}

// newTestStore returns a Store backed by a temp dir with a fixed incrementing
// clock and deterministic randomness, so times and ids are stable across runs.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.now = incrClock(time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))
	s.rnd = zeroReader{}
	return s
}

// testPin builds a resolved pin with a fixed hash; store logic never re-hashes,
// so the content is irrelevant here — the checkPin stub decides freshness.
func testPin(file string, start, end int) pin.Pin {
	return pin.Pin{File: file, StartLine: start, EndLine: end, ContentSHA256: "deadbeef"}
}

// validInput is a well-formed AssertInput with one pin, for reuse across cases.
func validInput(subject string, pins ...pin.Pin) AssertInput {
	if len(pins) == 0 {
		pins = []pin.Pin{testPin("a.go", 1, 3)}
	}
	return AssertInput{
		Kind:       KindCodeBehavior,
		Subject:    subject,
		Statement:  "it does the thing",
		Confidence: ConfidenceDerived,
		SessionID:  "sess-1",
		CostTokens: 42,
		Pins:       pins,
	}
}

func TestAssertValidates(t *testing.T) {
	s := newTestStore(t)
	cases := map[string]AssertInput{
		"bad kind": func() AssertInput {
			in := validInput("api")
			in.Kind = "guess"
			return in
		}(),
		"bad confidence": func() AssertInput {
			in := validInput("api")
			in.Confidence = "sure"
			return in
		}(),
		"empty subject":   validInput("   "),
		"empty statement": func() AssertInput { in := validInput("api"); in.Statement = ""; return in }(),
		"zero pins":       func() AssertInput { in := validInput("api"); in.Pins = nil; return in }(),
	}
	for name, in := range cases {
		if _, err := s.Assert(in); err == nil {
			t.Errorf("%s: want error, got nil", name)
		}
	}
}

func TestAssertGetRoundTrips(t *testing.T) {
	s := newTestStore(t)
	a, err := s.Assert(validInput("api/router"))
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}
	if a.ID == "" || a.Status != StatusFresh {
		t.Fatalf("unexpected created assertion: %+v", a)
	}
	if a.Provenance.SessionID != "sess-1" || a.Provenance.CostTokens != 42 {
		t.Errorf("provenance not carried: %+v", a.Provenance)
	}
	if !a.Provenance.DerivedAt.Equal(a.CreatedAt) {
		t.Errorf("DerivedAt %v != CreatedAt %v", a.Provenance.DerivedAt, a.CreatedAt)
	}

	got, err := s.Get(a.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Subject != "api/router" || got.Statement != "it does the thing" || len(got.Pins) != 1 {
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
	a, err := s1.Assert(validInput("api"))
	if err != nil {
		t.Fatalf("Assert: %v", err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got, err := s2.Get(a.ID)
	if err != nil {
		t.Fatalf("Get after reload: %v", err)
	}
	if got.Subject != "api" || len(got.Pins) != 1 || got.Pins[0].File != "a.go" {
		t.Errorf("reloaded assertion wrong: %+v", got)
	}
}

func TestListFiltersAndOrders(t *testing.T) {
	s := newTestStore(t)
	first, _ := s.Assert(validInput("api/router"))
	second := mustAssert(t, s, func() AssertInput {
		in := validInput("api/store")
		in.Kind = KindDecision
		return in
	}())
	third, _ := s.Assert(validInput("cli/root"))

	// Newest first.
	all := s.List(Filter{})
	if len(all) != 3 || all[0].ID != third.ID || all[2].ID != first.ID {
		t.Fatalf("List not newest-first: %+v", ids(all))
	}

	// Subject prefix.
	api := s.List(Filter{Subject: "api/"})
	if len(api) != 2 {
		t.Errorf("subject prefix: got %d, want 2", len(api))
	}

	// Kind exact.
	decisions := s.List(Filter{Kind: KindDecision})
	if len(decisions) != 1 || decisions[0].ID != second.ID {
		t.Errorf("kind filter wrong: %+v", ids(decisions))
	}

	// Combined subject + kind.
	combined := s.List(Filter{Subject: "api/", Kind: KindDecision})
	if len(combined) != 1 || combined[0].ID != second.ID {
		t.Errorf("combined filter wrong: %+v", ids(combined))
	}

	// Status filter after retracting one.
	if _, err := s.Retract(first.ID, "obsolete"); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	retracted := s.List(Filter{Status: StatusRetracted})
	if len(retracted) != 1 || retracted[0].ID != first.ID {
		t.Errorf("status filter wrong: %+v", ids(retracted))
	}
}

func TestRetractIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	a, _ := s.Assert(validInput("api"))

	first, err := s.Retract(a.ID, "wrong assumption")
	if err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if first.Status != StatusRetracted || first.RetractNote != "wrong assumption" ||
		first.RetractedAt == nil {
		t.Fatalf("retract did not set fields: %+v", first)
	}

	// A second retract must not overwrite the original note.
	second, err := s.Retract(a.ID, "different note")
	if err != nil {
		t.Fatalf("second Retract: %v", err)
	}
	if second.RetractNote != "wrong assumption" || !second.RetractedAt.Equal(*first.RetractedAt) {
		t.Errorf("second retract mutated record: %+v", second)
	}
}

func TestCheckFlipsAndCounts(t *testing.T) {
	s := newTestStore(t)
	a := mustAssert(t, s, validInput("api", testPin("a.go", 2, 4)))

	// Stub reports the pin no longer holds.
	s.checkPin = func(pin.Pin) (bool, string) { return false, "content changed" }
	report, err := s.Check(a.ID)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.Checked != 1 || report.Stale != 1 || report.Fresh != 0 || report.Flipped != 1 {
		t.Errorf("stale report counts wrong: %+v", report)
	}
	got, _ := s.Get(a.ID)
	if got.Status != StatusStale {
		t.Fatalf("want stale, got %s", got.Status)
	}
	if want := "content changed (a.go:2-4)"; got.StaleReason != want {
		t.Errorf("StaleReason = %q, want %q", got.StaleReason, want)
	}
	if got.CheckedAt == nil {
		t.Error("CheckedAt not set")
	}

	// Stub now reports it holds again — status restores to fresh.
	s.checkPin = func(pin.Pin) (bool, string) { return true, "" }
	report, err = s.Check(a.ID)
	if err != nil {
		t.Fatalf("re-Check: %v", err)
	}
	if report.Fresh != 1 || report.Stale != 0 || report.Flipped != 1 {
		t.Errorf("restore report counts wrong: %+v", report)
	}
	if got, _ := s.Get(a.ID); got.Status != StatusFresh || got.StaleReason != "" {
		t.Errorf("restore failed: status=%s reason=%q", got.Status, got.StaleReason)
	}
}

func TestCheckSkipsRetractedAndScopesToID(t *testing.T) {
	s := newTestStore(t)
	s.checkPin = func(pin.Pin) (bool, string) { return true, "" }
	kept := mustAssert(t, s, validInput("api"))
	other := mustAssert(t, s, validInput("cli"))
	gone := mustAssert(t, s, validInput("dead"))
	if _, err := s.Retract(gone.ID, "obsolete"); err != nil {
		t.Fatalf("Retract: %v", err)
	}

	// Whole-store check: retracted assertion is reported untouched, not counted.
	report, err := s.Check("")
	if err != nil {
		t.Fatalf("Check all: %v", err)
	}
	if report.Checked != 2 {
		t.Errorf("Checked = %d, want 2 (retracted skipped)", report.Checked)
	}
	if g, _ := s.Get(gone.ID); g.CheckedAt != nil {
		t.Error("retracted assertion CheckedAt should stay nil")
	}

	// Single-id check touches only that assertion.
	s2 := newTestStore(t)
	s2.checkPin = func(pin.Pin) (bool, string) { return true, "" }
	one := mustAssert(t, s2, validInput("one"))
	two := mustAssert(t, s2, validInput("two"))
	if _, err := s2.Check(one.ID); err != nil {
		t.Fatalf("Check one: %v", err)
	}
	if g, _ := s2.Get(two.ID); g.CheckedAt != nil {
		t.Errorf("unchecked assertion %s should have nil CheckedAt", two.ID)
	}
	if _, err := s2.Check("missing"); err != ErrNotFound {
		t.Errorf("Check missing id: want ErrNotFound, got %v", err)
	}
	_ = kept
	_ = other
}

// mustAssert asserts in and fails the test on error.
func mustAssert(t *testing.T, s *Store, in AssertInput) Assertion {
	t.Helper()
	a, err := s.Assert(in)
	if err != nil {
		t.Fatalf("Assert(%q): %v", in.Subject, err)
	}
	return a
}

// ids extracts the ids of a slice for readable failure messages.
func ids(as []Assertion) []string {
	out := make([]string, len(as))
	for i, a := range as {
		out[i] = a.ID
	}
	return out
}

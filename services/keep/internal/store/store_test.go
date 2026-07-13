package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// seedAssertion builds a persisted-shape assertion for writing directly into
// store files from tests.
func seedAssertion(id, subject, status string, updated time.Time) *Assertion {
	return &Assertion{
		ID:         id,
		Kind:       KindCodeBehavior,
		Subject:    subject,
		Statement:  "it does the thing",
		Pins:       []pin.Pin{testPin("a.go", 1, 3)},
		Confidence: ConfidenceDerived,
		Status:     status,
		CreatedAt:  updated,
		UpdatedAt:  updated,
	}
}

// jsonLine marshals one assertion as a single log line.
func jsonLine(t *testing.T, a *Assertion) string {
	t.Helper()
	raw, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal %s: %v", a.ID, err)
	}
	return string(raw) + "\n"
}

// fileLines reads path and returns its non-empty lines.
func fileLines(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []string
	for l := range strings.SplitSeq(string(raw), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

// captureLogf swaps the package logger for one that records messages,
// restoring it when the test ends.
func captureLogf(t *testing.T) *[]string {
	t.Helper()
	old := logf
	t.Cleanup(func() { logf = old })
	var msgs []string
	logf = func(format string, args ...any) {
		msgs = append(msgs, fmt.Sprintf(format, args...))
	}
	return &msgs
}

func TestMigrationSplitsLegacyArray(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	repoRec := seedAssertion("00000000000000000001-0000", "repo:mad01/x", StatusFresh, base)
	machRec := seedAssertion(
		"00000000000000000002-0000",
		"machine:yesyes/ollama",
		StatusFresh,
		base,
	)
	raw, err := json.Marshal([]*Assertion{repoRec, machRec})
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacyFileName), raw, 0o644); err != nil {
		t.Fatalf("seed legacy: %v", err)
	}

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if all := s.List(Filter{}); len(all) != 2 {
		t.Fatalf("migrated store has %d records, want 2: %v", len(all), ids(all))
	}
	if lines := fileLines(t, filepath.Join(dir, logFileName)); len(lines) != 1 {
		t.Errorf("%s has %d lines, want 1", logFileName, len(lines))
	}
	if lines := fileLines(t, filepath.Join(dir, localFileName)); len(lines) != 1 {
		t.Errorf("%s has %d lines, want 1", localFileName, len(lines))
	}
	if _, err := os.Stat(filepath.Join(dir, migratedFileName)); err != nil {
		t.Errorf("backup missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, legacyFileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("legacy file should be renamed away, stat err: %v", err)
	}

	// A second New is a no-op: same records, log bytes untouched.
	before, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if all := s2.List(Filter{}); len(all) != 2 {
		t.Errorf("reopened store has %d records, want 2", len(all))
	}
	after, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatalf("re-read log: %v", err)
	}
	if string(before) != string(after) {
		t.Error("second New rewrote the log")
	}
}

func TestMigrationLeavesStrayLegacyWhenLogExists(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	logged := seedAssertion("00000000000000000001-0000", "repo:a", StatusFresh, base)
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte(jsonLine(t, logged)), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	stray := seedAssertion("00000000000000000002-0000", "repo:b", StatusFresh, base)
	raw, err := json.Marshal([]*Assertion{stray})
	if err != nil {
		t.Fatalf("marshal stray: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, legacyFileName), raw, 0o644); err != nil {
		t.Fatalf("seed stray legacy: %v", err)
	}

	warnings := captureLogf(t)
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	all := s.List(Filter{})
	if len(all) != 1 || all[0].ID != logged.ID {
		t.Fatalf("log should win over stray legacy, got %v", ids(all))
	}
	if _, err := os.Stat(filepath.Join(dir, legacyFileName)); err != nil {
		t.Errorf("stray legacy file should be left alone: %v", err)
	}
	if len(*warnings) == 0 {
		t.Error("expected a warning about the stray legacy file")
	}
}

func TestLoadNewestPerIDWins(t *testing.T) {
	base := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	const id = "00000000000000000001-0000"

	// updated_at decides, not line order: newer record first.
	dir := t.TempDir()
	newer := seedAssertion(id, "repo:a", StatusStale, base.Add(time.Minute))
	older := seedAssertion(id, "repo:a", StatusFresh, base)
	content := jsonLine(t, newer) + jsonLine(t, older)
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, _ := s.Get(id); got.Status != StatusStale {
		t.Errorf("newer record should win: status = %s, want %s", got.Status, StatusStale)
	}

	// Equal updated_at: the later line wins.
	dir2 := t.TempDir()
	first := seedAssertion(id, "repo:b", StatusFresh, base)
	second := seedAssertion(id, "repo:b", StatusStale, base)
	content2 := jsonLine(t, first) + jsonLine(t, second)
	if err := os.WriteFile(filepath.Join(dir2, logFileName), []byte(content2), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	s2, err := New(dir2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, _ := s2.Get(id); got.Status != StatusStale {
		t.Errorf("later line should win on equal updated_at: status = %s", got.Status)
	}
}

func TestWritesAppendLines(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.now = incrClock(time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))
	s.rnd = zeroReader{}

	a := mustAssert(t, s, validInput("repo:api"))
	logPath := filepath.Join(dir, logFileName)
	initial := fileLines(t, logPath)
	if len(initial) != 1 {
		t.Fatalf("assert wrote %d lines, want 1", len(initial))
	}

	if _, err := s.Retract(a.ID, "obsolete"); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	lines := fileLines(t, logPath)
	if len(lines) != 2 {
		t.Fatalf("retract should append one line, log has %d", len(lines))
	}
	if lines[0] != initial[0] {
		t.Error("retract rewrote an earlier line")
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if got, _ := s2.Get(a.ID); got.Status != StatusRetracted {
		t.Errorf("reloaded status = %s, want %s", got.Status, StatusRetracted)
	}
}

func TestCheckAppendsPerCheckedRecord(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.now = incrClock(time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))
	s.rnd = zeroReader{}
	s.checkPin = func(pin.Pin) (bool, string) { return true, "" }

	mustAssert(t, s, validInput("repo:a"))
	mustAssert(t, s, validInput("repo:b"))
	if _, err := s.Check(""); err != nil {
		t.Fatalf("Check: %v", err)
	}
	lines := fileLines(t, filepath.Join(dir, logFileName))
	if len(lines) != 4 {
		t.Errorf("2 asserts + 2 checked records should be 4 lines, got %d", len(lines))
	}
}

func TestMachineSubjectsRouteToLocalFile(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.now = incrClock(time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))
	s.rnd = zeroReader{}

	m := mustAssert(t, s, validInput("machine:yesyes/disk"))
	mustAssert(t, s, validInput("repo:api"))
	if lines := fileLines(t, filepath.Join(dir, localFileName)); len(lines) != 1 {
		t.Errorf("%s has %d lines, want 1", localFileName, len(lines))
	}
	if lines := fileLines(t, filepath.Join(dir, logFileName)); len(lines) != 1 {
		t.Errorf("%s has %d lines, want 1", logFileName, len(lines))
	}

	// Updates follow the record's file: retracting the machine record appends
	// to local.jsonl only.
	if _, err := s.Retract(m.ID, "moved on"); err != nil {
		t.Fatalf("Retract: %v", err)
	}
	if lines := fileLines(t, filepath.Join(dir, localFileName)); len(lines) != 2 {
		t.Errorf("%s has %d lines after retract, want 2", localFileName, len(lines))
	}
	if lines := fileLines(t, filepath.Join(dir, logFileName)); len(lines) != 1 {
		t.Errorf("%s has %d lines after retract, want 1", logFileName, len(lines))
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if all := s2.List(Filter{}); len(all) != 2 {
		t.Errorf("reopened store has %d records, want 2", len(all))
	}
}

func TestLoadRepairsTornFinalLine(t *testing.T) {
	base := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	good := seedAssertion("00000000000000000001-0000", "repo:a", StatusFresh, base)

	// Unparseable torn tail: dropped with a warning and truncated away so the
	// next append starts a clean line.
	dir := t.TempDir()
	content := jsonLine(t, good) + `{"id":"torn`
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	warnings := captureLogf(t)
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if all := s.List(Filter{}); len(all) != 1 {
		t.Fatalf("store has %d records, want 1", len(all))
	}
	if len(*warnings) == 0 {
		t.Error("expected a torn-line warning")
	}
	raw, err := os.ReadFile(filepath.Join(dir, logFileName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(raw) != jsonLine(t, good) {
		t.Errorf("torn tail not truncated, log is %q", raw)
	}

	// Parseable final line missing its newline: the record is kept and the
	// newline is added so the next append cannot merge into it.
	dir2 := t.TempDir()
	tail := strings.TrimSuffix(jsonLine(t, good), "\n")
	if err := os.WriteFile(filepath.Join(dir2, logFileName), []byte(tail), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	s2, err := New(dir2)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if all := s2.List(Filter{}); len(all) != 1 {
		t.Fatalf("store has %d records, want 1", len(all))
	}
	raw2, err := os.ReadFile(filepath.Join(dir2, logFileName))
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.HasSuffix(string(raw2), "\n") {
		t.Error("missing trailing newline was not repaired")
	}
}

func TestLoadFailsOnCorruptLine(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)
	good := seedAssertion("00000000000000000001-0000", "repo:a", StatusFresh, base)
	content := jsonLine(t, good) + "not json\n" + jsonLine(t, good)
	if err := os.WriteFile(filepath.Join(dir, logFileName), []byte(content), 0o644); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	_, err := New(dir)
	if err == nil {
		t.Fatal("New should fail on a corrupt mid-file line")
	}
	if !strings.Contains(err.Error(), logFileName+":2") {
		t.Errorf("error should name file and line, got: %v", err)
	}
}

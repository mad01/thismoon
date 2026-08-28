package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/events/internal/event"
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

// incrClock returns a clock that advances 1ms per call, so each Append gets a
// distinct, monotonically increasing timestamp (and id).
func incrClock(start time.Time) func() time.Time {
	cur := start
	return func() time.Time {
		t := cur
		cur = cur.Add(time.Millisecond)
		return t
	}
}

func newTestStore(t *testing.T, perCap, globalCap int) *Store {
	t.Helper()
	s, err := New(t.TempDir(), perCap, globalCap, incrClock(time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s.rand = zeroReader{}
	return s
}

func mustAppend(t *testing.T, s *Store, ev event.Event) event.Event {
	t.Helper()
	got, err := s.Append(ev)
	if err != nil {
		t.Fatalf("Append(%q): %v", ev.Title, err)
	}
	return got
}

func diskLineCount(t *testing.T, path string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	n := 0
	for _, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) > 0 {
			n++
		}
	}
	return n
}

func TestPurgeWholeSource(t *testing.T) {
	s := newTestStore(t, 0, 0)
	for i := 0; i < 3; i++ {
		mustAppend(t, s, event.Event{Source: "junk", Title: "j"})
	}
	mustAppend(t, s, event.Event{Source: "keep", Title: "k"})

	n, err := s.Purge("junk", "")
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 3 {
		t.Errorf("purged = %d, want 3", n)
	}
	if got := s.Query(Filter{Source: "junk"}); len(got) != 0 {
		t.Errorf("junk still has %d events", len(got))
	}
	if _, err := os.Stat(filepath.Join(s.dir, "junk.jsonl")); !os.IsNotExist(err) {
		t.Errorf("junk.jsonl should be removed, stat err = %v", err)
	}
	if got := s.Query(Filter{Source: "keep"}); len(got) != 1 {
		t.Errorf("keep has %d events, want 1", len(got))
	}
}

func TestPurgeBeforeCursor(t *testing.T) {
	s := newTestStore(t, 0, 0)
	var evs []event.Event
	for i := 0; i < 4; i++ {
		evs = append(evs, mustAppend(t, s, event.Event{Source: "a", Title: "t"}))
	}

	// Drop the two oldest; the cursor is inclusive.
	n, err := s.Purge("a", evs[1].ID)
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 2 {
		t.Errorf("purged = %d, want 2", n)
	}
	got := s.Query(Filter{Source: "a"})
	if len(got) != 2 {
		t.Fatalf("kept %d events, want 2", len(got))
	}
	if got[0].ID != evs[3].ID || got[1].ID != evs[2].ID {
		t.Errorf("kept wrong events: %v", got)
	}
	// The file is compacted to exactly the kept events.
	if n := diskLineCount(t, filepath.Join(s.dir, "a.jsonl")); n != 2 {
		t.Errorf("disk lines = %d, want 2", n)
	}
}

func TestPurgeUnknownSourceIsNoop(t *testing.T) {
	s := newTestStore(t, 0, 0)
	n, err := s.Purge("nope", "")
	if err != nil {
		t.Fatalf("Purge: %v", err)
	}
	if n != 0 {
		t.Errorf("purged = %d, want 0", n)
	}
}

func TestAppendValidates(t *testing.T) {
	s := newTestStore(t, 0, 0)
	if _, err := s.Append(event.Event{Title: "x"}); err == nil {
		t.Error("missing source should fail")
	}
	if _, err := s.Append(event.Event{Source: "a"}); err == nil {
		t.Error("missing title should fail")
	}
	if _, err := s.Append(event.Event{Source: "a", Title: "x", Level: "fatal"}); err == nil {
		t.Error("bad level should fail")
	}
}

func TestAppendStampsAndDefaults(t *testing.T) {
	s := newTestStore(t, 0, 0)
	got := mustAppend(t, s, event.Event{Source: "DepS", Title: "hi"})
	if got.ID == "" {
		t.Error("ID should be stamped")
	}
	if got.Time.IsZero() {
		t.Error("Time should be stamped")
	}
	if got.Level != event.LevelInfo {
		t.Errorf("default level = %q, want info", got.Level)
	}
	if got.Source != "deps" {
		t.Errorf("source should be sanitized: got %q", got.Source)
	}
}

func TestQueryNewestFirst(t *testing.T) {
	s := newTestStore(t, 0, 0)
	mustAppend(t, s, event.Event{Source: "a", Title: "first"})
	mustAppend(t, s, event.Event{Source: "a", Title: "second"})
	mustAppend(t, s, event.Event{Source: "a", Title: "third"})

	got := s.Query(Filter{})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].Title != "third" || got[2].Title != "first" {
		t.Errorf("not newest-first: %q … %q", got[0].Title, got[2].Title)
	}
}

func TestPerSourceCapEviction(t *testing.T) {
	s := newTestStore(t, 500, 1500)
	for i := 0; i < 600; i++ {
		mustAppend(t, s, event.Event{Source: "bulk", Title: "e" + itoa(i)})
	}
	got := s.Query(Filter{Source: "bulk"})
	if len(got) != 500 {
		t.Fatalf("in-memory count = %d, want 500", len(got))
	}
	// Newest kept, oldest evicted.
	if got[0].Title != "e599" {
		t.Errorf("newest = %q, want e599", got[0].Title)
	}
	for _, ev := range got {
		if ev.Title == "e0" {
			t.Fatal("oldest event e0 should have been evicted")
		}
	}
}

func TestCompactionBoundsDisk(t *testing.T) {
	s := newTestStore(t, 5, 1500)
	for i := 0; i < 20; i++ {
		mustAppend(t, s, event.Event{Source: "c", Title: "e" + itoa(i)})
	}
	perCap := 5
	threshold := int(float64(perCap) * compactFactor)
	lines := diskLineCount(t, s.filePath("c"))
	if lines > threshold {
		t.Errorf("on-disk lines = %d, want <= %d after compaction", lines, threshold)
	}
	if got := s.Query(Filter{Source: "c"}); len(got) != 5 {
		t.Errorf("in-memory count = %d, want 5", len(got))
	}
}

func TestGlobalCap(t *testing.T) {
	s := newTestStore(t, 100, 10)
	for _, src := range []string{"a", "b", "c"} {
		for i := 0; i < 8; i++ {
			mustAppend(t, s, event.Event{Source: src, Title: "x"})
		}
	}
	got := s.Query(Filter{}) // 24 events, capped to 10
	if len(got) != 10 {
		t.Fatalf("len = %d, want 10 (global cap)", len(got))
	}
	if got[0].ID <= got[len(got)-1].ID {
		t.Error("result should be newest-first across sources")
	}
}

func TestQueryFilters(t *testing.T) {
	s := newTestStore(t, 0, 0)
	mustAppend(t, s, event.Event{Source: "a", Title: "alpha", Level: "info"})
	e2 := mustAppend(t, s, event.Event{Source: "a", Title: "beta", Level: "warn", Message: "needle here"})
	mustAppend(t, s, event.Event{Source: "b", Title: "gamma", Level: "error", Tags: map[string]string{"repo": "needle-repo"}})

	if got := s.Query(Filter{Source: "a"}); len(got) != 2 {
		t.Errorf("source filter: len = %d, want 2", len(got))
	}
	if got := s.Query(Filter{Level: "error"}); len(got) != 1 || got[0].Title != "gamma" {
		t.Errorf("level filter: %+v", got)
	}
	if got := s.Query(Filter{Q: "NEEDLE"}); len(got) != 2 {
		t.Errorf("q substring (message+tags, case-insensitive): len = %d, want 2", len(got))
	}
	since := s.Query(Filter{Since: e2.ID})
	if len(since) != 1 || since[0].Title != "gamma" {
		t.Errorf("since-exclusive: %+v", since)
	}
	if got := s.Query(Filter{Limit: 1}); len(got) != 1 {
		t.Errorf("limit: len = %d, want 1", len(got))
	}
}

func TestQueryBeforeCursor(t *testing.T) {
	s := newTestStore(t, 0, 0)
	var evs []event.Event
	for i := 0; i < 5; i++ {
		evs = append(evs, mustAppend(t, s, event.Event{Source: "a", Title: "e" + itoa(i)}))
	}

	// Before is exclusive: only events strictly older than the cursor, newest-first.
	got := s.Query(Filter{Before: evs[3].ID})
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	if got[0].ID != evs[2].ID || got[2].ID != evs[0].ID {
		t.Errorf("not newest-first below cursor: %+v", got)
	}

	// Limit pages the window: the newest page under the cursor.
	page := s.Query(Filter{Before: evs[4].ID, Limit: 2})
	if len(page) != 2 || page[0].ID != evs[3].ID || page[1].ID != evs[2].ID {
		t.Errorf("before+limit page: %+v", page)
	}

	// Since and Before combine to the open interval (since, before).
	mid := s.Query(Filter{Since: evs[0].ID, Before: evs[4].ID})
	if len(mid) != 3 || mid[0].ID != evs[3].ID || mid[2].ID != evs[1].ID {
		t.Errorf("since+before window: %+v", mid)
	}

	// A cursor at the oldest id yields nothing.
	if got := s.Query(Filter{Before: evs[0].ID}); len(got) != 0 {
		t.Errorf("cursor at oldest: len = %d, want 0", len(got))
	}
}

func TestLoadAfterRestart(t *testing.T) {
	dir := t.TempDir()
	clock := incrClock(time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC))
	s1, err := New(dir, 5, 1500, clock)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	s1.rand = zeroReader{}
	for i := 0; i < 10; i++ {
		mustAppend(t, s1, event.Event{Source: "x", Title: "ev" + itoa(i)})
	}

	s2, err := New(dir, 5, 1500, nil)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	got := s2.Query(Filter{Source: "x"})
	if len(got) != 5 {
		t.Fatalf("reloaded count = %d, want 5 (capped)", len(got))
	}
	if got[0].Title != "ev9" {
		t.Errorf("newest reloaded = %q, want ev9", got[0].Title)
	}
	for _, ev := range got {
		if ev.Title == "ev0" {
			t.Fatal("evicted ev0 should not survive reload")
		}
	}
}

func TestLoadSkipsMalformedLine(t *testing.T) {
	dir := t.TempDir()
	sources := filepath.Join(dir, sourcesSubdir)
	if err := os.MkdirAll(sources, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	good := event.Event{
		ID:     "00000000000000000001-0000",
		Time:   time.Date(2026, 6, 29, 9, 0, 0, 0, time.UTC),
		Source: "test",
		Level:  "info",
		Title:  "good",
	}
	raw, _ := json.Marshal(good)
	content := "{ this is not json\n" + string(raw) + "\n"
	if err := os.WriteFile(filepath.Join(sources, "test.jsonl"), []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := New(dir, 0, 0, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got := s.Query(Filter{Source: "test"})
	if len(got) != 1 || got[0].Title != "good" {
		t.Fatalf("malformed line not skipped cleanly: %+v", got)
	}
}

func TestSources(t *testing.T) {
	s := newTestStore(t, 0, 0)
	mustAppend(t, s, event.Event{Source: "zeta", Title: "x"})
	mustAppend(t, s, event.Event{Source: "alpha", Title: "x"})
	mustAppend(t, s, event.Event{Source: "alpha", Title: "y"})

	got := s.Sources()
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Source != "alpha" || got[0].Count != 2 {
		t.Errorf("sorted/count wrong: %+v", got[0])
	}
	if got[1].Source != "zeta" || got[1].Count != 1 {
		t.Errorf("second source wrong: %+v", got[1])
	}
}

// itoa is a tiny strconv.Itoa to keep the test imports lean.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

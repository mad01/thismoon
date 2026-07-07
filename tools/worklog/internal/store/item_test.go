package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	tick := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	s := New(dir)
	s.Now = func() time.Time { now := tick; tick = tick.Add(time.Hour); return now }
	return s
}

func mustCheckpoint(t *testing.T, s *Store, key string, in CheckpointInput) {
	t.Helper()
	if _, err := s.Checkpoint(key, in); err != nil {
		t.Fatal(err)
	}
}

func mustSetStatus(t *testing.T, s *Store, key, status string) {
	t.Helper()
	if _, err := s.SetStatus(key, status); err != nil {
		t.Fatal(err)
	}
}

func TestCheckpointCreatesAndGroups(t *testing.T) {
	s := testStore(t)

	it, err := s.Checkpoint("ABC-1234", CheckpointInput{
		Ticket: "ABC-1234", Where: "wiring the store", Note: "started", Repo: "dotfiles", Cwd: "/tmp/x",
	})
	if err != nil {
		t.Fatal(err)
	}
	if it.FM.Status != "active" || it.FM.Ticket != "ABC-1234" {
		t.Fatalf("unexpected frontmatter: %+v", it.FM)
	}
	if it.FM.LastCwd != "/tmp/x" || len(it.FM.Repos) != 1 || it.FM.Repos[0] != "dotfiles" {
		t.Fatalf("cwd/repo not recorded: %+v", it.FM)
	}

	// Second checkpoint on a different repo: same item, repos grow, no dup.
	if _, err := s.Checkpoint("ABC-1234", CheckpointInput{Note: "ralph side", Repo: "ralph"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Checkpoint("ABC-1234", CheckpointInput{Note: "more ralph", Repo: "ralph"}); err != nil {
		t.Fatal(err)
	}
	it, _ = s.Load("ABC-1234")
	if len(it.FM.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %v", it.FM.Repos)
	}

	// Per-repo note file exists with both ralph entries.
	note, err := s.RepoNote("ABC-1234", "ralph")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(note, "### ") != 2 {
		t.Fatalf("expected 2 ralph entries, got:\n%s", note)
	}
}

func TestWhereIamReplacedLogAppended(t *testing.T) {
	s := testStore(t)
	mustCheckpoint(t, s, "topic", CheckpointInput{Where: "first state", Note: "n1", Repo: "a"})
	mustCheckpoint(t, s, "topic", CheckpointInput{Where: "second state", Note: "n2", Repo: "a"})

	it, _ := s.Load("topic")
	if strings.Contains(it.Body, "first state") {
		t.Fatal("Where I am should be replaced, not appended")
	}
	if !strings.Contains(it.Body, "second state") {
		t.Fatal("latest Where I am missing")
	}
	if !strings.Contains(it.Body, "n1") || !strings.Contains(it.Body, "n2") {
		t.Fatal("log should keep both entries")
	}
	// Newest log entry first.
	if strings.Index(it.Body, "n2") > strings.Index(it.Body, "n1") {
		t.Fatal("log not reverse-chronological")
	}
}

func TestListFiltersAndStatus(t *testing.T) {
	s := testStore(t)
	mustCheckpoint(t, s, "a", CheckpointInput{Note: "x", Repo: "dotfiles"})
	mustCheckpoint(t, s, "b", CheckpointInput{Note: "y", Repo: "ralph"})
	if _, err := s.SetStatus("b", "done"); err != nil {
		t.Fatal(err)
	}

	active, _ := s.List("active", "")
	if len(active) != 1 || active[0].FM.Key != "a" {
		t.Fatalf("active filter wrong: %+v", active)
	}
	byRepo, _ := s.List("", "ralph")
	if len(byRepo) != 1 || byRepo[0].FM.Key != "b" {
		t.Fatalf("repo filter wrong: %+v", byRepo)
	}
}

func TestSearchActiveFirst(t *testing.T) {
	s := testStore(t)
	mustCheckpoint(t, s, "old-one", CheckpointInput{Note: "kubernetes work", Repo: "a"})
	mustSetStatus(t, s, "old-one", "done")
	mustCheckpoint(t, s, "new-one", CheckpointInput{Note: "kubernetes again", Repo: "b"})

	hits, _ := s.Search("kubernetes")
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(hits))
	}
	if hits[0].FM.Key != "new-one" {
		t.Fatalf("active item should rank first, got %s", hits[0].FM.Key)
	}
}

func TestSanitize(t *testing.T) {
	cases := map[string]string{
		"ABC-1234":       "ABC-1234",
		"webkit theme!":  "webkit-theme",
		"  spaced  out ": "spaced-out",
		"":               "untitled",
	}
	for in, want := range cases {
		if got := Sanitize(in); got != want {
			t.Errorf("Sanitize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRoundTripFrontmatter(t *testing.T) {
	s := testStore(t)
	mustCheckpoint(t, s, "rt", CheckpointInput{Where: "w", Note: "n", Repo: "r", Ticket: "ABC-9"})
	raw, err := os.ReadFile(filepath.Join(s.Root, "rt", "CONTEXT.md"))
	if err != nil {
		t.Fatal(err)
	}
	it, err := parseItem(raw)
	if err != nil {
		t.Fatal(err)
	}
	if it.FM.Ticket != "ABC-9" || it.FM.Key != "rt" {
		t.Fatalf("round-trip lost data: %+v", it.FM)
	}
}

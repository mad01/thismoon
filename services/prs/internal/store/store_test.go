package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var base = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

func seedStates() []RepoState {
	return []RepoState{
		{
			Host: "github.com", Repo: "org/alpha", FetchedAt: base,
			PRs: []PR{
				{
					Host: "github.com", Repo: "org/alpha", Number: 1, Author: "ada",
					CreatedAt: base.Add(-48 * time.Hour), ReviewDecision: "APPROVED",
				},
				{
					Host: "github.com", Repo: "org/alpha", Number: 2, Author: "bo",
					CreatedAt: base.Add(-1 * time.Hour),
				},
			},
		},
		{
			Host: "git.example.com", Repo: "org/beta", FetchedAt: base,
			PRs: []PR{
				{
					Host: "git.example.com", Repo: "org/beta", Number: 7, Author: "ada",
					CreatedAt: base.Add(-24 * time.Hour), ReviewDecision: "CHANGES_REQUESTED",
				},
			},
		},
		{Host: "github.com", Repo: "org/broken", FetchedAt: base, Error: "boom"},
	}
}

func seed(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepos(seedStates(), base); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestListSortsNewestFirstByDefault(t *testing.T) {
	s := seed(t)
	prs := s.List(Filter{})
	if len(prs) != 3 {
		t.Fatalf("got %d PRs, want 3", len(prs))
	}
	if prs[0].Number != 2 || prs[2].Number != 1 {
		t.Errorf("wrong newest-first order: %v", numbers(prs))
	}
}

func TestListSortsOldest(t *testing.T) {
	s := seed(t)
	prs := s.List(Filter{Sort: "oldest"})
	if prs[0].Number != 1 || prs[2].Number != 2 {
		t.Errorf("wrong oldest-first order: %v", numbers(prs))
	}
}

func TestListFilters(t *testing.T) {
	s := seed(t)
	if got := s.List(Filter{Repo: "org/alpha"}); len(got) != 2 {
		t.Errorf("repo filter: got %d, want 2", len(got))
	}
	if got := s.List(Filter{Author: "ada"}); len(got) != 2 {
		t.Errorf("author filter: got %d, want 2", len(got))
	}
	if got := s.List(Filter{ReviewDecision: "APPROVED"}); len(got) != 1 || got[0].Number != 1 {
		t.Errorf("review filter: got %v", numbers(got))
	}
	if got := s.List(Filter{Repo: "org/alpha", Author: "ada"}); len(got) != 1 {
		t.Errorf("combined filter: got %d, want 1", len(got))
	}
}

func TestFacets(t *testing.T) {
	s := seed(t)
	repos, authors := s.Facets()
	if len(repos) != 2 || repos[0] != "org/alpha" || repos[1] != "org/beta" {
		t.Errorf("repos = %v", repos)
	}
	if len(authors) != 2 || authors[0] != "ada" || authors[1] != "bo" {
		t.Errorf("authors = %v", authors)
	}
}

func TestStatus(t *testing.T) {
	s := seed(t)
	st := s.Status()
	if st.Repos != 3 || st.OpenPRs != 3 {
		t.Errorf("Repos=%d OpenPRs=%d, want 3/3", st.Repos, st.OpenPRs)
	}
	if len(st.Errors) != 1 || st.Errors[0].Repo != "org/broken" {
		t.Errorf("Errors = %v", st.Errors)
	}
	if !st.PolledAt.Equal(base) {
		t.Errorf("PolledAt = %s", st.PolledAt)
	}
	if len(st.Hosts) != 2 {
		t.Errorf("Hosts = %v", st.Hosts)
	}
}

func TestLogRoundTripNewestWins(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepos(seedStates(), base); err != nil {
		t.Fatal(err)
	}
	// Second cycle: alpha loses PR #2 — the reloaded store must see the
	// newer record, not the first one.
	later := base.Add(time.Hour)
	updated := seedStates()
	updated[0].PRs = updated[0].PRs[:1]
	updated[0].FetchedAt = later
	if err := s.SetRepos(updated, later); err != nil {
		t.Fatal(err)
	}

	reloaded, err := New(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	prs := reloaded.List(Filter{Repo: "org/alpha"})
	if len(prs) != 1 || prs[0].Number != 1 {
		t.Errorf("reloaded alpha PRs = %v, want [1]", numbers(prs))
	}
	if !reloaded.Status().PolledAt.Equal(later) {
		t.Errorf("PolledAt = %s, want %s", reloaded.Status().PolledAt, later)
	}
}

func TestQuietCycleAppendsNothing(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepos(seedStates(), base); err != nil {
		t.Fatal(err)
	}
	before := logLines(t, dir)

	// Same content, only fetched_at advanced: no new lines.
	again := seedStates()
	for i := range again {
		again[i].FetchedAt = base.Add(time.Hour)
	}
	if err := s.SetRepos(again, base.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if after := logLines(t, dir); after != before {
		t.Errorf("quiet cycle grew the log: %d -> %d lines", before, after)
	}
}

func TestUndiscoveredRepoGetsTombstone(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepos(seedStates(), base); err != nil {
		t.Fatal(err)
	}
	// beta disappears from discovery.
	later := base.Add(time.Hour)
	remaining := seedStates()[:1]
	if err := s.SetRepos(remaining, later); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Repo("git.example.com/org/beta"); ok {
		t.Error("tombstoned repo still reported live")
	}
	if st := s.Status(); st.Repos != 1 {
		t.Errorf("Repos = %d, want 1 after tombstones", st.Repos)
	}

	reloaded, err := New(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := reloaded.Repo("git.example.com/org/beta"); ok {
		t.Error("tombstone did not survive reload")
	}
}

func TestTornFinalLineTruncated(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetRepos(seedStates()[:1], base); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "repos.jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(`{"host":"github.com","repo":"org/t`); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	reloaded, err := New(dir)
	if err != nil {
		t.Fatalf("reload with torn tail: %v", err)
	}
	if st := reloaded.Status(); st.Repos != 1 {
		t.Errorf("Repos = %d, want 1", st.Repos)
	}
}

func TestMidFileCorruptionFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repos.jsonl")
	content := "{torn\n" + `{"host":"github.com","repo":"org/a","fetched_at":"2026-08-01T12:00:00Z"}` + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(dir); err == nil {
		t.Fatal("expected error for mid-file corruption")
	}
}

func TestCompactionShrinksTheLog(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	// One repo flip-flopping produces many superseded lines.
	for i := range 202 {
		st := seedStates()[:1]
		st[0].FetchedAt = base.Add(time.Duration(i) * time.Minute)
		st[0].Error = ""
		if i%2 == 0 {
			st[0].Error = "flap"
		}
		if err := s.SetRepos(st, st[0].FetchedAt); err != nil {
			t.Fatal(err)
		}
	}
	if lines := logLines(t, dir); lines <= 100 {
		t.Fatalf("setup produced only %d lines", lines)
	}

	reloaded, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	if lines := logLines(t, dir); lines != 1 {
		t.Errorf("compaction left %d lines, want 1", lines)
	}
	if st := reloaded.Status(); st.Repos != 1 {
		t.Errorf("Repos = %d, want 1 after compaction", st.Repos)
	}
}

func TestValidSort(t *testing.T) {
	if !ValidSort("newest") || !ValidSort("oldest") {
		t.Error("newest/oldest should be valid")
	}
	if ValidSort("sideways") {
		t.Error("sideways should be invalid")
	}
}

func logLines(t *testing.T, dir string) int {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "repos.jsonl"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Count(raw, []byte("\n"))
}

func numbers(prs []PR) []int {
	out := make([]int, len(prs))
	for i, pr := range prs {
		out[i] = pr.Number
	}
	return out
}

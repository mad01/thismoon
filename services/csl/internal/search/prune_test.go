package search

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/sourcegraph/zoekt"
	zoektsearch "github.com/sourcegraph/zoekt/search"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// seedIndex indexes repos into indexDir in order and records each one in
// state.json, the way a sync leaves them.
func seedIndex(t *testing.T, indexDir string, repos ...finder.Repo) {
	t.Helper()
	if err := IndexRepos(indexDir, repos, nil, nil); err != nil {
		t.Fatalf("IndexRepos: %v", err)
	}
	state, err := LoadState(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range repos {
		fp, err := Fingerprint(r.Path)
		if err != nil {
			t.Fatal(err)
		}
		state.SetRepo(r.Path, fp)
	}
	if err := state.Save(indexDir); err != nil {
		t.Fatal(err)
	}
}

// shardsNaming returns the readable shards in indexDir holding repo name.
func shardsNaming(t *testing.T, indexDir, name string) []string {
	t.Helper()
	shards, _, err := ValidateShards(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range shards {
		if slices.Contains(s.Repos, name) {
			out = append(out, s.Path)
		}
	}
	return out
}

// searchRepos returns the repo names a query for "hello" hits.
func searchRepos(t *testing.T, searcher zoekt.Searcher) []string {
	t.Helper()
	matches, err := SearchWith(context.Background(), searcher, SearchOptions{Pattern: "hello"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var repos []string
	for _, m := range matches {
		if !slices.Contains(repos, m.Repo) {
			repos = append(repos, m.Repo)
		}
	}
	slices.Sort(repos)
	return repos
}

func assertExists(t *testing.T, path string, want bool) {
	t.Helper()
	_, err := os.Stat(path)
	if got := err == nil; got != want {
		t.Errorf("%s exists = %v, want %v (stat err: %v)", filepath.Base(path), got, want, err)
	}
}

func TestPruneOrphans_RemovesRepoNotKept(t *testing.T) {
	gitAvailable(t)
	keep := finder.Repo{Name: "org/keep", Path: initGitRepo(t, t.TempDir())}
	gone := finder.Repo{Name: "org/gone", Path: initGitRepo(t, t.TempDir())}
	indexDir := t.TempDir()
	seedIndex(t, indexDir, keep, gone)

	goneShards := shardsNaming(t, indexDir, gone.Name)
	keepShards := shardsNaming(t, indexDir, keep.Name)
	if len(goneShards) == 0 || len(keepShards) == 0 {
		t.Fatalf("seeded shards: gone %v, keep %v", goneShards, keepShards)
	}
	sidecar, err := json.Marshal(zoekt.Repository{Name: gone.Name, Source: gone.Path})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(goneShards[0]+".meta", sidecar, 0o644); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(indexDir, ".csl-sync.lock")
	junk := filepath.Join(indexDir, "org%2Fjunk_v16.00000.zoekt")
	for _, f := range []string{lock, junk} {
		if err := os.WriteFile(f, []byte("123"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// A searcher opened before the prune stands in for the running daemon.
	daemon, err := zoektsearch.NewDirectorySearcher(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	defer daemon.Close()

	removed, err := PruneOrphans(indexDir, []finder.Repo{keep})
	if err != nil {
		t.Fatalf("PruneOrphans: %v", err)
	}

	if !slices.Equal(removed, goneShards) {
		t.Errorf("removed = %v, want %v", removed, goneShards)
	}
	for _, s := range goneShards {
		assertExists(t, s, false)
		assertExists(t, s+".meta", false)
	}
	for _, f := range append(keepShards, lock, junk) {
		assertExists(t, f, true)
	}

	state, err := LoadState(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.GetRepo(gone.Path); ok {
		t.Error("state still holds the pruned repo")
	}
	if _, ok := state.GetRepo(keep.Path); !ok {
		t.Error("state lost the kept repo")
	}

	fresh, err := zoektsearch.NewDirectorySearcher(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	if got := searchRepos(t, fresh); !slices.Equal(got, []string{keep.Name}) {
		t.Errorf("fresh search hit repos %v, want only %s", got, keep.Name)
	}
	waitForRepos(t, daemon, []string{keep.Name})
}

// waitForRepos polls a long-lived searcher until "hello" hits exactly want,
// since zoekt unloads a deleted shard asynchronously on a directory event.
func waitForRepos(t *testing.T, searcher zoekt.Searcher, want []string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		got := searchRepos(t, searcher)
		if slices.Equal(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("long-lived searcher still hits %v, want %v", got, want)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestPruneOrphans_KeepsShardRebuiltByKeptCheckout(t *testing.T) {
	gitAvailable(t)
	// Two checkouts of one remote share an org/repo name and so a shard file
	// name; the later index run overwrote the shard with the kept checkout.
	old := finder.Repo{Name: "org/same", Path: initGitRepo(t, t.TempDir())}
	kept := finder.Repo{Name: "org/same", Path: initGitRepo(t, t.TempDir())}
	indexDir := t.TempDir()
	seedIndex(t, indexDir, old, kept)
	shards := shardsNaming(t, indexDir, kept.Name)

	removed, err := PruneOrphans(indexDir, []finder.Repo{kept})
	if err != nil {
		t.Fatalf("PruneOrphans: %v", err)
	}

	if len(removed) != 0 {
		t.Errorf("removed = %v, want none", removed)
	}
	for _, s := range shards {
		assertExists(t, s, true)
	}
	state, err := LoadState(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := state.GetRepo(old.Path); ok {
		t.Error("state still holds the pruned checkout")
	}
	if _, ok := state.GetRepo(kept.Path); !ok {
		t.Error("state lost the kept checkout")
	}
}

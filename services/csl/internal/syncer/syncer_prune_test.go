package syncer

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// isolateState points csl's state directory at a temp dir, so Run's index,
// lock, and queue never touch the real ones, and returns the index dir.
func isolateState(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		t.Fatal(err)
	}
	return indexDir
}

// seedIndex indexes repos into indexDir and records each in state.json, the
// way an earlier sync leaves them.
func seedIndex(t *testing.T, indexDir string, repos ...finder.Repo) {
	t.Helper()
	if err := search.IndexRepos(indexDir, repos, nil, nil); err != nil {
		t.Fatalf("IndexRepos: %v", err)
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range repos {
		fp, err := search.Fingerprint(r.Path)
		if err != nil {
			t.Fatal(err)
		}
		state.SetRepo(r.Path, fp)
	}
	if err := state.Save(indexDir); err != nil {
		t.Fatal(err)
	}
}

// indexedRepos returns the repo paths state.json records, sorted.
func indexedRepos(t *testing.T, indexDir string) []string {
	t.Helper()
	state, err := search.LoadState(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for p := range state.Repos {
		paths = append(paths, p)
	}
	slices.Sort(paths)
	return paths
}

// shardedRepos returns the repo names the readable shards hold, sorted.
func shardedRepos(t *testing.T, indexDir string) []string {
	t.Helper()
	shards, _, err := search.ValidateShards(indexDir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range shards {
		names = append(names, s.Repos...)
	}
	slices.Sort(names)
	return names
}

// discoverOne returns the single repo cfg discovers.
func discoverOne(t *testing.T, cfg *config.Config) finder.Repo {
	t.Helper()
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 {
		t.Fatalf("discovered %d repos, want 1: %v", len(repos), repos)
	}
	return repos[0]
}

// vanishedRepo indexes a checkout outside every configured dir and then
// deletes it, leaving a state entry and shard for a path that no longer
// exists.
func vanishedRepo(t *testing.T, indexDir string, alongside ...finder.Repo) finder.Repo {
	t.Helper()
	path := setupSyncRepo(t, "gone", "main", false, withNoRemote)
	gone := finder.Repo{Name: "org/gone", Path: path}
	seedIndex(t, indexDir, append(alongside, gone)...)
	if err := os.RemoveAll(filepath.Dir(path)); err != nil {
		t.Fatal(err)
	}
	return gone
}

func TestRun_FullRunPrunesVanishedRepo(t *testing.T) {
	indexDir := isolateState(t)
	keptPath := setupSyncRepo(t, "kept", "main", false, withNoRemote)
	cfg := &config.Config{Dirs: []string{filepath.Dir(keptPath)}}
	kept := discoverOne(t, cfg)
	gone := vanishedRepo(t, indexDir, kept)

	var out bytes.Buffer
	rep, err := Run(context.Background(), cfg, Options{Out: &out})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if rep.Pruned != 1 {
		t.Errorf("Report.Pruned = %d, want 1", rep.Pruned)
	}
	if !strings.Contains(out.String(), "pruned 1 orphan shard(s)") {
		t.Errorf("output does not report the prune:\n%s", out.String())
	}
	if got := indexedRepos(t, indexDir); !slices.Equal(got, []string{kept.Path}) {
		t.Errorf("state holds %v, want only %s", got, kept.Path)
	}
	if got := shardedRepos(t, indexDir); !slices.Equal(got, []string{kept.Name}) {
		t.Errorf("shards hold %v, want only %s (pruned %s)", got, kept.Name, gone.Name)
	}
}

func TestRun_OnlyRunDoesNotPrune(t *testing.T) {
	indexDir := isolateState(t)
	keptPath := setupSyncRepo(t, "kept", "main", false, withNoRemote)
	cfg := &config.Config{Dirs: []string{filepath.Dir(keptPath)}}
	kept := discoverOne(t, cfg)
	gone := vanishedRepo(t, indexDir, kept)

	rep, err := Run(context.Background(), cfg, Options{Only: kept.Path})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if rep.Pruned != 0 {
		t.Errorf("Report.Pruned = %d, want 0", rep.Pruned)
	}
	want := []string{gone.Path, kept.Path}
	slices.Sort(want)
	if got := indexedRepos(t, indexDir); !slices.Equal(got, want) {
		t.Errorf("state holds %v, want %v", got, want)
	}
	if got := shardedRepos(t, indexDir); !slices.Contains(got, gone.Name) {
		t.Errorf("shards hold %v, want %s kept", got, gone.Name)
	}
}

func TestRun_KeepsExcludedRepo(t *testing.T) {
	indexDir := isolateState(t)
	keptPath := setupSyncRepo(t, "kept", "main", false, withNoRemote)
	exclPath := setupSyncRepo(t, "excl", "main", false, withNoRemote)
	cfg := &config.Config{Dirs: []string{filepath.Dir(exclPath)}}
	excluded := discoverOne(t, cfg)
	cfg.Dirs = append(cfg.Dirs, filepath.Dir(keptPath))
	cfg.Hooks.PostMerge.Exclude = []string{excluded.Path}
	kept := discoverOne(t, cfg)
	seedIndex(t, indexDir, kept, excluded)

	rep, err := Run(context.Background(), cfg, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if rep.Pruned != 0 {
		t.Errorf("Report.Pruned = %d, want 0", rep.Pruned)
	}
	want := []string{excluded.Path, kept.Path}
	slices.Sort(want)
	if got := indexedRepos(t, indexDir); !slices.Equal(got, want) {
		t.Errorf("state holds %v, want %v", got, want)
	}
}

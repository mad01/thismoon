package syncer

import (
	"context"
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// withLocalRemote points origin at a bare clone of the repo and tracks its
// main branch, so pullRepo pulls offline and reports ok. The repo must be on
// main.
func withLocalRemote(t *testing.T, repoDir string, run func(args ...string)) {
	t.Helper()
	bare := filepath.Join(t.TempDir(), "remote.git")
	run("clone", "--bare", repoDir, bare)
	run("remote", "set-url", "origin", bare)
	run("fetch", "origin")
	run("branch", "--set-upstream-to=origin/main")
}

// pullFor runs the real pull phase on one repo and fails unless it reports
// wantStatus.
func pullFor(t *testing.T, name, dir, wantStatus string) PullResult {
	t.Helper()
	repo := finder.Repo{Name: "org/" + name, Path: dir, Remote: "origin"}
	res := pullRepo(context.Background(), repo, false)
	if res.Status != wantStatus {
		t.Fatalf("pull %s: status = %q (%s), want %q", name, res.Status, res.Message, wantStatus)
	}
	return res
}

// seedState writes a state.json holding entries into indexDir.
func seedState(t *testing.T, indexDir string, entries map[string]search.RepoState) {
	t.Helper()
	state := search.EmptyState()
	for path, rs := range entries {
		state.SetRepo(path, rs)
	}
	if err := state.Save(indexDir); err != nil {
		t.Fatalf("seed state: %v", err)
	}
}

// storedRepo reads repoPath's entry back from indexDir's state.json.
func storedRepo(t *testing.T, indexDir, repoPath string) search.RepoState {
	t.Helper()
	state, err := search.LoadState(indexDir)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	rs, ok := state.GetRepo(repoPath)
	if !ok {
		t.Fatalf("state.json has no entry for %s", repoPath)
	}
	return rs
}

// currentFingerprint probes git for dir's fingerprint as search computes it.
func currentFingerprint(t *testing.T, dir string) search.RepoState {
	t.Helper()
	fp, err := search.Fingerprint(dir)
	if err != nil {
		t.Fatalf("fingerprint %s: %v", dir, err)
	}
	return fp
}

func runIndexChanged(
	t *testing.T,
	indexDir string,
	results []PullResult,
	queued []finder.Repo,
) []finder.Repo {
	t.Helper()
	indexed, err := indexChanged(indexInput{
		indexDir:    indexDir,
		results:     results,
		queued:      queued,
		concurrency: 2,
		errw:        io.Discard,
	})
	if err != nil {
		t.Fatalf("indexChanged: %v", err)
	}
	return indexed
}

func indexedPaths(repos []finder.Repo) []string {
	paths := make([]string, len(repos))
	for i, r := range repos {
		paths[i] = r.Path
	}
	return paths
}

func TestIndexChanged_OkRepoWithStaleFingerprintIsIndexed(t *testing.T) {
	dir := setupSyncRepo(t, "okstale", "main", false, withLocalRemote)
	res := pullFor(t, "okstale", dir, "ok")
	indexDir := t.TempDir()
	// A fingerprint from an older index format: same tree, different hash.
	seedState(t, indexDir, map[string]search.RepoState{
		dir: {Fingerprint: "written-by-an-older-index-format"},
	})

	got := runIndexChanged(t, indexDir, []PullResult{res}, nil)

	if paths := indexedPaths(got); len(paths) != 1 || paths[0] != dir {
		t.Fatalf("indexed = %v, want only %s", paths, dir)
	}
	stored := storedRepo(t, indexDir, dir)
	if want := currentFingerprint(t, dir); stored.Fingerprint != want.Fingerprint {
		t.Errorf("stored fingerprint = %q, want current %q", stored.Fingerprint, want.Fingerprint)
	}
	if stored.IndexedAt.IsZero() {
		t.Error("stored IndexedAt is zero, want the index time")
	}
}

func TestIndexChanged_StaleRepoOnNonDefaultBranchIsIndexed(t *testing.T) {
	dir := setupSyncRepo(t, "feature", "feature-x", false)
	res := pullFor(t, "feature", dir, "skip")
	indexDir := t.TempDir()
	seedState(t, indexDir, map[string]search.RepoState{
		dir: {Fingerprint: "written-by-an-older-index-format", Branch: "feature-x"},
	})

	got := runIndexChanged(t, indexDir, []PullResult{res}, nil)

	if paths := indexedPaths(got); len(paths) != 1 || paths[0] != dir {
		t.Fatalf("indexed = %v, want only %s", paths, dir)
	}
	if stored, want := storedRepo(t, indexDir, dir), currentFingerprint(t, dir); stored.Fingerprint != want.Fingerprint {
		t.Errorf("stored fingerprint = %q, want current %q", stored.Fingerprint, want.Fingerprint)
	}
}

func TestIndexChanged_FreshRepoIsNotReindexed(t *testing.T) {
	dir := setupSyncRepo(t, "fresh", "main", false, withLocalRemote)
	res := pullFor(t, "fresh", dir, "ok")
	indexDir := t.TempDir()
	fresh := currentFingerprint(t, dir)
	fresh.IndexedAt = time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	seedState(t, indexDir, map[string]search.RepoState{dir: fresh})

	got := runIndexChanged(t, indexDir, []PullResult{res}, nil)

	if len(got) != 0 {
		t.Fatalf("indexed = %v, want nothing for a fresh repo", indexedPaths(got))
	}
	if stored := storedRepo(t, indexDir, dir); !stored.IndexedAt.Equal(fresh.IndexedAt) {
		t.Errorf("IndexedAt = %v, want untouched %v", stored.IndexedAt, fresh.IndexedAt)
	}
}

func TestIndexChanged_StaleQueuedRepoIndexedOnce(t *testing.T) {
	dir := setupSyncRepo(t, "queued", "feature-x", false)
	res := pullFor(t, "queued", dir, "skip")
	indexDir := t.TempDir()
	seedState(t, indexDir, map[string]search.RepoState{
		dir: {Fingerprint: "written-by-an-older-index-format"},
	})

	got := runIndexChanged(t, indexDir, []PullResult{res}, []finder.Repo{res.Repo})

	if paths := indexedPaths(got); len(paths) != 1 || paths[0] != dir {
		t.Fatalf("indexed = %v, want %s exactly once", paths, dir)
	}
}

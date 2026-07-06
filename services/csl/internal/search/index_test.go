package search

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// gitAvailable reports whether git is on PATH.
func gitAvailable(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
}

// initGitRepo creates a minimal git repo with one committed file inside dir.
// Returns the repo path (same as dir).
func initGitRepo(t *testing.T, dir string) string {
	t.Helper()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	run("init")
	run("checkout", "-b", "main")

	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "hello.txt")
	run("commit", "-m", "initial")
	return dir
}

// ---------- LoadState / Save round-trip ----------

func TestLoadState_NonExistentDir(t *testing.T) {
	dir := t.TempDir()
	nonExistent := filepath.Join(dir, "does-not-exist")

	state, err := LoadState(nonExistent)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if state == nil {
		t.Fatal("expected non-nil state")
	}
	if len(state.Repos) != 0 {
		t.Fatalf("expected empty repos map, got %d entries", len(state.Repos))
	}
}

func TestStateRoundTrip(t *testing.T) {
	dir := t.TempDir()

	original := &IndexState{Repos: make(map[string]RepoState)}
	original.SetRepo("/tmp/repo-a", RepoState{
		Fingerprint: "abc123",
		HEAD:        "deadbeef",
		Branch:      "main",
		Dirty:       false,
		IndexedAt:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	original.SetRepo("/tmp/repo-b", RepoState{
		Fingerprint: "def456",
		HEAD:        "cafebabe",
		Branch:      "feature",
		Dirty:       true,
		IndexedAt:   time.Date(2025, 6, 15, 12, 0, 0, 0, time.UTC),
	})

	if err := original.Save(dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}

	if len(loaded.Repos) != 2 {
		t.Fatalf("expected 2 repos, got %d", len(loaded.Repos))
	}

	tests := []struct {
		path string
		want RepoState
	}{
		{"/tmp/repo-a", original.Repos["/tmp/repo-a"]},
		{"/tmp/repo-b", original.Repos["/tmp/repo-b"]},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			got, ok := loaded.GetRepo(tc.path)
			if !ok {
				t.Fatalf("repo %s not found", tc.path)
			}
			if got.Fingerprint != tc.want.Fingerprint {
				t.Errorf("fingerprint: got %q, want %q", got.Fingerprint, tc.want.Fingerprint)
			}
			if got.HEAD != tc.want.HEAD {
				t.Errorf("HEAD: got %q, want %q", got.HEAD, tc.want.HEAD)
			}
			if got.Branch != tc.want.Branch {
				t.Errorf("Branch: got %q, want %q", got.Branch, tc.want.Branch)
			}
			if got.Dirty != tc.want.Dirty {
				t.Errorf("Dirty: got %v, want %v", got.Dirty, tc.want.Dirty)
			}
			if !got.IndexedAt.Equal(tc.want.IndexedAt) {
				t.Errorf("IndexedAt: got %v, want %v", got.IndexedAt, tc.want.IndexedAt)
			}
		})
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "path")

	state := &IndexState{Repos: make(map[string]RepoState)}
	state.SetRepo("/tmp/r", RepoState{Fingerprint: "x"})

	if err := state.Save(dir); err != nil {
		t.Fatalf("Save to nested dir: %v", err)
	}

	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if _, ok := loaded.GetRepo("/tmp/r"); !ok {
		t.Fatal("expected repo /tmp/r after save to nested dir")
	}
}

func TestLoadState_CorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	if err := os.WriteFile(path, []byte(`{"repos":{}}`+"\n}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState(dir)
	if err == nil {
		t.Fatal("expected error for corrupt JSON")
	}

	if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
		t.Error("expected original state.json to be removed after corruption")
	}
	backup := path + ".corrupt"
	if _, statErr := os.Stat(backup); statErr != nil {
		t.Fatalf("expected backup at %s, got: %v", backup, statErr)
	}

	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"repos":{}}`+"\n}" {
		t.Errorf("backup content mismatch: %q", data)
	}
}

func TestLoadState_TrailingGarbage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	// Valid JSON followed by an extra closing brace — the exact corruption we hit.
	if err := os.WriteFile(path, []byte("{\n  \"repos\": {}\n}\n}"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := LoadState(dir)
	if err == nil {
		t.Fatal("expected error for trailing garbage")
	}

	backup := path + ".corrupt"
	if _, statErr := os.Stat(backup); statErr != nil {
		t.Fatalf("expected corrupt backup at %s: %v", backup, statErr)
	}
}

func TestSave_ConcurrentWriters(t *testing.T) {
	dir := t.TempDir()

	const writers = 20
	const iterations = 50

	var wg sync.WaitGroup
	wg.Add(writers)

	errs := make(chan error, writers*iterations)

	for w := range writers {
		go func(id int) {
			defer wg.Done()
			for i := range iterations {
				state := &IndexState{Repos: make(map[string]RepoState)}
				state.SetRepo("/tmp/repo", RepoState{
					Fingerprint: fmt.Sprintf("writer-%d-iter-%d", id, i),
					HEAD:        "deadbeef",
					Branch:      "main",
				})
				if err := state.Save(dir); err != nil {
					errs <- err
				}
			}
		}(w)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Save error: %v", err)
	}

	// The final state must be valid JSON regardless of write order.
	loaded, err := LoadState(dir)
	if err != nil {
		t.Fatalf("LoadState after concurrent writes: %v", err)
	}
	if _, ok := loaded.GetRepo("/tmp/repo"); !ok {
		t.Fatal("expected /tmp/repo in final state")
	}
}

func TestSave_NoLeftoverTempFiles(t *testing.T) {
	dir := t.TempDir()

	state := &IndexState{Repos: make(map[string]RepoState)}
	state.SetRepo("/tmp/r", RepoState{Fingerprint: "x"})

	for range 10 {
		if err := state.Save(dir); err != nil {
			t.Fatal(err)
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "state.json" {
			t.Errorf("leftover file after Save: %s", e.Name())
		}
	}
}

// ---------- ReloadBeforeSave pattern (fix 4: prevents stale overwrites) ----------

func TestReloadBeforeSave_PreservesOtherWriters(t *testing.T) {
	dir := t.TempDir()

	// Writer 1 saves state for repo-a.
	state1 := &IndexState{Repos: make(map[string]RepoState)}
	state1.SetRepo("/tmp/repo-a", RepoState{Fingerprint: "a1", HEAD: "aaa", Branch: "main"})
	if err := state1.Save(dir); err != nil {
		t.Fatal(err)
	}

	// Writer 2 loads fresh state, adds repo-b, and saves.
	// This simulates the fix: reload from disk before saving.
	state2, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	state2.SetRepo("/tmp/repo-b", RepoState{Fingerprint: "b1", HEAD: "bbb", Branch: "main"})
	if err := state2.Save(dir); err != nil {
		t.Fatal(err)
	}

	// Verify both repos are preserved.
	final, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := final.GetRepo("/tmp/repo-a"); !ok {
		t.Error("repo-a should be preserved after writer 2 saves")
	}
	if _, ok := final.GetRepo("/tmp/repo-b"); !ok {
		t.Error("repo-b should exist after writer 2 saves")
	}
}

func TestStaleWriteOverwritesOtserviceata(t *testing.T) {
	dir := t.TempDir()

	// This test demonstrates the BUG that fix 4 prevents:
	// if a writer uses a stale in-memory state (loaded before another writer
	// saves), it will clobber the other writer's data.
	state1 := &IndexState{Repos: make(map[string]RepoState)}
	state1.SetRepo("/tmp/repo-a", RepoState{Fingerprint: "a1", HEAD: "aaa", Branch: "main"})
	if err := state1.Save(dir); err != nil {
		t.Fatal(err)
	}

	// staleState represents the BUG: loaded BEFORE state1 wrote.
	staleState := &IndexState{Repos: make(map[string]RepoState)}
	staleState.SetRepo("/tmp/repo-b", RepoState{Fingerprint: "b1", HEAD: "bbb", Branch: "main"})
	if err := staleState.Save(dir); err != nil {
		t.Fatal(err)
	}

	// repo-a is lost because staleState didn't know about it.
	final, err := LoadState(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := final.GetRepo("/tmp/repo-a"); ok {
		t.Error("repo-a should be lost (demonstrates the stale-write bug)")
	}
	if _, ok := final.GetRepo("/tmp/repo-b"); !ok {
		t.Error("repo-b should exist")
	}
}

// ---------- SetRepo / GetRepo thread safety ----------

func TestSetGetRepo_Concurrent(t *testing.T) {
	state := &IndexState{Repos: make(map[string]RepoState)}

	const goroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // writers + readers

	// Writers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				path := "/tmp/repo"
				state.SetRepo(path, RepoState{
					Fingerprint: "fp",
					HEAD:        "head",
					Branch:      "main",
				})
			}
		}(i)
	}

	// Readers
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				state.GetRepo("/tmp/repo")
			}
		}(i)
	}

	wg.Wait()

	// Verify final state is consistent.
	if _, ok := state.GetRepo("/tmp/repo"); !ok {
		t.Fatal("expected repo to exist after concurrent writes")
	}
}

// ---------- Fingerprint ----------

func TestFingerprint(t *testing.T) {
	gitAvailable(t)

	t.Run("same state same fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		fp1, err := Fingerprint(dir)
		if err != nil {
			t.Fatalf("Fingerprint: %v", err)
		}
		fp2, err := Fingerprint(dir)
		if err != nil {
			t.Fatalf("Fingerprint: %v", err)
		}

		if fp1.Fingerprint != fp2.Fingerprint {
			t.Errorf(
				"same repo state produced different fingerprints: %q vs %q",
				fp1.Fingerprint,
				fp2.Fingerprint,
			)
		}
		if fp1.Dirty {
			t.Error("expected clean repo, got dirty")
		}
		if fp1.Branch != "main" {
			t.Errorf("expected branch 'main', got %q", fp1.Branch)
		}
	})

	t.Run("modify file changes fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		fp1, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		fp2, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		if fp1.Fingerprint == fp2.Fingerprint {
			t.Error("modifying a file should change the fingerprint")
		}
		if !fp2.Dirty {
			t.Error("expected dirty=true after modifying file")
		}
	})

	t.Run("new commit changes fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		fp1, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		if err := os.WriteFile(filepath.Join(dir, "second.txt"), []byte("second\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", "second.txt")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add: %v\n%s", err, out)
		}
		cmd = exec.Command("git", "commit", "-m", "second")
		cmd.Dir = dir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}

		fp2, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		if fp1.Fingerprint == fp2.Fingerprint {
			t.Error("new commit should change the fingerprint")
		}
		if fp2.Dirty {
			t.Error("expected clean after commit")
		}
	})

	t.Run("branch name included in fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		fp1, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		cmd := exec.Command("git", "checkout", "-b", "feature")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git checkout: %v\n%s", err, out)
		}

		fp2, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		if fp1.Fingerprint == fp2.Fingerprint {
			t.Error("different branch should change the fingerprint")
		}
		if fp2.Branch != "feature" {
			t.Errorf("expected branch 'feature', got %q", fp2.Branch)
		}
	})

	t.Run("dirty state included in fingerprint", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		fpClean, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		// Add an untracked file.
		if err := os.WriteFile(filepath.Join(dir, "untracked.txt"), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		fpDirty, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		if fpClean.Fingerprint == fpDirty.Fingerprint {
			t.Error("untracked file should change the fingerprint")
		}
		if fpClean.Dirty {
			t.Error("expected clean before untracked file")
		}
		if !fpDirty.Dirty {
			t.Error("expected dirty after untracked file")
		}
	})

	t.Run("not a git repo returns error", func(t *testing.T) {
		dir := t.TempDir()
		_, err := Fingerprint(dir)
		if err == nil {
			t.Error("expected error for non-git directory")
		}
	})
}

// ---------- CheckStaleness ----------

func TestCheckStaleness(t *testing.T) {
	gitAvailable(t)

	t.Run("fresh repo", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		fp, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}
		fp.IndexedAt = time.Now()

		state := &IndexState{Repos: make(map[string]RepoState)}
		state.SetRepo(dir, fp)

		repos := []finder.Repo{{Name: "test/repo", Path: dir}}
		result, err := CheckStaleness(repos, state)
		if err != nil {
			t.Fatal(err)
		}

		if len(result.Fresh) != 1 {
			t.Errorf("expected 1 fresh, got %d", len(result.Fresh))
		}
		if len(result.Stale) != 0 {
			t.Errorf("expected 0 stale, got %d", len(result.Stale))
		}
	})

	t.Run("stale after modification", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		fp, err := Fingerprint(dir)
		if err != nil {
			t.Fatal(err)
		}

		state := &IndexState{Repos: make(map[string]RepoState)}
		state.SetRepo(dir, fp)

		// Modify the repo.
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		repos := []finder.Repo{{Name: "test/repo", Path: dir}}
		result, err := CheckStaleness(repos, state)
		if err != nil {
			t.Fatal(err)
		}

		if len(result.Fresh) != 0 {
			t.Errorf("expected 0 fresh, got %d", len(result.Fresh))
		}
		if len(result.Stale) != 1 {
			t.Errorf("expected 1 stale, got %d", len(result.Stale))
		}
	})

	t.Run("unknown repo is stale", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		state := &IndexState{Repos: make(map[string]RepoState)}

		repos := []finder.Repo{{Name: "test/repo", Path: dir}}
		result, err := CheckStaleness(repos, state)
		if err != nil {
			t.Fatal(err)
		}

		if len(result.Stale) != 1 {
			t.Errorf("expected 1 stale, got %d", len(result.Stale))
		}
		if len(result.Fresh) != 0 {
			t.Errorf("expected 0 fresh, got %d", len(result.Fresh))
		}
	})

	t.Run("non-git repo is stale", func(t *testing.T) {
		dir := t.TempDir() // no git init

		state := &IndexState{Repos: make(map[string]RepoState)}

		repos := []finder.Repo{{Name: "test/notgit", Path: dir}}
		result, err := CheckStaleness(repos, state)
		if err != nil {
			t.Fatal(err)
		}

		if len(result.Stale) != 1 {
			t.Errorf("expected 1 stale for non-git repo, got %d", len(result.Stale))
		}
	})

	t.Run("mixed fresh and stale", func(t *testing.T) {
		dirFresh := t.TempDir()
		initGitRepo(t, dirFresh)
		dirStale := t.TempDir()
		initGitRepo(t, dirStale)

		fpFresh, err := Fingerprint(dirFresh)
		if err != nil {
			t.Fatal(err)
		}

		state := &IndexState{Repos: make(map[string]RepoState)}
		state.SetRepo(dirFresh, fpFresh)
		// dirStale has no stored state -> stale

		repos := []finder.Repo{
			{Name: "org/fresh", Path: dirFresh},
			{Name: "org/stale", Path: dirStale},
		}
		result, err := CheckStaleness(repos, state)
		if err != nil {
			t.Fatal(err)
		}

		if len(result.Fresh) != 1 {
			t.Errorf("expected 1 fresh, got %d", len(result.Fresh))
		}
		if len(result.Stale) != 1 {
			t.Errorf("expected 1 stale, got %d", len(result.Stale))
		}
		if result.Fresh[0].Name != "org/fresh" {
			t.Errorf("expected fresh repo 'org/fresh', got %q", result.Fresh[0].Name)
		}
		if result.Stale[0].Name != "org/stale" {
			t.Errorf("expected stale repo 'org/stale', got %q", result.Stale[0].Name)
		}
	})
}

// ---------- fingerprint cache ----------

func TestCheckStaleness_FingerprintCache(t *testing.T) {
	gitAvailable(t)

	dir := t.TempDir()
	initGitRepo(t, dir)

	fp, err := Fingerprint(dir)
	if err != nil {
		t.Fatal(err)
	}

	state := &IndexState{Repos: make(map[string]RepoState)}
	state.SetRepo(dir, fp)

	repos := []finder.Repo{{Name: "test/repo", Path: dir}}

	// First check populates the cache.
	result, err := CheckStaleness(repos, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fresh) != 1 {
		t.Fatalf("expected 1 fresh, got %d", len(result.Fresh))
	}

	// Modify the repo. Within the TTL the cached fingerprint still wins, so
	// the repo is reported fresh — the documented tradeoff of the cache.
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err = CheckStaleness(repos, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fresh) != 1 {
		t.Fatalf("expected cached fingerprint to report fresh, got %d fresh", len(result.Fresh))
	}

	// Invalidation forces a re-read, surfacing the modification.
	InvalidateFingerprint(dir)
	result, err = CheckStaleness(repos, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Stale) != 1 {
		t.Fatalf("expected 1 stale after invalidation, got %d", len(result.Stale))
	}
}

// ---------- DirtyInfo ----------

func TestDirtyInfo(t *testing.T) {
	gitAvailable(t)

	t.Run("clean repo", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		mod, untracked, err := DirtyInfo(dir)
		if err != nil {
			t.Fatal(err)
		}
		if mod != 0 || untracked != 0 {
			t.Errorf("expected 0 modified, 0 untracked; got %d, %d", mod, untracked)
		}
	})

	t.Run("modified files", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("modified\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		mod, untracked, err := DirtyInfo(dir)
		if err != nil {
			t.Fatal(err)
		}
		if mod != 1 {
			t.Errorf("expected 1 modified, got %d", mod)
		}
		if untracked != 0 {
			t.Errorf("expected 0 untracked, got %d", untracked)
		}
	})

	t.Run("untracked files", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		if err := os.WriteFile(filepath.Join(dir, "new1.txt"), []byte("new\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "new2.txt"), []byte("new\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		mod, untracked, err := DirtyInfo(dir)
		if err != nil {
			t.Fatal(err)
		}
		if mod != 0 {
			t.Errorf("expected 0 modified, got %d", mod)
		}
		if untracked != 2 {
			t.Errorf("expected 2 untracked, got %d", untracked)
		}
	})

	t.Run("mixed modified and untracked", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		// Modify tracked file.
		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		// Add untracked files.
		for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		mod, untracked, err := DirtyInfo(dir)
		if err != nil {
			t.Fatal(err)
		}
		if mod != 1 {
			t.Errorf("expected 1 modified, got %d", mod)
		}
		if untracked != 3 {
			t.Errorf("expected 3 untracked, got %d", untracked)
		}
	})

	t.Run("staged file counts as modified", func(t *testing.T) {
		dir := t.TempDir()
		initGitRepo(t, dir)

		if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("staged\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("git", "add", "hello.txt")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add: %v\n%s", err, out)
		}

		mod, untracked, err := DirtyInfo(dir)
		if err != nil {
			t.Fatal(err)
		}
		if mod != 1 {
			t.Errorf("expected 1 modified (staged), got %d", mod)
		}
		if untracked != 0 {
			t.Errorf("expected 0 untracked, got %d", untracked)
		}
	})
}

// ---------- DefaultIndexDir ----------

func TestDefaultIndexDir(t *testing.T) {
	dir, err := DefaultIndexDir()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	if filepath.Base(dir) != "search-index" {
		t.Errorf("expected dir to end with 'search-index', got %q", filepath.Base(dir))
	}
}

// ---------- IndexDirSize ----------

func TestIndexDirSize(t *testing.T) {
	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		size, err := IndexDirSize(dir)
		if err != nil {
			t.Fatal(err)
		}
		if size != 0 {
			t.Errorf("expected 0 bytes, got %d", size)
		}
	})

	t.Run("directory with files", func(t *testing.T) {
		dir := t.TempDir()
		data := []byte("hello world") // 11 bytes
		if err := os.WriteFile(filepath.Join(dir, "a.txt"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.txt"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		size, err := IndexDirSize(dir)
		if err != nil {
			t.Fatal(err)
		}
		if size < 22 {
			t.Errorf("expected at least 22 bytes, got %d", size)
		}
	})

	t.Run("nested files counted", func(t *testing.T) {
		dir := t.TempDir()
		sub := filepath.Join(dir, "sub")
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		data := []byte("data")
		if err := os.WriteFile(filepath.Join(sub, "f.txt"), data, 0o644); err != nil {
			t.Fatal(err)
		}

		size, err := IndexDirSize(dir)
		if err != nil {
			t.Fatal(err)
		}
		if size < int64(len(data)) {
			t.Errorf("expected at least %d bytes, got %d", len(data), size)
		}
	})

	t.Run("non-existent directory returns zero", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "nope")
		size, err := IndexDirSize(dir)
		// filepath.Walk may or may not error for non-existent root;
		// the function is best-effort, so just verify it doesn't panic
		// and returns zero size.
		_ = err
		if size != 0 {
			t.Errorf("expected 0 bytes for non-existent dir, got %d", size)
		}
	})
}

// ---------- Benchmarks ----------

func BenchmarkFingerprint(b *testing.B) {
	if _, err := exec.LookPath("git"); err != nil {
		b.Skip("git not available")
	}

	dir := b.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git init: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "checkout", "-b", "main")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git checkout: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("bench\n"), 0o644); err != nil {
		b.Fatal(err)
	}
	cmd = exec.Command("git", "add", "file.txt")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git add: %v\n%s", err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "bench")
	cmd.Dir = dir
	cmd.Env = append(
		os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		b.Fatalf("git commit: %v\n%s", err, out)
	}

	b.ResetTimer()
	for b.Loop() {
		if _, err := Fingerprint(dir); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkStateRoundTrip(b *testing.B) {
	dir := b.TempDir()

	state := &IndexState{Repos: make(map[string]RepoState)}
	for i := 0; i < 100; i++ {
		state.SetRepo(
			filepath.Join("/tmp/repo", string(rune('a'+i%26)), string(rune('0'+i/26))),
			RepoState{
				Fingerprint: "abcdef1234567890",
				HEAD:        "deadbeef",
				Branch:      "main",
				IndexedAt:   time.Now(),
			},
		)
	}

	b.ResetTimer()
	for b.Loop() {
		if err := state.Save(dir); err != nil {
			b.Fatal(err)
		}
		if _, err := LoadState(dir); err != nil {
			b.Fatal(err)
		}
	}
}

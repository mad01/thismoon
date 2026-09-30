package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/queue"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

// indexTestCmd is a bare command whose stdout and stderr land in buffers.
func indexTestCmd(out, errOut *bytes.Buffer) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return cmd
}

// countShards counts the .zoekt shard files in indexDir; a missing directory
// counts as zero.
func countShards(t *testing.T, indexDir string) int {
	t.Helper()
	entries, err := os.ReadDir(indexDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read index dir: %v", err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".zoekt") {
			n++
		}
	}
	return n
}

// holdLock takes the sync lock on indexDir for the test's duration, standing
// in for the csl sync or web refresher that owns the index right now.
func holdLock(t *testing.T, indexDir string) {
	t.Helper()
	unlock, err := syncer.Lock(indexDir)
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	t.Cleanup(unlock)
}

// wantLockedError checks that err is the fail-fast error the index commands
// share with csl sync: it matches ErrLocked, names the holder's PID, and says
// what to do next.
func wantLockedError(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, syncer.ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	wants := []string{"PID " + strconv.Itoa(os.Getpid()), "wait for it to finish", "csl doctor"}
	for _, want := range wants {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("err = %q, want it to mention %q", err, want)
		}
	}
}

// wantNothingWritten checks that a locked run left no shard and no
// state.json behind.
func wantNothingWritten(t *testing.T, indexDir string) {
	t.Helper()
	if got := countShards(t, indexDir); got != 0 {
		t.Errorf("shard count = %d, want 0 while locked", got)
	}
	if _, err := os.Stat(filepath.Join(indexDir, "state.json")); !os.IsNotExist(err) {
		t.Errorf("state.json stat err = %v, want not exist while locked", err)
	}
}

// wantIndexed checks the outcome of a successful index write: at least one
// shard, a state.json entry for repoPath, and a released lock.
func wantIndexed(t *testing.T, indexDir, repoPath string) {
	t.Helper()
	if countShards(t, indexDir) == 0 {
		t.Error("shard count = 0, want at least one shard after the build")
	}
	state, err := search.LoadState(indexDir)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if _, ok := state.GetRepo(repoPath); !ok {
		t.Errorf("state.json has no entry for %s", repoPath)
	}
	if syncer.Locked(indexDir) {
		t.Error("lock still held after the build, want it released")
	}
}

func TestIndexStaleHoldsLock(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	repos := []finder.Repo{{Name: "test/repo", Path: repoDir}}
	current := map[string]search.RepoState{repoDir: {Fingerprint: "fp-1", Branch: "main"}}

	t.Run("fails fast without writing while another process holds the lock", func(t *testing.T) {
		indexDir := t.TempDir()
		holdLock(t, indexDir)

		var out, errOut bytes.Buffer
		err := indexStale(indexTestCmd(&out, &errOut), indexDir, repos, nil, current)
		wantLockedError(t, err)
		wantNothingWritten(t, indexDir)
	})

	t.Run("writes shards and state and releases the lock when free", func(t *testing.T) {
		indexDir := t.TempDir()

		var out, errOut bytes.Buffer
		err := indexStale(indexTestCmd(&out, &errOut), indexDir, repos, nil, current)
		if err != nil {
			t.Fatalf("indexStale: %v", err)
		}
		wantIndexed(t, indexDir, repoDir)
		state, _ := search.LoadState(indexDir)
		if rs, _ := state.GetRepo(repoDir); rs.Fingerprint != "fp-1" {
			t.Errorf("recorded fingerprint = %q, want the staleness check's fp-1", rs.Fingerprint)
		}
	})
}

func TestIndexSingleHoldsLock(t *testing.T) {
	repoDir := setupSyncE2ERepo(t, "single")
	setupTestConfig(t, "dirs:\n  - "+filepath.Dir(repoDir)+"\n")

	t.Run("fails fast without writing while another process holds the lock", func(t *testing.T) {
		indexDir := t.TempDir()
		holdLock(t, indexDir)

		var out, errOut bytes.Buffer
		err := runIndexSingle(indexTestCmd(&out, &errOut), indexDir, repoDir)
		wantLockedError(t, err)
		wantNothingWritten(t, indexDir)
		if out.Len() != 0 {
			t.Errorf("stdout = %q, want nothing while locked", out.String())
		}
	})

	t.Run("indexes and releases the lock when free", func(t *testing.T) {
		indexDir := t.TempDir()

		var out, errOut bytes.Buffer
		if err := runIndexSingle(indexTestCmd(&out, &errOut), indexDir, repoDir); err != nil {
			t.Fatalf("runIndexSingle: %v", err)
		}
		wantIndexed(t, indexDir, repoDir)
		if !strings.Contains(out.String(), "indexed org/single") {
			t.Errorf("stdout = %q, want the indexed line", out.String())
		}
	})
}

func TestIndexDrainHoldsLock(t *testing.T) {
	repoDir := setupSyncE2ERepo(t, "drain")
	setupTestConfig(t, "dirs:\n  - "+filepath.Dir(repoDir)+"\n")
	queuePath, err := queue.DefaultPath()
	if err != nil {
		t.Fatalf("queue path: %v", err)
	}

	t.Run(
		"fails fast and leaves the queue while another process holds the lock",
		func(t *testing.T) {
			indexDir := t.TempDir()
			holdLock(t, indexDir)
			if err := queue.Enqueue(queuePath, repoDir); err != nil {
				t.Fatalf("enqueue: %v", err)
			}

			var out, errOut bytes.Buffer
			err := runIndexDrain(indexTestCmd(&out, &errOut), indexDir)
			wantLockedError(t, err)
			wantNothingWritten(t, indexDir)
			if _, err := os.Stat(queuePath); err != nil {
				t.Errorf("queue stat err = %v, want the queue left unclaimed while locked", err)
			}
		},
	)

	t.Run("drains the queue and releases the lock when free", func(t *testing.T) {
		indexDir := t.TempDir()
		if err := queue.Enqueue(queuePath, repoDir); err != nil {
			t.Fatalf("enqueue: %v", err)
		}

		var out, errOut bytes.Buffer
		if err := runIndexDrain(indexTestCmd(&out, &errOut), indexDir); err != nil {
			t.Fatalf("runIndexDrain: %v", err)
		}
		wantIndexed(t, indexDir, repoDir)
		if _, err := os.Stat(queuePath); !os.IsNotExist(err) {
			t.Errorf("queue stat err = %v, want the queue consumed", err)
		}
		if !strings.Contains(out.String(), "indexed 1 repo(s)") {
			t.Errorf("stdout = %q, want the indexed line", out.String())
		}
	})
}

// TestIndexCommandFailsFastWhenLocked drives the real `csl index` command so
// the lock is exercised through cobra, the flags, and the resolved index dir.
func TestIndexCommandFailsFastWhenLocked(t *testing.T) {
	repoDir := setupSyncE2ERepo(t, "e2e")
	setupTestConfig(t, "dirs:\n  - "+filepath.Dir(repoDir)+"\n")
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		t.Fatalf("index dir: %v", err)
	}
	holdLock(t, indexDir)

	_, err = runCLI(t, "index")
	wantLockedError(t, err)
	wantNothingWritten(t, indexDir)
}

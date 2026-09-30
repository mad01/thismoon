package mcpserver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

// zoektShardCount counts the .zoekt shard files in indexDir; a missing
// directory counts as zero.
func zoektShardCount(t *testing.T, indexDir string) int {
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

func TestHandleRepoReindexHoldsLock(t *testing.T) {
	reposRoot := setupRepoEnv(t, []struct{ Org, Name string }{{"mad01", "octo"}})
	src := filepath.Join(reposRoot, "mad01", "octo", "main.go")
	if err := os.WriteFile(src, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		t.Fatalf("index dir: %v", err)
	}
	in := repoReindexInput{Name: "octo"}

	t.Run("returns a retry hint without writing while the lock is held", func(t *testing.T) {
		unlock, err := syncer.Lock(indexDir)
		if err != nil {
			t.Fatalf("acquire lock: %v", err)
		}
		defer unlock()

		_, _, err = handleRepoReindex(context.Background(), nil, in)
		if !errors.Is(err, syncer.ErrLocked) {
			t.Fatalf("err = %v, want ErrLocked", err)
		}
		wants := []string{"PID " + strconv.Itoa(os.Getpid()), "csl_repo_reindex", "csl_doctor"}
		for _, want := range wants {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("err = %q, want it to mention %q", err, want)
			}
		}
		if got := zoektShardCount(t, indexDir); got != 0 {
			t.Errorf("shard count = %d, want 0 while locked", got)
		}
	})

	t.Run("reindexes and releases the lock when free", func(t *testing.T) {
		_, out, err := handleRepoReindex(context.Background(), nil, in)
		if err != nil {
			t.Fatalf("handleRepoReindex: %v", err)
		}
		if !out.Reindexed || out.Name != "mad01/octo" {
			t.Errorf("out = %+v, want reindexed mad01/octo", out)
		}
		if got := zoektShardCount(t, indexDir); got == 0 {
			t.Error("shard count = 0, want at least one shard after the reindex")
		}
		if syncer.Locked(indexDir) {
			t.Error("lock still held after the reindex, want it released")
		}
	})
}

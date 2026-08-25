package web

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

// shardCount counts the .zoekt shard files in indexDir.
func shardCount(t *testing.T, indexDir string) int {
	t.Helper()
	entries, err := os.ReadDir(indexDir)
	if err != nil {
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

func TestBuildInitialIndex(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	repos := []finder.Repo{{Name: "test/repo", Path: repoDir}}

	t.Run("skips while another process holds the lock", func(t *testing.T) {
		indexDir := t.TempDir()
		unlock, err := syncer.Lock(indexDir)
		if err != nil {
			t.Fatalf("acquire lock: %v", err)
		}
		defer unlock()

		s := &Service{indexDir: indexDir}
		if err := s.buildInitialIndex(repos, &search.StalenessResult{}, search.EmptyState()); err != nil {
			t.Fatalf("buildInitialIndex: %v", err)
		}
		if got := shardCount(t, indexDir); got != 0 {
			t.Errorf("shard count = %d, want 0 (build must be skipped while locked)", got)
		}
	})

	t.Run("builds and releases the lock when free", func(t *testing.T) {
		indexDir := t.TempDir()
		s := &Service{indexDir: indexDir}
		if err := s.buildInitialIndex(repos, &search.StalenessResult{}, search.EmptyState()); err != nil {
			t.Fatalf("buildInitialIndex: %v", err)
		}
		if got := shardCount(t, indexDir); got == 0 {
			t.Error("shard count = 0, want at least one shard after the build")
		}
		if syncer.Locked(indexDir) {
			t.Error("lock still held after the build, want it released")
		}
	})
}

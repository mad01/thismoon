package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

func TestIndexForSearch(t *testing.T) {
	repoDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write repo file: %v", err)
	}
	repos := []finder.Repo{{Name: "test/repo", Path: repoDir}}

	newCmd := func(errOut *bytes.Buffer) *cobra.Command {
		cmd := &cobra.Command{}
		cmd.SetErr(errOut)
		return cmd
	}

	t.Run("skips with a note while another process holds the lock", func(t *testing.T) {
		indexDir := t.TempDir()
		unlock, err := syncer.Lock(indexDir)
		if err != nil {
			t.Fatalf("acquire lock: %v", err)
		}
		defer unlock()

		var errBuf bytes.Buffer
		err = indexForSearch(
			newCmd(&errBuf),
			indexDir,
			repos,
			&search.StalenessResult{},
			search.EmptyState(),
		)
		if err != nil {
			t.Fatalf("indexForSearch: %v", err)
		}
		if !strings.Contains(errBuf.String(), "another sync or background refresh") {
			t.Errorf("stderr = %q, want a skip note", errBuf.String())
		}
		entries, readErr := os.ReadDir(indexDir)
		if readErr != nil {
			t.Fatalf("read index dir: %v", readErr)
		}
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".zoekt") {
				t.Errorf("found shard %s, want none while locked", e.Name())
			}
		}
	})

	t.Run("builds and releases the lock when free", func(t *testing.T) {
		indexDir := t.TempDir()

		var errBuf bytes.Buffer
		err := indexForSearch(
			newCmd(&errBuf),
			indexDir,
			repos,
			&search.StalenessResult{},
			search.EmptyState(),
		)
		if err != nil {
			t.Fatalf("indexForSearch: %v", err)
		}
		if !strings.Contains(errBuf.String(), "Indexing 1 repo(s)") {
			t.Errorf("stderr = %q, want indexing progress", errBuf.String())
		}
		if syncer.Locked(indexDir) {
			t.Error("lock still held after the build, want it released")
		}
	})
}

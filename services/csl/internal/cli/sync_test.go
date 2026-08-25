package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func resetSyncFlags() {
	syncConcurrencyFlag = 0
	syncDryRunFlag = false
}

// setupSyncE2ERepo creates a committed repo on main with a remote, the minimal
// shape the sync dry-run needs to evaluate a repo as pullable. The pull state
// machine itself is covered in internal/syncer.
func setupSyncE2ERepo(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	repoDir := filepath.Join(dir, name)
	_ = os.MkdirAll(repoDir, 0o755)
	gitInit(t, repoDir)
	gitSetRemote(t, repoDir, "git@github.com:org/"+name+".git")

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=Test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=Test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("checkout", "-b", "main")
	_ = os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("content\n"), 0o644)
	run("add", "file.txt")
	run("commit", "-m", "init")

	return repoDir
}

func TestSyncDryRunE2E(t *testing.T) {
	dir := setupSyncE2ERepo(t, "e2e")
	parent := filepath.Dir(dir)

	cfg := "dirs:\n  - " + parent + "\nhooks:\n  post_merge:\n    enabled: true\n"
	_, cleanup := setupTestConfig(t, cfg)
	defer cleanup()

	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs([]string{"sync", "--dry-run"})
	resetSyncFlags()
	defer resetSyncFlags()

	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("execute: %v\noutput: %s", err, buf.String())
	}

	output := buf.String()
	if !strings.Contains(output, "dry-run") {
		t.Errorf("expected dry-run in output, got: %s", output)
	}

	rootCmd.SetArgs(nil)
	rootCmd.SetOut(nil)
	rootCmd.SetErr(nil)
}

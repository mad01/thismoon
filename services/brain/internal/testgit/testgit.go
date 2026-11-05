// Package testgit runs git in a test's own temp repos. Every call ignores
// the system config and carries its own identity, so a test never depends
// on, or writes to, a real checkout or the machine's git setup. Tests import
// it; the binary never does.
package testgit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Run runs git -C dir with args and returns its trimmed stdout, failing the
// test on an error.
func Run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	base := []string{
		"-C", dir,
		"-c", "user.name=brain test",
		"-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false",
		"-c", "init.defaultBranch=main",
	}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = string(ee.Stderr)
		}
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, stderr)
	}
	return strings.TrimSpace(string(out))
}

// Init makes dir a git repo.
func Init(t testing.TB, dir string) {
	t.Helper()
	Run(t, dir, "init", "-q")
}

// Commit writes files (repo-relative path to content) into dir, commits
// every change with message and returns the new commit's hash.
func Commit(t testing.TB, dir, message string, files map[string]string) string {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	Run(t, dir, "add", "-A")
	Run(t, dir, "commit", "-q", "-m", message)
	return Run(t, dir, "rev-parse", "HEAD")
}

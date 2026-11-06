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

// Identity is the author and committer of every test commit.
const (
	Name  = "brain test"
	Email = "test@example.com"
)

// Isolate points the git processes brain itself starts at no global or
// system config and gives them Name and Email as author and committer. It
// uses t.Setenv, so the caller cannot run in parallel.
func Isolate(t testing.TB) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_SYSTEM", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, who := range []string{"AUTHOR", "COMMITTER"} {
		t.Setenv("GIT_"+who+"_NAME", Name)
		t.Setenv("GIT_"+who+"_EMAIL", Email)
	}
}

// Run runs git -C dir with args and returns its trimmed stdout, failing the
// test on an error.
func Run(t testing.TB, dir string, args ...string) string {
	t.Helper()
	base := []string{
		"-C", dir,
		"-c", "user.name=" + Name,
		"-c", "user.email=" + Email,
		"-c", "commit.gpgsign=false",
		"-c", "init.defaultBranch=main",
	}
	cmd := exec.Command("git", append(base, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_SYSTEM="+os.DevNull, "GIT_CONFIG_GLOBAL="+os.DevNull)
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

// Bare makes a bare repo in a fresh temp dir, the stand-in for a remote,
// and returns its path.
func Bare(t testing.TB) string {
	t.Helper()
	dir := t.TempDir()
	Run(t, dir, "init", "-q", "--bare")
	return dir
}

// Clone clones the repo at remote into dir, which must not exist or be
// empty, so dir's main branch tracks origin/main.
func Clone(t testing.TB, remote, dir string) {
	t.Helper()
	Run(t, filepath.Dir(dir), "clone", "-q", remote, dir)
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

// Subjects returns the commit subjects of dir's HEAD, newest first.
func Subjects(t testing.TB, dir string) []string {
	t.Helper()
	out := Run(t, dir, "log", "--format=%s")
	if out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

// Status returns dir's short status, untracked files included.
func Status(t testing.TB, dir string) string {
	t.Helper()
	return Run(t, dir, "status", "--porcelain")
}

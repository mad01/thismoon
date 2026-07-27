package pin

import (
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"
	"time"
)

// newTestRepo creates a temp git repo containing file with content, committed,
// and returns the repo path. It uses local git config so the commit succeeds
// without touching the caller's global identity.
func newTestRepo(t *testing.T, file, content string) string {
	t.Helper()
	dir := t.TempDir()
	git := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-q")
	git("config", "user.email", "test@example.com")
	git("config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	git("add", file)
	git("commit", "-q", "-m", "init")
	return dir
}

// fixture is a five-line file with a trailing newline.
const fixture = "l1\nl2\nl3\nl4\nl5\n"

// sha of a byte string, matching how Resolve/Check hash line ranges.
func sha(s string) string {
	sum := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", sum)
}

func TestResolve(t *testing.T) {
	repo := newTestRepo(t, "code.txt", fixture)
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)

	t.Run("hashes the line range and stamps HEAD", func(t *testing.T) {
		p, err := Resolve(Ref{RepoPath: repo, File: "code.txt", StartLine: 2, EndLine: 4}, now)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if want := sha("l2\nl3\nl4\n"); p.ContentSHA256 != want {
			t.Errorf("ContentSHA256 = %q, want %q", p.ContentSHA256, want)
		}
		if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(p.HeadCommit) {
			t.Errorf("HeadCommit = %q, want 40 hex chars", p.HeadCommit)
		}
		if !p.ResolvedAt.Equal(now) {
			t.Errorf("ResolvedAt = %v, want %v", p.ResolvedAt, now)
		}
	})

	t.Run("last line without trailing newline is in bounds", func(t *testing.T) {
		r := newTestRepo(t, "code.txt", "a\nb\nc") // no final '\n'
		p, err := Resolve(Ref{RepoPath: r, File: "code.txt", StartLine: 3, EndLine: 3}, now)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if want := sha("c"); p.ContentSHA256 != want {
			t.Errorf("ContentSHA256 = %q, want %q", p.ContentSHA256, want)
		}
	})

	errCases := []struct {
		name string
		ref  Ref
	}{
		{"file missing", Ref{RepoPath: repo, File: "nope.txt", StartLine: 1, EndLine: 1}},
		{"start below one", Ref{RepoPath: repo, File: "code.txt", StartLine: 0, EndLine: 1}},
		{"end before start", Ref{RepoPath: repo, File: "code.txt", StartLine: 3, EndLine: 2}},
		{"end beyond EOF", Ref{RepoPath: repo, File: "code.txt", StartLine: 4, EndLine: 9}},
		{
			"not a repo dir",
			Ref{
				RepoPath:  filepath.Join(repo, "code.txt"),
				File:      "code.txt",
				StartLine: 1,
				EndLine:   1,
			},
		},
	}
	for _, tc := range errCases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Resolve(tc.ref, now); err == nil {
				t.Errorf("Resolve(%+v): want error, got nil", tc.ref)
			}
		})
	}

	t.Run("non-git repo fails rev-parse", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "code.txt"), []byte(fixture), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := Resolve(Ref{RepoPath: dir, File: "code.txt", StartLine: 1, EndLine: 1}, now); err == nil {
			t.Error("Resolve in non-git dir: want error, got nil")
		}
	})
}

func TestRepoIdentity(t *testing.T) {
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	ref := func(repo string) Ref {
		return Ref{RepoPath: repo, File: "code.txt", StartLine: 1, EndLine: 2}
	}

	t.Run("origin remote becomes host/org/name", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		cmd := exec.Command(
			"git", "-C", repo, "remote", "add", "origin", "git@github.com:mad01/example.git",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git remote add: %v\n%s", err, out)
		}
		p, err := Resolve(ref(repo), now)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if want := "github.com/mad01/example"; p.Repo != want {
			t.Errorf("Repo = %q, want %q", p.Repo, want)
		}
	})

	t.Run("no origin remote yields empty identity", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		p, err := Resolve(ref(repo), now)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if p.Repo != "" {
			t.Errorf("Repo = %q, want empty without an origin remote", p.Repo)
		}
	})
}

func TestCanonicalRepo(t *testing.T) {
	cases := []struct {
		remote, want string
	}{
		{"git@github.com:mad01/thismoon.git", "github.com/mad01/thismoon"},
		{"https://github.com/mad01/thismoon.git", "github.com/mad01/thismoon"},
		{"https://github.com/mad01/thismoon", "github.com/mad01/thismoon"},
		{"https://github.com/mad01/thismoon/", "github.com/mad01/thismoon"},
		{"ssh://git@github.com/mad01/thismoon.git", "github.com/mad01/thismoon"},
		{"https://user@git.example.com/org/repo.git", "git.example.com/org/repo"},
		{"/some/local/path", ""},
		{"ssh://git@host:2222/org/repo", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := canonicalRepo(tc.remote); got != tc.want {
			t.Errorf("canonicalRepo(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

func TestCheck(t *testing.T) {
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)

	resolve := func(t *testing.T, repo string) Pin {
		t.Helper()
		p, err := Resolve(Ref{RepoPath: repo, File: "code.txt", StartLine: 2, EndLine: 4}, now)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		return p
	}
	write := func(t *testing.T, repo, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, "code.txt"), []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	t.Run("untouched holds", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		if ok, reason := Check(resolve(t, repo)); !ok || reason != "" {
			t.Errorf("Check = (%v, %q), want (true, \"\")", ok, reason)
		}
	})

	t.Run("edit inside range flips to content changed", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		p := resolve(t, repo)
		write(t, repo, "l1\nl2\nEDITED\nl4\nl5\n")
		if ok, reason := Check(p); ok || reason != "content changed" {
			t.Errorf("Check = (%v, %q), want (false, \"content changed\")", ok, reason)
		}
	})

	t.Run("edit below range still holds", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		p := resolve(t, repo)
		write(t, repo, "l1\nl2\nl3\nl4\nEDITED\n")
		if ok, reason := Check(p); !ok || reason != "" {
			t.Errorf("Check = (%v, %q), want (true, \"\")", ok, reason)
		}
	})

	t.Run("truncation puts range out of bounds", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		p := resolve(t, repo)
		write(t, repo, "l1\nl2\n")
		if ok, reason := Check(p); ok || reason != "range out of bounds" {
			t.Errorf("Check = (%v, %q), want (false, \"range out of bounds\")", ok, reason)
		}
	})

	t.Run("removed file reports file missing", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		p := resolve(t, repo)
		if err := os.Remove(filepath.Join(repo, "code.txt")); err != nil {
			t.Fatalf("remove: %v", err)
		}
		if ok, reason := Check(p); ok || reason != "file missing" {
			t.Errorf("Check = (%v, %q), want (false, \"file missing\")", ok, reason)
		}
	})

	t.Run("removed repo reports repo missing", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		p := resolve(t, repo)
		if err := os.RemoveAll(repo); err != nil {
			t.Fatalf("remove repo: %v", err)
		}
		if ok, reason := Check(p); ok || reason != "repo missing" {
			t.Errorf("Check = (%v, %q), want (false, \"repo missing\")", ok, reason)
		}
	})

	t.Run("reverting the edit restores the pin", func(t *testing.T) {
		repo := newTestRepo(t, "code.txt", fixture)
		p := resolve(t, repo)
		write(t, repo, "l1\nl2\nEDITED\nl4\nl5\n")
		if ok, _ := Check(p); ok {
			t.Fatal("precondition: edited pin should not hold")
		}
		write(t, repo, fixture)
		if ok, reason := Check(p); !ok || reason != "" {
			t.Errorf("Check after revert = (%v, %q), want (true, \"\")", ok, reason)
		}
	})
}

package repofind

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseRemote(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"ssh with .git", "git@github.com:org/repo.git", "org/repo"},
		{"ssh without .git", "git@github.com:org/repo", "org/repo"},
		{"https with .git", "https://github.com/org/repo.git", "org/repo"},
		{"https without .git", "https://github.com/org/repo", "org/repo"},
		{"nested org ssh", "git@github.com:a/b/c.git", "a/b/c"},
		{"empty", "", ""},
		{"unrecognised", "notaurl", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseRemote(tc.url)
			if got != tc.want {
				t.Errorf("ParseRemote(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestParseHost(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"ssh", "git@github.com:org/repo.git", "github.com"},
		{"ssh other user", "deploy@git.example.com:org/repo", "git.example.com"},
		{"https", "https://github.com/org/repo.git", "github.com"},
		{"https no path", "https://github.com", "github.com"},
		{"empty", "", ""},
		{"unrecognised", "notaurl", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseHost(tc.url)
			if got != tc.want {
				t.Errorf("ParseHost(%q) = %q, want %q", tc.url, got, tc.want)
			}
		})
	}
}

func TestFind_populatesHost(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "org/alpha", "git@github.com:org/alpha.git")

	repos, err := Find([]string{root}, nil)
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}
	if len(repos) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(repos))
	}
	if repos[0].Host != "github.com" {
		t.Errorf("expected host github.com, got %q", repos[0].Host)
	}
}

func TestIsRepo(t *testing.T) {
	dir := t.TempDir()
	if IsRepo(dir) {
		t.Fatal("empty dir should not be a repo")
	}
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if !IsRepo(dir) {
		t.Fatal("dir with .git should be a repo")
	}
}

func TestFind_basic(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "org/alpha", "git@github.com:org/alpha.git")
	makeRepo(t, root, "org/beta", "git@github.com:org/beta.git")
	makeRepo(t, root, "org/skip", "git@github.com:org/skip.git")

	repos, err := Find([]string{root}, []string{"org/skip"})
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}
	if len(repos) != 2 {
		t.Fatalf("expected 2 repos, got %d: %v", len(repos), repos)
	}
	if repos[0].Name != "org/alpha" {
		t.Errorf("expected first repo to be org/alpha, got %s", repos[0].Name)
	}
	if repos[1].Name != "org/beta" {
		t.Errorf("expected second repo to be org/beta, got %s", repos[1].Name)
	}
}

func TestFind_hiddenDirSkipped(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "org/visible", "git@github.com:org/visible.git")

	hidden := filepath.Join(root, ".hidden")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	makeRepo(t, hidden, "org/secret", "git@github.com:org/secret.git")

	repos, err := Find([]string{root}, nil)
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}
	for _, r := range repos {
		if r.Name == "org/secret" {
			t.Error("hidden repo should not be discovered")
		}
	}
}

func TestFind_globExclude(t *testing.T) {
	root := t.TempDir()
	makeRepo(t, root, "foo/bar", "git@github.com:foo/bar.git")
	makeRepo(t, root, "foo/baz", "git@github.com:foo/baz.git")

	repos, err := Find([]string{root}, []string{"foo/*"})
	if err != nil {
		t.Fatalf("Find error: %v", err)
	}
	if len(repos) != 0 {
		t.Errorf("expected 0 repos after glob exclude, got %d", len(repos))
	}
}

// makeRepo creates a fake git repo with a .git/config containing the given remote.
func makeRepo(t *testing.T, root, name, remoteURL string) {
	t.Helper()
	repoDir := filepath.Join(root, name)
	gitDir := filepath.Join(repoDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := "[remote \"origin\"]\n\turl = " + remoteURL + "\n"
	if err := os.WriteFile(filepath.Join(gitDir, "config"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindDedupesOverlappingRoots(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "src")
	repoPath := filepath.Join(nested, "myrepo")
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	repos, err := Find([]string{root, nested}, nil)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if len(repos) != 1 {
		t.Errorf("expected 1 repo from overlapping roots, got %d: %v", len(repos), repos)
	}
}

package search

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWalkRepo pins the skip rules the lexical index and the outline share
// outside a git work tree: hidden directories (the allowed ones too, since
// nothing is tracked here), generated trees, a worktree's .git pointer
// file, oversized files, and .cslignore matches stay out; everything else,
// hidden files at the root included, comes back in lexical order with its
// size.
func TestWalkRepo(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"a.go":                "package a\n",
		"sub/b.go":            "package sub\n",
		"big.txt":             strings.Repeat("x", 100),
		".git/config":         "[core]\n",
		".hidden/c.go":        "package c\n",
		".github/ci.yml":      "on: push\n",
		"node_modules/d/e.js": "x",
		"vendor/f.go":         "package f\n",
		"ignored/g.go":        "package g\n",
		".cslignore":          "ignored/\n",
		"worktree/.git":       "gitdir: /elsewhere\n",
		"worktree/h.go":       "package h\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var got []string
	err := WalkRepo(root, 50, nil, func(f WalkFile) error {
		if f.Abs != filepath.Join(root, f.Rel) {
			t.Errorf("Abs = %q, want it under root for %q", f.Abs, f.Rel)
		}
		if f.Size != int64(len(files[filepath.ToSlash(f.Rel)])) {
			t.Errorf("Size of %q = %d", f.Rel, f.Size)
		}
		got = append(got, filepath.ToSlash(f.Rel))
		return nil
	})
	if err != nil {
		t.Fatalf("WalkRepo: %v", err)
	}
	want := []string{".cslignore", "a.go", "sub/b.go", "worktree/h.go"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("walked %v, want %v", got, want)
	}
}

// TestWalkRepo_VisitErrorStops: the visitor's error ends the walk and comes
// back unchanged.
func TestWalkRepo_VisitErrorStops(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	calls := 0
	err := WalkRepo(root, IndexSizeMax(), nil, func(WalkFile) error {
		calls++
		return os.ErrClosed
	})
	if err != os.ErrClosed || calls != 1 {
		t.Errorf("err = %v after %d calls, want os.ErrClosed after 1", err, calls)
	}
}

// TestWalkRepo_HiddenDirs pins the hidden-directory allowlist in a git work
// tree: tracked files under .github and .claude come back, untracked ones
// under them stay out, every other hidden directory stays out even when git
// tracks its files, and the configured extra names open further
// directories but never .git.
func TestWalkRepo_HiddenDirs(t *testing.T) {
	gitAvailable(t)
	root := initGitRepo(t, t.TempDir())
	files := map[string]string{
		".github/workflows/ci.yml":    "on: push\n",
		".github/local.yml":           "scratch\n",
		".claude/settings.json":       "{}\n",
		".claude/settings.local.json": "{}\n",
		".circleci/config.yml":        "version: 2\n",
		".idea/workspace.xml":         "<project/>\n",
		".cache/blob":                 "x\n",
		".venv/bin/python":            "x\n",
		".terraform/state":            "x\n",
		".gradle/cache":               "x\n",
		".next/page.js":               "x\n",
	}
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, root, "add",
		".github/workflows/ci.yml", ".claude/settings.json",
		".circleci/config.yml", ".idea/workspace.xml")

	walk := func(hidden []string) []string {
		t.Helper()
		var got []string
		err := WalkRepo(root, IndexSizeMax(), hidden, func(f WalkFile) error {
			got = append(got, filepath.ToSlash(f.Rel))
			return nil
		})
		if err != nil {
			t.Fatalf("WalkRepo(%v): %v", hidden, err)
		}
		return got
	}

	want := []string{".claude/settings.json", ".github/workflows/ci.yml", "hello.txt"}
	if got := walk(nil); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("default walk %v, want %v", got, want)
	}
	want = []string{
		".circleci/config.yml",
		".claude/settings.json",
		".github/workflows/ci.yml",
		"hello.txt",
	}
	if got := walk([]string{".circleci", ".git"}); strings.Join(
		got,
		" ",
	) != strings.Join(
		want,
		" ",
	) {
		t.Errorf("walk with extras %v, want %v", got, want)
	}
}

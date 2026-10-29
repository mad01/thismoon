// Package lexicaltest builds the small zoekt index the lexical search tests
// share, so the lexical package, the csl_search tool, and the csl search
// command are all checked against the same files.
package lexicaltest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// Index indexes a two-file repo named test/repo into a fresh index dir and
// returns the dir and the repo. a.go holds IndexRepo twice and
// symbolSections once; b.go holds Blocks. No file holds both IndexRepo and
// Blocks. The index has no state.json, so the repo counts as unindexed in a
// zero-result hint.
func Index(t testing.TB) (string, finder.Repo) {
	t.Helper()
	repoDir := t.TempDir()
	files := map[string]string{
		"a.go": "package a\n\nfunc IndexRepo() {}\n\n// IndexRepo is called by symbolSections.\nfunc symbolSections() {}\n",
		"b.go": "package b\n\nfunc Blocks() {}\n",
	}
	for rel, content := range files {
		if err := os.WriteFile(filepath.Join(repoDir, rel), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	repo := finder.Repo{Name: "test/repo", Path: repoDir}
	indexDir := t.TempDir()
	if err := search.IndexRepo(indexDir, repo, nil); err != nil {
		t.Fatalf("IndexRepo: %v", err)
	}
	return indexDir, repo
}

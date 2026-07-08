package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// makeFakeRepo creates a directory that looks like a git repo to finder.Walk:
// a .git/config file with an [remote "origin"] url. No git subprocess needed.
func makeFakeRepo(t *testing.T, root, org, repoName string) string {
	t.Helper()
	dir := filepath.Join(root, org, repoName)
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	cfg := "[remote \"origin\"]\n\turl = git@github.com:" + org + "/" + repoName + ".git\n"
	if err := os.WriteFile(filepath.Join(dir, ".git", "config"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write git config: %v", err)
	}
	return dir
}

// setupRepoEnv builds a fake HOME with a csl config pointing at a temp root
// containing the given repos (one per "org/name" pair). Returns the temp root
// and a cleanup that restores HOME.
func setupRepoEnv(t *testing.T, repos []struct{ Org, Name string }) (string, func()) {
	t.Helper()
	tmp := t.TempDir()
	reposRoot := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(reposRoot, 0o755); err != nil {
		t.Fatalf("mkdir repos root: %v", err)
	}
	for _, r := range repos {
		makeFakeRepo(t, reposRoot, r.Org, r.Name)
	}

	cfgDir := filepath.Join(tmp, ".config", "csl")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	cfg := "dirs:\n  - " + reposRoot + "\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	origHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", tmp)
	return reposRoot, func() { _ = os.Setenv("HOME", origHome) }
}

// TestResolveRepoWhitespace covers the whitespace normalization bug:
// users often type or paste a repo name with trailing/leading whitespace,
// including exotic Unicode spaces (NBSP, narrow no-break space, zero-width
// no-break space). Matching must tolerate all of these.
func TestResolveRepoWhitespace(t *testing.T) {
	_, cleanup := setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
		{"mad01", "other-repo"},
		{"someone", "unrelated"},
	})
	defer cleanup()

	tests := []struct {
		name    string
		query   string
		wantErr bool
		wantHit string // expected repo.Name on success
	}{
		{name: "clean match", query: "octo", wantHit: "mad01/octo"},
		{name: "trailing ascii space", query: "octo ", wantHit: "mad01/octo"},
		{name: "leading ascii space", query: " octo", wantHit: "mad01/octo"},
		{name: "leading and trailing space", query: "  octo  ", wantHit: "mad01/octo"},
		{name: "trailing tab", query: "octo\t", wantHit: "mad01/octo"},
		{name: "trailing newline", query: "octo\n", wantHit: "mad01/octo"},
		{name: "non-breaking space (U+00A0)", query: "octo ", wantHit: "mad01/octo"},
		{name: "narrow no-break space (U+202F)", query: "octo ", wantHit: "mad01/octo"},
		{name: "ideographic space (U+3000)", query: "octo　", wantHit: "mad01/octo"},
		{name: "mixed case still works", query: "OCTO ", wantHit: "mad01/octo"},
		{name: "whitespace-only is rejected", query: "   ", wantErr: true},
		{name: "empty is rejected", query: "", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo, err := resolveRepo(tc.query)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveRepo(%q) = %+v, want error", tc.query, repo)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveRepo(%q) error: %v", tc.query, err)
			}
			if repo.Name != tc.wantHit {
				t.Errorf("resolveRepo(%q).Name = %q, want %q", tc.query, repo.Name, tc.wantHit)
			}
		})
	}
}

// TestResolveRepoExactDisambiguation checks that trailing whitespace does not
// defeat the exact-match tiebreaker when multiple repos fuzzy-match.
func TestResolveRepoExactDisambiguation(t *testing.T) {
	_, cleanup := setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
		{"mad01", "octopus"},
	})
	defer cleanup()

	// Query "octo" fuzzy-matches both; the exact-match tiebreaker should
	// still pick "mad01/octo" even when the query has trailing whitespace.
	repo, err := resolveRepo("mad01/octo ")
	if err != nil {
		t.Fatalf("resolveRepo: %v", err)
	}
	if repo.Name != "mad01/octo" {
		t.Errorf("expected exact match to win, got %q", repo.Name)
	}
}

// TestHandleRepoLookupWhitespace verifies the MCP tool layer handles
// whitespace-padded names gracefully.
func TestHandleRepoLookupWhitespace(t *testing.T) {
	_, cleanup := setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
		{"someone", "unrelated"},
	})
	defer cleanup()

	queries := []string{"octo", "octo ", " octo", "octo ", "OCTO\t"}
	for _, q := range queries {
		t.Run(q, func(t *testing.T) {
			_, out, err := handleRepoLookup(context.Background(), nil, repoLookupInput{Name: q})
			if err != nil {
				t.Fatalf("handleRepoLookup(%q) error: %v", q, err)
			}
			if len(out.Matches) == 0 {
				t.Fatalf("handleRepoLookup(%q) returned no matches", q)
			}
			found := false
			for _, m := range out.Matches {
				if m.Name == "mad01/octo" {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("handleRepoLookup(%q) did not return mad01/octo; got %+v", q, out.Matches)
			}
		})
	}
}

// TestHandleRepoLookupRejectsEmpty ensures whitespace-only names are rejected
// the same way empty names are.
func TestHandleRepoLookupRejectsEmpty(t *testing.T) {
	_, cleanup := setupRepoEnv(t, []struct{ Org, Name string }{
		{"mad01", "octo"},
	})
	defer cleanup()

	for _, q := range []string{"", " ", "\t\n", " "} {
		_, _, err := handleRepoLookup(context.Background(), nil, repoLookupInput{Name: q})
		if err == nil {
			t.Errorf("handleRepoLookup(%q) expected error, got nil", q)
		}
	}
}

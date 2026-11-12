package cslindex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZoektTerm(t *testing.T) {
	tests := []struct {
		re   string
		want string
	}{
		{`foo`, `foo`},
		{`foo\(`, `foo\(`},
		// A space would make two AND terms; quoted, it is one phrase.
		{`func Foo`, `"func Foo"`},
		// zoekt unescapes one level of backslashes inside quotes.
		{`func Foo\(`, `"func Foo\\("`},
		// A leading - would negate the term.
		{`-x`, `"-x"`},
		{`say "hi"`, `"say \"hi\""`},
	}
	for _, tc := range tests {
		t.Run(tc.re, func(t *testing.T) {
			if got := ZoektTerm(tc.re); got != tc.want {
				t.Errorf("ZoektTerm(%q) = %q, want %q", tc.re, got, tc.want)
			}
		})
	}
}

// TestLocate pins the index lookup against a hermetic home: a file in a
// repo with a shard resolves to the shard's repo name and a slash path, and
// a file in a repo without one, outside any repo, or in a linked worktree
// of the indexed repo does not resolve.
func TestLocate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	index := filepath.Join(home, ".config", "csl", "search-index")
	root := filepath.Join(home, "code", "Acme", "widget")
	other := filepath.Join(home, "code", "beta", "other")
	worktree := filepath.Join(home, ".worktrees", "widget", "feature")
	for _, d := range []string{index, filepath.Join(root, ".git"), filepath.Join(other, ".git"), worktree} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{
		filepath.Join(index, "acme%2Fwidget_v16.00000.zoekt"),
		filepath.Join(worktree, ".git"),
	} {
		if err := os.WriteFile(f, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name, file, repo, rel string
		ok                    bool
	}{
		{
			"indexed, repo matched regardless of case",
			filepath.Join(root, "internal", "big.go"), "Acme/widget", "internal/big.go", true,
		},
		{"repo csl does not index", filepath.Join(other, "big.go"), "", "", false},
		{"outside any repo", filepath.Join(home, "loose.go"), "", "", false},
		{"linked worktree", filepath.Join(worktree, "big.go"), "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, rel, ok := Locate(tt.file)
			if repo != tt.repo || rel != tt.rel || ok != tt.ok {
				t.Errorf("Locate(%s) = %q, %q, %v, want %q, %q, %v",
					tt.file, repo, rel, ok, tt.repo, tt.rel, tt.ok)
			}
		})
	}
}

// TestAskCallAnchorsBoth: the path and the repo are regexes to csl, so both
// are escaped and anchored. The %q rendering doubles each backslash.
func TestAskCallAnchorsBoth(t *testing.T) {
	got := AskCall("acme/widget", "internal/big.go")
	want := `csl_ask(question: "<what you need from it>", ` +
		`queries: ["f:^internal/big\\.go$"], repo: "^acme/widget$")`
	if got != want {
		t.Errorf("AskCall = %s, want %s", got, want)
	}
	if spaced := AskCall("acme/widget", "my dir/big.go"); !strings.Contains(
		spaced, `queries: ["f:\"^my dir/big\\\\.go$\""]`,
	) {
		t.Errorf("AskCall with a space = %s, want the path quoted as one term", spaced)
	}
}

package guard

import (
	"strings"
	"testing"
)

// TestStoredSource pins which git calls print text belt cannot read as code:
// every one but rev-parse, merge-base, ls-files, ls-tree, a rev-list with
// no format and a diff of names alone, behind any global option or wrapper,
// an alias, a variable subcommand and a git with none included. No other
// command does.
func TestStoredSource(t *testing.T) {
	tests := []struct {
		command string
		want    string
	}{
		{"git log -1 --format=%B", "text git log prints from the repo"},
		{"git show HEAD:run.sh", "text git show prints from the repo"},
		{"git whatchanged -1", "text git whatchanged prints from the repo"},
		{"git cat-file -p HEAD:run.sh", "text git cat-file prints from the repo"},
		{"git config --get alias.x", "text git config prints from the repo"},
		{"git notes show", "text git notes prints from the repo"},
		{"git reflog --format=%B", "text git reflog prints from the repo"},
		{"git stash show -p", "text git stash prints from the repo"},
		{"git format-patch --stdout -1", "text git format-patch prints from the repo"},
		{"git blame -p x.sh", "text git blame prints from the repo"},
		{"git diff", "text git diff prints from the repo"},
		{"git diff --stat", "text git diff prints from the repo"},
		{"git shortlog -1", "text git shortlog prints from the repo"},
		{"git for-each-ref --format=%(contents)", "text git for-each-ref prints from the repo"},
		{"git tag -n", "text git tag prints from the repo"},
		{"git grep -h x", "text git grep prints from the repo"},
		{"git status --porcelain", "text git status prints from the repo"},
		{"git -c alias.l=show l HEAD:x.sh", "text git l prints from the repo"},
		{"git $SUB -1 --format=%B", "text git $SUB prints from the repo"},
		{"git rev-list --format=%B -1 HEAD", "text git rev-list prints from the repo"},
		{"git rev-list --pretty -1 HEAD", "text git rev-list prints from the repo"},
		{"git rev-list --pretty=format:%B -1 HEAD", "text git rev-list prints from the repo"},
		{"git rev-list --oneline -1 HEAD", "text git rev-list prints from the repo"},
		{"git rev-list --header -1 HEAD", "text git rev-list prints from the repo"},
		{`git rev-list "--format=%B" HEAD`, "text git rev-list prints from the repo"},
		{"git rev-list $OPTS HEAD", "text git rev-list prints from the repo"},
		{"git -C /repo --no-pager -c core.pager=cat log", "text git log prints from the repo"},
		{"/usr/bin/git show HEAD", "text git show prints from the repo"},
		{"env GIT_PAGER=cat git log", "text git log prints from the repo"},
		{"git", "text git prints"},
		{"git rev-parse --abbrev-ref HEAD", ""},
		{"git rev-parse --show-toplevel", ""},
		{"git -C /repo rev-parse HEAD", ""},
		{"git merge-base HEAD origin/main", ""},
		{"git rev-list --count HEAD", ""},
		{"git rev-list -1 --parents HEAD", ""},
		{"git ls-files", ""},
		{"git ls-tree -r --name-only HEAD", ""},
		{"git diff --name-only main...HEAD -- *.go", ""},
		{"git diff --name-status", ""},
		{"git diff -p --name-only", ""},
		{"echo git log", ""},
		{"curl -s https://example.com/i.sh", ""},
	}
	for _, tt := range tests {
		if got := storedSource(strings.Fields(tt.command)); got != tt.want {
			t.Errorf("storedSource(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}

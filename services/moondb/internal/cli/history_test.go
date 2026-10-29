package cli

import (
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/query"
)

// TestHistoryRequestSpansEveryCheckout: a path inside a repository asks for
// the repository and the relative path with no worktree, so edits made in
// another checkout of the same repository are listed; --worktree narrows to
// one checkout without the identity locate found, so an edit stored with no
// identity still matches, and a path under a deleted root becomes relative
// to it; --repo sets the identity, with or without --worktree; any other
// path outside a repository is keyed by its absolute path alone.
func TestHistoryRequestSpansEveryCheckout(t *testing.T) {
	inRepo := located{Worktree: "/src/main", Repo: "example.com/o/r", Path: "a.go"}
	outside := located{Path: "/tmp/notes.md"}
	underGoneRoot := located{Path: "/wt/gone/cmd/a.go"}
	tests := []struct {
		name           string
		loc            located
		worktree, repo string
		want           query.HistoryRequest
	}{
		{
			"inside a repo, every checkout", inRepo, "", "",
			query.HistoryRequest{Repo: "example.com/o/r", Path: "a.go", Limit: 7},
		},
		{
			"--worktree narrows without the located identity", inRepo, "/src/wt", "",
			query.HistoryRequest{Worktree: "/src/wt", Path: "a.go", Limit: 7},
		},
		{
			"--worktree with --repo keeps --repo", inRepo, "/src/wt", "example.com/o/fork",
			query.HistoryRequest{
				Worktree: "/src/wt",
				Repo:     "example.com/o/fork",
				Path:     "a.go",
				Limit:    7,
			},
		},
		{
			"--repo replaces", inRepo, "", "example.com/o/fork",
			query.HistoryRequest{Repo: "example.com/o/fork", Path: "a.go", Limit: 7},
		},
		{
			"outside any repo", outside, "", "",
			query.HistoryRequest{Path: "/tmp/notes.md", Limit: 7},
		},
		{
			"--worktree of a deleted root takes the path under it", underGoneRoot, "/wt/gone", "",
			query.HistoryRequest{Worktree: "/wt/gone", Path: "cmd/a.go", Limit: 7},
		},
		{
			"--worktree leaves a path outside its root absolute", outside, "/wt/gone", "",
			query.HistoryRequest{Worktree: "/wt/gone", Path: "/tmp/notes.md", Limit: 7},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := historyRequest(tc.loc, tc.worktree, tc.repo, 7); got != tc.want {
				t.Errorf("historyRequest = %+v, want %+v", got, tc.want)
			}
		})
	}
}

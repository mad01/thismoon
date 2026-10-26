package cli

import (
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/query"
)

// TestHistoryRequestSpansEveryCheckout: a path inside a repository asks for
// the repository and the relative path with no worktree, so edits made in
// another checkout of the same repository are listed; --worktree narrows to
// one checkout and --repo replaces the identity locate found; a path outside
// any repository is keyed by its absolute path alone.
func TestHistoryRequestSpansEveryCheckout(t *testing.T) {
	inRepo := located{Worktree: "/src/main", Repo: "example.com/o/r", Path: "a.go"}
	outside := located{Path: "/tmp/notes.md"}
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
			"--worktree narrows", inRepo, "/src/wt", "",
			query.HistoryRequest{
				Worktree: "/src/wt",
				Repo:     "example.com/o/r",
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
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := historyRequest(tc.loc, tc.worktree, tc.repo, 7); got != tc.want {
				t.Errorf("historyRequest = %+v, want %+v", got, tc.want)
			}
		})
	}
}

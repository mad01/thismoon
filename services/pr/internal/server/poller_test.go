package server

import (
	"testing"

	"github.com/mad01/thismoon/services/pr/internal/github"
)

func TestRepoArchived(t *testing.T) {
	tests := []struct {
		name string
		prs  []github.PullRequest
		want bool
	}{
		{name: "no PRs", prs: nil, want: false},
		{
			name: "active repo",
			prs:  []github.PullRequest{{Base: github.Ref{Repo: github.RefRepo{Archived: false}}}},
			want: false,
		},
		{
			name: "archived repo",
			prs:  []github.PullRequest{{Base: github.Ref{Repo: github.RefRepo{Archived: true}}}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := repoArchived(tt.prs); got != tt.want {
				t.Errorf("repoArchived() = %v, want %v", got, tt.want)
			}
		})
	}
}

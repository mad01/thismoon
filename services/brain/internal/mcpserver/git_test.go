package mcpserver

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/testgit"
)

// TestWriteToolsReportAFailedCommit runs brain_write, brain_edit and
// brain_promote in a git vault whose commits fail two ways: a pre-commit
// hook refuses them, and git cannot read its global config. Each tool fails,
// says the note is changed on disk but not committed, and leaves nothing
// staged.
func TestWriteToolsReportAFailedCommit(t *testing.T) {
	tests := []struct {
		name   string
		breaks func(t *testing.T, root string)
		want   string
	}{
		{
			"hook refuses",
			func(t *testing.T, root string) {
				testgit.Hook(t, root, "pre-commit", "echo 'hook says no' >&2\nexit 1\n")
			},
			"hook says no",
		},
		{
			"git cannot read its config",
			func(t *testing.T, _ string) { testgit.UnreadableGlobalConfig(t) },
			"unable to access",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := setupFixtureVault(t)
			testgit.Isolate(t)
			testgit.Init(t, root)
			testgit.Commit(t, root, "seed", nil)
			tt.breaks(t, root)
			ctx := context.Background()
			h := testServer(t)

			_, _, writeErr := h.handleWrite(ctx, nil, writeInput{
				Tier: "inbox", Title: "Brand New Thought", Body: "fresh",
			})
			_, _, editErr := h.handleEdit(ctx, nil, editInput{
				Path: "notes/concept-alpha.md", Append: "\nmore\n",
			})
			_, _, promoteErr := h.handlePromote(
				ctx,
				nil,
				promoteInput{Path: "inbox/raw-thought.md"},
			)
			for tool, err := range map[string]error{
				"brain_write": writeErr, "brain_edit": editErr, "brain_promote": promoteErr,
			} {
				if err == nil ||
					!strings.Contains(
						err.Error(),
						"is changed on disk, but the git commit failed",
					) ||
					!strings.Contains(err.Error(), tt.want) {
					t.Errorf("%s: err = %v, want a failed commit naming %q", tool, err, tt.want)
				}
			}
			if got := testgit.Subjects(t, root); !slices.Equal(got, []string{"seed"}) {
				t.Errorf("commits = %q, want the seed alone", got)
			}
			if staged := testgit.Staged(t, root); len(staged) != 0 {
				t.Errorf("staged = %q, want nothing", staged)
			}
		})
	}
}

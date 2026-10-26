package mcpserver

import (
	"context"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// registerInflightTool adds the tool that starts from a checkout: which of
// its files other sessions edited without a commit taking the change.
func registerInflightTool(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "moondb_inflight",
		Description: "List the files of one checkout that other agent sessions edited and no commit has taken yet: " +
			"work in flight that a commit, reset, stash or rebase in that checkout could sweep up or throw away. " +
			"`worktree` is REQUIRED (absolute checkout root). Give `exclude_session` (your own session id) so your own edits do not count. " +
			"An edit is in flight while neither it nor a later edit of the same file has a link from a commit made in this checkout. " +
			"A file a person committed or reverted outside any session stays listed until the window passes. " +
			fmt.Sprintf(
				"So does one whose last edit left no content to match a commit by, even when a session saw the commit: "+
					"a delete, a path capture.exclude matches, a NotebookEdit, or a file over %d MiB. ",
				moondb.MaxChunkBytes>>20,
			) +
			"Check `git status` before acting on a listed file. " +
			"Each file lists the sessions behind it (id, title, last edit, edit count), newest first, and `linked_commit`, " +
			"the commit that took the file's newest committed edit, or null. " +
			fmt.Sprintf(
				"`since` bounds the edits read: an RFC 3339 time or a duration such as 24h or 7d (default %dh). "+
					"`limit` caps the files (default %d). ",
				int(moondb.InflightWindow.Hours()),
				moondb.InflightLimit,
			) +
			"Follow up with moondb_history on a path for its edits, or moondb_sessions for a session.",
		Annotations: readOnly(),
	}, h.handleInflight)
}

type inflightInput struct {
	Worktree       string `json:"worktree"                  jsonschema:"absolute root of the checkout (required)"`
	ExcludeSession string `json:"exclude_session,omitempty" jsonschema:"your own session id; its edits never count"`
	Since          string `json:"since,omitempty"           jsonschema:"reads edits since then: an RFC 3339 time, or a duration before now such as 24h or 7d"`
	Limit          int    `json:"limit,omitempty"           jsonschema:"maximum files to return, newest edit first"`
}

type inflightOutput struct {
	Worktree string               `json:"worktree"`
	Files    []store.InFlightFile `json:"files"`
	Summary  string               `json:"summary"`
	URL      string               `json:"url"      jsonschema:"web page where the user can browse sessions"`
}

func (h *handlers) handleInflight(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in inflightInput,
) (*mcp.CallToolResult, inflightOutput, error) {
	v, err := h.client.Inflight(ctx, client.InflightQuery{
		Worktree:       in.Worktree,
		ExcludeSession: in.ExcludeSession,
		Since:          in.Since,
		Limit:          in.Limit,
	})
	if err != nil {
		return nil, inflightOutput{}, err
	}
	if v.Files == nil {
		v.Files = []store.InFlightFile{}
	}
	return nil, inflightOutput{
		Worktree: v.Worktree,
		Files:    v.Files,
		Summary:  inflightSummary(v.Worktree, v.Files),
		URL:      h.webURL,
	}, nil
}

// inflightSummary says in one line how many files are in flight and which
// one changed last, by whom.
func inflightSummary(worktree string, files []store.InFlightFile) string {
	if len(files) == 0 {
		return "no files in " + worktree + " have edits no commit has taken yet"
	}
	f := files[0]
	by := f.Sessions[0]
	return fmt.Sprintf(
		"%d files in %s have edits no commit has taken yet; newest %s, edited %s by %q (%s)",
		len(files), worktree, f.Path, f.LastEditAt.Format("2006-01-02 15:04"), by.Title, by.ID,
	)
}

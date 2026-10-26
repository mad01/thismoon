package cli

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/store"
)

var (
	historyWorktree string
	historyRepo     string
	historyLimit    int
	historyJSON     bool
)

var historyCmd = &cobra.Command{
	Use:   "history <path>",
	Short: "List the recorded edits of a path, newest first",
	Long: `List every recorded edit of a file, newest first: when, which tool, which
agent, which session and turn, and where the before-content came from. The
list spans every checkout of the repository, since agents edit in worktrees
while a person reads in the main checkout; --worktree narrows it to one.`,
	Args: cobra.ExactArgs(1),
	RunE: runHistory,
}

func init() {
	historyCmd.Flags().
		StringVar(&historyWorktree, "worktree", "", "checkout root (default: every checkout)")
	historyCmd.Flags().
		StringVar(&historyRepo, "repo", "", "repository identity (default: the checkout's origin)")
	historyCmd.Flags().
		IntVar(&historyLimit, "limit", 0, "most edits to list (default: the server's)")
	historyCmd.Flags().BoolVar(&historyJSON, "json", false, "print the API answer as JSON")
	rootCmd.AddCommand(historyCmd)
}

func runHistory(cmd *cobra.Command, args []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	loc, err := locate(ctx, args[0])
	if err != nil {
		return err
	}
	req := historyRequest(loc, historyWorktree, historyRepo, historyLimit)
	edits, err := client.New(apiURL()).History(ctx, req)
	if err != nil {
		return err
	}
	if historyJSON {
		return printJSON(cmd.OutOrStdout(), edits)
	}
	printEdits(cmd.OutOrStdout(), edits)
	return nil
}

// historyRequest is the query for a located path: the repository and the
// path inside it, every checkout included, unless --worktree names one. A
// path outside any repository is keyed by its absolute path alone. The
// flags stand in for what locate found.
func historyRequest(loc located, worktree, repo string, limit int) query.HistoryRequest {
	req := query.HistoryRequest{Worktree: worktree, Repo: loc.Repo, Path: loc.Path, Limit: limit}
	if repo != "" {
		req.Repo = repo
	}
	return req
}

// printEdits renders edits as a table.
func printEdits(w io.Writer, edits []store.Edit) {
	if len(edits) == 0 {
		fmt.Fprintln(w, "no edits recorded for that path")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AT\tTOOL\tKIND\tAGENT\tSESSION\tTURN\tBEFORE\tBRANCH")
	for _, e := range edits {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			when(e.At), e.Tool, e.Kind, agentLabel(e.AgentType),
			short(e.SessionID), short(e.TurnID), e.BeforeSource, orDash(e.Branch))
	}
	_ = tw.Flush()
}

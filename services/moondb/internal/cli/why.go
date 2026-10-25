package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/query"
)

var (
	whyWorktree string
	whyRepo     string
	whyJSON     bool
)

var whyCmd = &cobra.Command{
	Use:   "why <path>:<line>",
	Short: "Explain one line of one file: the edit, turn, prompt and commit",
	Long: `Answer why a line looks the way it does. A committed line is traced through
git blame to the commit, and the commit's blob for the file to the edit that
produced it. An uncommitted line, or one whose commit moondb never saw, is
matched by its text against the lines recorded edits added, newest first.

The path is resolved against the git checkout it sits in, the same way the
hooks key edits, so a relative path from inside a worktree just works.

Examples:
  moondb why internal/cli/root.go:42
  moondb why --repo github.com/org/repo --worktree /src/repo main.go:10`,
	Args: cobra.ExactArgs(1),
	RunE: runWhy,
}

func init() {
	whyCmd.Flags().
		StringVar(&whyWorktree, "worktree", "", "checkout root (default: the path's own)")
	whyCmd.Flags().
		StringVar(&whyRepo, "repo", "", "repository identity (default: the checkout's origin)")
	whyCmd.Flags().BoolVar(&whyJSON, "json", false, "print the API answer as JSON")
	rootCmd.AddCommand(whyCmd)
}

func runWhy(cmd *cobra.Command, args []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	req, err := whyRequest(ctx, args[0])
	if err != nil {
		return err
	}
	c := client.New(apiURL())
	res, err := c.Why(ctx, req)
	if err != nil {
		return err
	}
	if whyJSON {
		return printJSON(cmd.OutOrStdout(), res)
	}
	printWhy(ctx, cmd.OutOrStdout(), c, res)
	return nil
}

// whyRequest turns "<path>:<line>" into the request, locating the path in
// its checkout unless --worktree and --repo say otherwise.
func whyRequest(ctx context.Context, arg string) (query.WhyRequest, error) {
	i := strings.LastIndexByte(arg, ':')
	if i <= 0 {
		return query.WhyRequest{}, errors.New("want <path>:<line>")
	}
	line, err := strconv.Atoi(arg[i+1:])
	if err != nil || line < 1 {
		return query.WhyRequest{}, fmt.Errorf("line %q is not a number from 1", arg[i+1:])
	}
	loc, err := locate(ctx, arg[:i])
	if err != nil {
		return query.WhyRequest{}, err
	}
	req := query.WhyRequest{Worktree: loc.Worktree, Repo: loc.Repo, Path: loc.Path, Line: line}
	if whyWorktree != "" {
		req.Worktree = whyWorktree
	}
	if whyRepo != "" {
		req.Repo = whyRepo
	}
	return req, nil
}

// printWhy renders the answer for a terminal.
func printWhy(ctx context.Context, w io.Writer, c *client.Client, res query.WhyResult) {
	fmt.Fprintf(w, "%s:%d  (%s)\n", res.Path, res.Line, res.Method)
	if res.Text != "" {
		fmt.Fprintf(w, "  text:     %s\n", firstLine(res.Text, promptPreview))
	}
	if res.Edit == nil {
		fmt.Fprintln(
			w,
			"  nothing recorded explains this line; moondb history <path> lists what was",
		)
		return
	}
	e := res.Edit
	fmt.Fprintf(w, "  edit:     %s %s at %s by %s  [%s]\n",
		e.Tool, e.Kind, when(e.At), agentLabel(e.AgentType), e.ID)
	if res.Turn != nil {
		fmt.Fprintf(w, "  turn:     %s", res.Turn.ID)
		if prompt := promptOf(ctx, c, res.Turn.PromptChunk); prompt != "" {
			fmt.Fprintf(w, "  %q", prompt)
		}
		fmt.Fprintln(w)
	}
	if res.Session != nil {
		fmt.Fprintf(
			w,
			"  session:  %s  %s\n",
			res.Session.ID,
			orDash(sessionLabel(res.Session.Title, res.Session.Cwd)),
		)
	}
	if res.Commit != "" {
		fmt.Fprintf(w, "  commit:   %s\n", res.Commit)
	}
	if len(res.Candidates) > 1 {
		fmt.Fprintf(w, "  also:     %d other edits added this line\n", len(res.Candidates)-1)
	}
	fmt.Fprintf(w, "  url:      %s\n", sessionURL(e.SessionID))
}

// sessionLabel is the title when the harness gave one, else the cwd.
func sessionLabel(title, cwd string) string {
	if title != "" {
		return title
	}
	return cwd
}

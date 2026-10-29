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
	"github.com/mad01/thismoon/services/moondb/internal/store"
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
	if res.Agent != nil {
		printAgent(ctx, w, c, *res.Agent)
	}
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

// printAgent names the subagent that made the edit and the first line of
// its spawn brief, the prompt it was spawned with, or why the brief is
// unknown. Then the first line of the last message its parent sent it
// before the edit, with when it arrived and who sent it, and the count of
// messages sent after the brief.
func printAgent(ctx context.Context, w io.Writer, c *client.Client, a query.WhyAgent) {
	who := a.Label()
	if a.Name != "" && a.Name != a.Type {
		who += " (" + a.Type + ")"
	}
	fmt.Fprintf(w, "  agent:    %s  [%s]\n", who, a.ID)
	brief := promptOf(ctx, c, a.BriefChunk)
	if brief == "" {
		fmt.Fprintf(w, "  brief:    unknown, %s\n", query.MissingBrief(a.Type, a.Recorded))
		return
	}
	fmt.Fprintf(w, "  brief:    %q\n", brief)
	shown := printLastMessage(ctx, w, c, a.LastMessage)
	const later = "  later:    %d messages from its parent after the spawn brief"
	switch {
	case a.LaterMessages == 0:
	case shown:
		fmt.Fprintf(w, later+"\n", a.LaterMessages)
	case a.LastMessage == nil && a.MessagesKept:
		fmt.Fprintf(w, later+", none before this edit\n", a.LaterMessages)
	default:
		fmt.Fprintf(w, later+", none shown\n", a.LaterMessages)
	}
}

// printLastMessage prints the first line of the last message an agent got
// before the edit, with when it arrived and who sent it, and reports
// whether it printed one: not for none, or one whose text serve cannot
// give.
func printLastMessage(
	ctx context.Context,
	w io.Writer,
	c *client.Client,
	m *store.AgentMessage,
) bool {
	if m == nil {
		return false
	}
	text := promptOf(ctx, c, m.Chunk)
	if text == "" {
		return false
	}
	from := ""
	if m.From != "" {
		from = " from " + m.From
	}
	fmt.Fprintf(w, "  last message: %q  (%s%s, before this edit)\n",
		text, when(m.At), from)
	return true
}

// sessionLabel is the title when the harness gave one, else the cwd.
func sessionLabel(title, cwd string) string {
	if title != "" {
		return title
	}
	return cwd
}

package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

// Trace display limits. A short id is matched against the newest
// prefixScan sessions, more than the listing shows by default.
const (
	prefixScan     = 200
	tracePromptLen = 60
)

var traceJSON bool

var traceCmd = &cobra.Command{
	Use:   "trace <session-id>",
	Short: "Show what took time in a session and what it cost",
	Long: fmt.Sprintf(`Trace a session: its wall time split into model, tool, harness and user
time, its tokens by kind and its cost at list prices. Then the %[1]d longest
tool calls and the %[1]d most expensive model calls, each with its turn
number and the first line of its prompt.

The id may be the short form moondb sessions prints. Cost covers the main
conversation: a subagent's model calls are in its own transcript, which
moondb does not keep. A model with no price adds tokens and no cost, and is
named as unpriced. --json prints the whole trace, every span included.`,
		moondb.TraceTopSpans),
	Args: cobra.ExactArgs(1),
	RunE: runTrace,
}

func init() {
	traceCmd.Flags().BoolVar(&traceJSON, "json", false, "print the API answer as JSON")
	rootCmd.AddCommand(traceCmd)
}

func runTrace(cmd *cobra.Command, args []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	st, err := sessionTrace(ctx, client.New(apiURL()), args[0])
	if err != nil {
		return err
	}
	if traceJSON {
		return printJSON(cmd.OutOrStdout(), st)
	}
	printTrace(cmd.OutOrStdout(), st)
	return nil
}

// sessionTrace fetches the trace of the session id names: a full id, or a
// prefix of exactly one of the newest sessions.
func sessionTrace(ctx context.Context, c *client.Client, id string) (trace.SessionTrace, error) {
	st, err := c.SessionTrace(ctx, id)
	if !errors.Is(err, store.ErrNotFound) {
		return st, err
	}
	full, err := resolveSession(ctx, c, id)
	if err != nil {
		return trace.SessionTrace{}, err
	}
	return c.SessionTrace(ctx, full)
}

// resolveSession finds the one recent session whose id starts with prefix.
func resolveSession(ctx context.Context, c *client.Client, prefix string) (string, error) {
	list, err := c.Sessions(ctx, client.SessionQuery{Limit: prefixScan})
	if err != nil {
		return "", err
	}
	var matches []string
	for _, s := range list {
		if strings.HasPrefix(s.ID, prefix) {
			matches = append(matches, s.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf(
			"moondb: no session %q among the newest %d; moondb sessions lists them",
			prefix, prefixScan)
	case 1:
		return matches[0], nil
	}
	return "", fmt.Errorf("moondb: %q matches %d sessions; give more of the id",
		prefix, len(matches))
}

// printTrace renders a session trace: the totals, then the rankings.
func printTrace(w io.Writer, st trace.SessionTrace) {
	turns := "turns"
	if len(st.Turns) == 1 {
		turns = "turn"
	}
	fmt.Fprintf(w, "session %s, %d %s: %s\n\n",
		st.SessionID, len(st.Turns), turns, sessionURL(st.SessionID))
	printTotals(w, st.Totals)

	fmt.Fprintln(w)
	tools := trace.LongestTools(st, moondb.TraceTopSpans)
	if len(tools) == 0 {
		fmt.Fprintln(w, "no tool calls in the recorded transcript lines")
	} else {
		fmt.Fprintln(w, "LONGEST TOOL CALLS")
		printTools(w, tools)
	}

	fmt.Fprintln(w)
	models := trace.CostliestModels(st, moondb.TraceTopSpans)
	if len(models) == 0 {
		fmt.Fprintln(w, "no model calls in the recorded transcript lines")
		return
	}
	fmt.Fprintln(w, "MOST EXPENSIVE MODEL CALLS")
	printModels(w, models)
}

// printTotals is the totals block: time, tokens, cost and call counts.
func printTotals(w io.Writer, t trace.Totals) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	d := trace.FormatDuration
	fmt.Fprintf(tw, "wall\t%s\tmodel %s, tool %s, harness %s, user %s\n",
		d(t.WallMS), d(t.ModelMS), d(t.ToolMS), d(t.HarnessMS), d(t.UserMS))
	k := t.Tokens
	n := trace.FormatTokens
	fmt.Fprintf(tw,
		"tokens\t%s\tinput %s, cache write 5m %s, cache write 1h %s, cache read %s, output %s\n",
		n(k.Total()), n(k.Input), n(k.CacheWrite5m), n(k.CacheWrite1h), n(k.CacheRead),
		n(k.Output))
	unpriced := ""
	if len(t.Unpriced) > 0 {
		unpriced = "unpriced: " + strings.Join(t.Unpriced, ", ")
	}
	fmt.Fprintf(tw, "cost\t%s\t%s\n", trace.FormatTotalsCost(t), unpriced)
	fmt.Fprintf(tw, "calls\t%d\tmodel %d, tool %d (%d failed)\n",
		t.ModelCalls+t.ToolCalls, t.ModelCalls, t.ToolCalls, t.ToolErrors)
	_ = tw.Flush()
}

func printTools(w io.Writer, ranked []trace.Ranked) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TURN\tTOOL\tDURATION\tRESULT\tPROMPT")
	for _, r := range ranked {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\n", r.Seq, r.Span.Name,
			trace.FormatDuration(r.Span.DurationMS), toolResult(r.Span),
			orDash(firstLine(r.Prompt, tracePromptLen)))
	}
	_ = tw.Flush()
}

func printModels(w io.Writer, ranked []trace.Ranked) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TURN\tMODEL\tCOST\tTOKENS\tDURATION\tPROMPT")
	for _, r := range ranked {
		tokens := int64(0)
		if r.Span.Tokens != nil {
			tokens = r.Span.Tokens.Total()
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", r.Seq, orDash(r.Span.Name),
			trace.FormatCost(r.Span.Cost), trace.FormatTokens(tokens),
			trace.FormatDuration(r.Span.DurationMS), orDash(firstLine(r.Prompt, tracePromptLen)))
	}
	_ = tw.Flush()
}

// toolResult is how a tool call ended: ok, error, or unfinished when the
// recorded lines hold no result.
func toolResult(s trace.Span) string {
	switch {
	case s.Unfinished:
		return "unfinished"
	case s.IsError:
		return "error"
	}
	return "ok"
}

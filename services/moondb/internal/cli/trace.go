package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
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

The agents table lists the main conversation, then every subagent that
stopped while recording or was imported, costliest first. Cost covers them
all, and the cost line splits it between the main conversation and the
agents. The rankings take the agents' calls too, each marked with its
agent. A model with no price adds tokens and no cost, and is named as
unpriced. The id may be the short form moondb sessions prints. --json
prints the whole trace, every span included.`,
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
	if len(st.Agents) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "AGENTS")
		printAgents(w, st)
	}

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
	fmt.Fprintf(tw, "wall\t%s\tmodel %s, tool %s, harness %s, user %s, idle %s%s\n",
		d(t.WallMS), d(t.ModelMS), d(t.ToolMS), d(t.HarnessMS), d(t.UserMS), d(t.IdleMS),
		idleNote(t.IdleMS))
	k := t.Tokens
	n := trace.FormatTokens
	fmt.Fprintf(tw,
		"tokens\t%s\tinput %s, cache write 5m %s, cache write 1h %s, cache read %s, output %s\n",
		n(k.Total()), n(k.Input), n(k.CacheWrite5m), n(k.CacheWrite1h), n(k.CacheRead),
		n(k.Output))
	fmt.Fprintf(tw, "cost\t%s\t%s\n", trace.FormatTotalsCost(t), costNote(t))
	calls := fmt.Sprintf("model %d, tool %d (%d failed)", t.ModelCalls, t.ToolCalls, t.ToolErrors)
	if t.TokensAgents != nil {
		calls = fmt.Sprintf("main: %s; agents: model %d, tool %d",
			calls, t.AgentModelCalls, t.AgentToolCalls)
	}
	fmt.Fprintf(tw, "calls\t%d\t%s\n",
		t.ModelCalls+t.ToolCalls+t.AgentModelCalls+t.AgentToolCalls, calls)
	_ = tw.Flush()
}

// idleNote says what idle time is when there is some: no transcript line
// inside a turn for longer than moondb.IdleGapAfter.
func idleNote(idleMS int64) string {
	if idleMS == 0 {
		return ""
	}
	return fmt.Sprintf(" (no transcript line inside a turn for over %d min)",
		int(moondb.IdleGapAfter.Minutes()))
}

// costNote says what the cost line's figure is made of: the main
// conversation's part and the agents', and the models it leaves out.
func costNote(t trace.Totals) string {
	var parts []string
	if t.TokensAgents != nil {
		parts = append(parts, fmt.Sprintf("main %s, agents %s",
			trace.FormatCost(t.CostMain), trace.FormatCost(t.CostAgents)))
	}
	if len(t.Unpriced) > 0 {
		parts = append(parts, "unpriced: "+strings.Join(t.Unpriced, ", "))
	}
	return strings.Join(parts, "; ")
}

// printAgents is the agents table: the main conversation, then each
// subagent costliest first, with the turns it ran under.
func printAgents(w io.Writer, st trace.SessionTrace) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tTYPE\tTURNS\tDURATION\tCALLS\tEDITS\tTOKENS\tCOST")
	t := st.Totals
	mainTokens := t.Tokens
	if t.TokensMain != nil {
		mainTokens = *t.TokensMain
	}
	fmt.Fprintf(tw, "%s\t-\t%d\t%s\t%d\t%d\t%s\t%s\n", trace.MainAgent, len(st.Turns),
		trace.FormatDuration(t.WallMS), t.ModelCalls+t.ToolCalls, st.MainEdits(),
		trace.FormatTokens(mainTokens.Total()), trace.FormatCost(t.CostMain))
	for _, a := range trace.AgentsByCost(st.Agents) {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n", a.Span.Name, orDash(a.Type),
			orDash(joinInts(st.TurnSeqs(a.TurnIDs))), trace.FormatDuration(a.Span.DurationMS),
			a.Totals.ModelCalls+a.Totals.ToolCalls, a.Span.EditCount,
			trace.FormatTokens(a.Totals.Tokens.Total()), agentCost(a))
	}
	_ = tw.Flush()
}

// agentCost is an agent's cost, marked when the transcript's size cap cut
// lines whose calls it leaves out.
func agentCost(a trace.AgentRun) string {
	c := trace.FormatTotalsCost(a.Totals)
	if a.LinesCut > 0 {
		c += fmt.Sprintf(" (%d lines cut)", a.LinesCut)
	}
	return c
}

func joinInts(ns []int) string {
	parts := make([]string, len(ns))
	for i, n := range ns {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

func printTools(w io.Writer, ranked []trace.Ranked) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TURN\tAGENT\tTOOL\tDURATION\tRESULT\tPROMPT")
	for _, r := range ranked {
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\n", r.Seq, r.Who(), r.Span.Name,
			trace.FormatDuration(r.Span.DurationMS), toolResult(r.Span),
			orDash(firstLine(r.Prompt, tracePromptLen)))
	}
	_ = tw.Flush()
}

func printModels(w io.Writer, ranked []trace.Ranked) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TURN\tAGENT\tMODEL\tCOST\tTOKENS\tDURATION\tPROMPT")
	for _, r := range ranked {
		tokens := int64(0)
		if r.Span.Tokens != nil {
			tokens = r.Span.Tokens.Total()
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", r.Seq, r.Who(), orDash(r.Span.Name),
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

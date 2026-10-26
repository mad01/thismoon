package cli

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/scorecard"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

var scorecardJSON bool

var scorecardCmd = &cobra.Command{
	Use:   "scorecard <session-id>",
	Short: "Show how a session spent its context",
	Long: fmt.Sprintf(`Score a session: its time, tokens, cost and calls, split between the main
conversation and its subagents; its tools by calls; its csl searches by
how they came back (zero results with or without a zero_result_hint, cut
at the limit, relaxed, failed); its Read calls by size (a read of a whole
file of %d lines or more is large); the belt hints it was shown, and
whether any agent of the session read or edited the files the shown kof
assertions are pinned to later; and its subagents with the belt ledger's
counts. Flags name the costliest patterns. The hints and the ledger come
from the events service and the pins from keeper-of-facts; a service that
does not answer leaves its part out and the sources line says so.

The text lists the top %d tools and agents. --json prints the whole
scorecard, the shape GET /api/sessions/{id}/scorecard and the
moondb_scorecard tool return. The id may be the short form moondb sessions
prints.`, moondb.LargeReadLines, moondb.ScorecardTopTools),
	Args: cobra.ExactArgs(1),
	RunE: runScorecard,
}

func init() {
	scorecardCmd.Flags().BoolVar(&scorecardJSON, "json", false, "print the API answer as JSON")
	rootCmd.AddCommand(scorecardCmd)
}

func runScorecard(cmd *cobra.Command, args []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	sc, err := sessionScorecard(ctx, client.New(apiURL()), args[0])
	if err != nil {
		return err
	}
	if scorecardJSON {
		return printJSON(cmd.OutOrStdout(), sc)
	}
	printScorecard(cmd.OutOrStdout(), sc)
	return nil
}

// sessionScorecard fetches the scorecard of the session id names: a full
// id, or a prefix of exactly one of the newest sessions.
func sessionScorecard(
	ctx context.Context,
	c *client.Client,
	id string,
) (scorecard.Scorecard, error) {
	sc, err := c.SessionScorecard(ctx, id)
	if !errors.Is(err, store.ErrNotFound) {
		return sc, err
	}
	full, err := resolveSession(ctx, c, id)
	if err != nil {
		return scorecard.Scorecard{}, err
	}
	return c.SessionScorecard(ctx, full)
}

// printScorecard renders a scorecard: the sources, the totals, the flags,
// one line per part, then the tools and agents tables.
func printScorecard(w io.Writer, sc scorecard.Scorecard) {
	fmt.Fprintf(w, "session %s, %d %s: %s\n", sc.SessionID, sc.Totals.Turns,
		noun(sc.Totals.Turns, "turn", "turns"), sessionURL(sc.SessionID))
	fmt.Fprintf(w, "sources: %s\n\n", sourcesLine(sc.Sources))
	printScoreTotals(w, sc.Totals)
	fmt.Fprintln(w)
	if len(sc.Flags) == 0 {
		fmt.Fprintln(w, "FLAGS: none")
	} else {
		fmt.Fprintln(w, "FLAGS")
		for _, f := range sc.Flags {
			fmt.Fprintf(w, "  %s\n", f)
		}
	}
	fmt.Fprintln(w)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "search\t%s\n", searchLine(sc.Search))
	fmt.Fprintf(tw, "reads\t%s\n", readsLine(sc.Reads))
	fmt.Fprintf(tw, "hints\t%s\n", hintsLine(sc.Hints))
	fmt.Fprintf(tw, "agents\t%s\n", agentsLine(sc.Agents))
	_ = tw.Flush()
	if len(sc.Tools) > 0 {
		fmt.Fprintln(w)
		printScoreTools(w, sc.Tools)
	}
	if len(sc.Agents.Runs) > 0 {
		fmt.Fprintln(w)
		printScoreAgents(w, sc.Agents.Runs)
	}
}

// sourcesLine names each input's status, with the detail of one that is
// not ok.
func sourcesLine(s scorecard.Sources) string {
	parts := make([]string, 0, 3)
	for _, src := range []struct {
		name string
		src  scorecard.Source
	}{{"transcripts", s.Transcripts}, {"events", s.Events}, {"kof", s.Kof}} {
		part := src.name + " " + string(src.src.Status)
		if src.src.Detail != "" {
			part += " (" + src.src.Detail + ")"
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ", ")
}

// printScoreTotals is the totals block: time, tokens, cost and calls, with
// the main conversation's and the agents' parts.
func printScoreTotals(w io.Writer, t scorecard.Totals) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	d, n := trace.FormatDuration, trace.FormatTokens
	fmt.Fprintf(tw, "wall\t%s\tmodel %s, tool %s, harness %s, user %s, idle %s\n",
		d(t.WallMS), d(t.ModelMS), d(t.ToolMS), d(t.HarnessMS), d(t.UserMS), d(t.IdleMS))
	fmt.Fprintf(tw, "tokens\t%s\tmain %s, agents %s; cache read %s, cache write %s, output %s\n",
		n(t.Tokens.Total()), n(t.Main.Tokens.Total()), n(t.Agents.Tokens.Total()),
		n(t.Tokens.CacheRead), n(t.Tokens.CacheWrite5m+t.Tokens.CacheWrite1h),
		n(t.Tokens.Output))
	cost := fmt.Sprintf("main %s, agents %s",
		trace.FormatCost(t.Main.Cost), trace.FormatCost(t.Agents.Cost))
	if len(t.Unpriced) > 0 {
		cost += "; unpriced: " + strings.Join(t.Unpriced, ", ")
	}
	fmt.Fprintf(tw, "cost\t%s\t%s\n", trace.FormatCost(t.Cost), cost)
	fmt.Fprintf(tw, "calls\t%d\tmain: %s; agents: %s\n", t.ModelCalls+t.ToolCalls,
		callsPart(t.Main), callsPart(t.Agents))
	_ = tw.Flush()
}

// callsPart is one part's call counts.
func callsPart(p scorecard.Part) string {
	return fmt.Sprintf("model %d, tool %d (%d failed)", p.ModelCalls, p.ToolCalls, p.ToolErrors)
}

// searchLine sums the csl searches.
func searchLine(s scorecard.SearchStats) string {
	if s.Calls == 0 {
		return "no csl searches"
	}
	return fmt.Sprintf("%d csl %s: %d zero (%d with a zero_result_hint), %d cut at the limit, "+
		"%d relaxed, %d failed, %d unread",
		s.Calls, noun(s.Calls, "search", "searches"), s.Zero, s.ZeroResultHint, s.Truncated,
		s.Relaxed, s.Errors, s.Unread)
}

// readsLine sums the Read calls and names the largest.
func readsLine(r scorecard.ReadStats) string {
	if r.Calls == 0 {
		return "no reads"
	}
	line := fmt.Sprintf("%d %s: %d of a whole file, %d large (whole file, %d+ lines), "+
		"%d lines in all, %d unsized, %d failed",
		r.Calls, noun(r.Calls, "read", "reads"), r.Unranged, r.Large, r.LargeLines,
		r.Lines, r.Unsized, r.Errors)
	if l := r.Largest; l != nil {
		ranged := "whole file"
		if l.Ranged {
			ranged = "part of the file"
		}
		line += fmt.Sprintf("; largest %d lines, %s, by %s: %s",
			l.Lines, ranged, agentOf(l.AgentID), l.Path)
	}
	return line
}

// hintsLine sums the hints by id and the kof hints' hit rate. Without
// hints the events service did not answer; the sources line says why.
func hintsLine(h *scorecard.HintStats) string {
	if h == nil {
		return "unavailable: the events service did not answer"
	}
	if h.Shown == 0 {
		return "none shown"
	}
	counts := make([]string, 0, len(h.ByHint))
	for _, c := range h.ByHint {
		counts = append(counts, fmt.Sprintf("%s %d", c.Hint, c.Count))
	}
	line := fmt.Sprintf("%d shown: %s", h.Shown, strings.Join(counts, ", "))
	k := h.Kof
	if len(k.Assertions) == 0 {
		return line
	}
	line += fmt.Sprintf("; kof: %d %s shown, pinned to %d %s, %d read or edited later",
		len(k.Assertions), noun(len(k.Assertions), "assertion", "assertions"),
		k.PinnedPaths, noun(k.PinnedPaths, "file", "files"), k.UsedPaths)
	if k.HitRate != nil {
		line += fmt.Sprintf(" (%.0f%%)", *k.HitRate*100)
	}
	if k.UnknownPins > 0 {
		line += fmt.Sprintf(", %d with unknown pins", k.UnknownPins)
	}
	return line
}

// agentsLine counts the subagents and the ledger's entries.
func agentsLine(a scorecard.AgentStats) string {
	line := fmt.Sprintf("%d %s", a.Count, noun(a.Count, "subagent", "subagents"))
	l := a.Ledger
	if l == nil {
		return line + "; ledger unavailable"
	}
	return line + fmt.Sprintf("; ledger: %d started, %d stopped, %d permission denied, "+
		"%d compactions, %d stop failures",
		l.SubagentStart, l.SubagentStop, l.PermissionDenied, l.PreCompact, l.StopFailure)
}

// printScoreTools is the tools table, the top moondb.ScorecardTopTools.
func printScoreTools(w io.Writer, tools []scorecard.ToolStat) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "TOOL\tCALLS\tBY AGENTS\tFAILED\tTIME")
	for _, t := range tools[:min(len(tools), moondb.ScorecardTopTools)] {
		fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%s\n", t.Name, t.Calls, t.AgentCalls, t.Errors,
			trace.FormatDuration(t.DurationMS))
	}
	if more := len(tools) - moondb.ScorecardTopTools; more > 0 {
		fmt.Fprintf(tw, "(%d more in --json)\t\t\t\t\n", more)
	}
	_ = tw.Flush()
}

// printScoreAgents is the agents table, costliest first, the top
// moondb.ScorecardTopTools.
func printScoreAgents(w io.Writer, runs []scorecard.AgentRun) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tTYPE\tMODEL\tDURATION\tCALLS\tFAILED\tTOKENS\tCOST")
	for _, a := range runs[:min(len(runs), moondb.ScorecardTopTools)] {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\t%d\t%s\t%s\n", orDash(cmp.Or(a.Name, a.ID)),
			orDash(a.Type), orDash(a.Model), trace.FormatDuration(a.DurationMS),
			a.ModelCalls+a.ToolCalls, a.ToolErrors, trace.FormatTokens(a.Tokens.Total()),
			trace.FormatCost(a.Cost))
	}
	if more := len(runs) - moondb.ScorecardTopTools; more > 0 {
		fmt.Fprintf(tw, "(%d more in --json)\t\t\t\t\t\t\t\n", more)
	}
	_ = tw.Flush()
}

// agentOf names who made a call: main, or the subagent's id.
func agentOf(id string) string {
	if id == "" {
		return trace.MainAgent
	}
	return id
}

// noun is one for a count of 1, many otherwise.
func noun(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

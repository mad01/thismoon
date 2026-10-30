package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

var (
	sessionsRepo  string
	sessionsSince string
	sessionsLimit int
	sessionsJSON  bool
	sessionsAll   bool
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List recorded sessions, most recently active first",
	Long: `List the sessions moondb has recorded, most recently active first, with how many turns
and edits each made, its active and wall time, and which repositories it
touched. ACTIVE is the time inside its turns, less the time a turn waited
for input or sat idle; the pauses between turns never count. WALL runs
from the first prompt to the last Stop, pauses included. moondb trace
<session-id> shows where that time went and what it cost.

--since keeps the sessions active since then, whenever they started, and
takes an RFC 3339 time or a span before now: 90m, 24h, 7d.

--repo keeps the sessions that edited the repository, worked in it (their
cwd or a subagent's cwd sat in it) or attribute a part of their cost to
it, and adds two columns. COST is the whole session's cost. ATTRIBUTED
is the part the attribution rule gives the repository, "unknown" when no
price covers that part. Each subagent's cost goes to
the repository most of its edits touched, else its cwd's repository,
else no_repo. The main conversation's goes to the repository most of its
own edits touched, else to orchestration.

moondb sessions hide takes a session out of this listing, the web page
and the moondb_sessions MCP tool; moondb sessions show puts it back.
--all lists the hidden sessions too and adds a HIDDEN column.`,
	Args: cobra.NoArgs,
	RunE: runSessions,
}

func init() {
	sessionsCmd.Flags().
		StringVar(
			&sessionsRepo,
			"repo",
			"",
			"keep sessions that edited this repository, worked in it, or attribute cost to it",
		)
	sessionsCmd.Flags().
		StringVar(&sessionsSince, "since", "", "keep sessions active since this time or span")
	sessionsCmd.Flags().
		IntVar(&sessionsLimit, "limit", 0, "most sessions to list (default: the server's)")
	sessionsCmd.Flags().BoolVar(&sessionsJSON, "json", false, "print the API answer as JSON")
	sessionsCmd.Flags().
		BoolVarP(&sessionsAll, "all", "a", false, "list hidden sessions too, marked in a HIDDEN column")
	rootCmd.AddCommand(sessionsCmd)
}

func runSessions(cmd *cobra.Command, _ []string) error {
	if err := requireConfig(); err != nil {
		return err
	}
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	q := client.SessionQuery{
		Repo:          sessionsRepo,
		Since:         sessionsSince,
		IncludeHidden: sessionsAll,
		Limit:         sessionsLimit,
	}
	sessions, err := client.New(apiURL()).Sessions(ctx, q)
	if err != nil {
		return err
	}
	if sessionsJSON {
		return printJSON(cmd.OutOrStdout(), sessions)
	}
	printSessions(cmd.OutOrStdout(), sessions, sessionsRepo, sessionsAll)
	return nil
}

// printSessions renders session summaries as a table. A repo, the one the
// listing filters by, adds the whole cost and the part attributed to it.
// all, a listing that includes hidden sessions, adds a HIDDEN column.
func printSessions(w io.Writer, sessions []server.SessionSummary, repo string, all bool) {
	if len(sessions) == 0 {
		fmt.Fprintln(
			w,
			"no sessions recorded yet; the hooks feed moondb record, serve drains the spool",
		)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	costs, hidden := "", ""
	if repo != "" {
		costs = "COST\tATTRIBUTED\t"
	}
	if all {
		hidden = "HIDDEN\t"
	}
	fmt.Fprintln(tw, "STARTED\tSESSION\tTURNS\tEDITS\tACTIVE\tWALL\t"+costs+hidden+"REPOS\tTITLE")
	for _, s := range sessions {
		if repo != "" {
			costs = rowCost(s.Cost, s.Unpriced) + "\t" +
				rowCost(s.RepoCost, partUnpriced(s, repo)) + "\t"
		}
		if all {
			hidden = hiddenCell(s.Hidden) + "\t"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s%s%s\t%s\n",
			when(s.StartedAt), short(s.ID), fmtCount(s.TurnCount), fmtCount(s.EditCount),
			trace.FormatDuration(s.ActiveMS), trace.FormatDuration(s.DurationMS), costs, hidden,
			orDash(strings.Join(s.Repos, ",")), orDash(s.Title))
	}
	_ = tw.Flush()
}

// hiddenCell renders the HIDDEN column: "yes" for a hidden session, else
// a dash.
func hiddenCell(hidden bool) string {
	if hidden {
		return "yes"
	}
	return "-"
}

// rowCost renders a listing cost: "-" when there is none and no unpriced
// model to name, else trace.FormatCost, "unknown" for a cost no price
// covers.
func rowCost(usd *float64, unpriced []string) string {
	if usd == nil && len(unpriced) == 0 {
		return "-"
	}
	return trace.FormatCost(usd)
}

// partUnpriced names the models of s's attributed part for repo that no
// price covers, nil when the rule sends repo nothing.
func partUnpriced(s server.SessionSummary, repo string) []string {
	for _, c := range s.RepoCosts {
		if c.Repo == repo {
			return c.Unpriced
		}
	}
	return nil
}

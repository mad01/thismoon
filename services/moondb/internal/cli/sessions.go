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
)

var sessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "List recorded sessions, newest first",
	Long: `List the sessions moondb has recorded, newest first, with how many turns
and edits each made, how long it ran from the first prompt to the last Stop,
and which repositories it touched. moondb trace <session-id> shows where
that time went and what it cost.

--since takes an RFC 3339 time or a span before now: 90m, 24h, 7d.`,
	Args: cobra.NoArgs,
	RunE: runSessions,
}

func init() {
	sessionsCmd.Flags().
		StringVar(&sessionsRepo, "repo", "", "keep sessions that edited this repository")
	sessionsCmd.Flags().
		StringVar(&sessionsSince, "since", "", "keep sessions started after this time or span")
	sessionsCmd.Flags().
		IntVar(&sessionsLimit, "limit", 0, "most sessions to list (default: the server's)")
	sessionsCmd.Flags().BoolVar(&sessionsJSON, "json", false, "print the API answer as JSON")
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
	q := client.SessionQuery{Repo: sessionsRepo, Since: sessionsSince, Limit: sessionsLimit}
	sessions, err := client.New(apiURL()).Sessions(ctx, q)
	if err != nil {
		return err
	}
	if sessionsJSON {
		return printJSON(cmd.OutOrStdout(), sessions)
	}
	printSessions(cmd.OutOrStdout(), sessions)
	return nil
}

// printSessions renders session summaries as a table.
func printSessions(w io.Writer, sessions []server.SessionSummary) {
	if len(sessions) == 0 {
		fmt.Fprintln(
			w,
			"no sessions recorded yet; the hooks feed moondb record, serve drains the spool",
		)
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "STARTED\tSESSION\tTURNS\tEDITS\tDURATION\tREPOS\tTITLE")
	for _, s := range sessions {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			when(s.StartedAt), short(s.ID), fmtCount(s.TurnCount), fmtCount(s.EditCount),
			trace.FormatDuration(s.DurationMS),
			orDash(strings.Join(s.Repos, ",")), orDash(sessionLabel(s.Title, s.Cwd)))
	}
	_ = tw.Flush()
}

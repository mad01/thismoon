package cli

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/prs/internal/client"
)

var (
	listRepo   string
	listAuthor string
	listReview string
	listSort   string
	listJSON   bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List the cached open PRs",
	Long: `List the open PRs from the local cache, newest first. Filters mirror the
web page: --repo (exact org/name), --author (exact login), --review
(APPROVED | CHANGES_REQUESTED), --sort (newest | oldest).`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		res, err := serveClient().List(client.ListFilter{
			Repo:   listRepo,
			Author: listAuthor,
			Review: listReview,
			Sort:   listSort,
		})
		if err != nil {
			return err
		}
		if listJSON {
			return printJSON(cmd, res)
		}
		if len(res.PRs) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "no open PRs match")
			return nil
		}
		for _, pr := range res.PRs {
			age := shortAge(time.Since(pr.CreatedAt))
			review := ""
			switch pr.ReviewDecision {
			case "APPROVED":
				review = "  [approved]"
			case "CHANGES_REQUESTED":
				review = "  [changes requested]"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-40s #%-5d %-6s %-14s %s%s\n",
				pr.Repo, pr.Number, age, pr.Author, pr.Title, review)
			fmt.Fprintf(cmd.OutOrStdout(), "  %s\n", pr.URL)
		}
		for _, e := range res.Status.Errors {
			fmt.Fprintf(cmd.OutOrStdout(), "! %s/%s: %s\n", e.Host, e.Repo, e.Error)
		}
		return nil
	},
}

var refreshJSON bool

var refreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Force one poll cycle now",
	Long: `Force one synchronous poll cycle: re-discover the local repos, fetch open
PRs from every GitHub host, update the cache, and report the outcome.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		sum, err := serveClient().Refresh()
		if err != nil {
			return err
		}
		if refreshJSON {
			return printJSON(cmd, sum)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "polled %d repos: %d open PRs, %d errors (%.1fs)\n",
			sum.Repos, sum.OpenPRs, sum.Errors, float64(sum.DurationMS)/1000)
		return nil
	},
}

var statusJSON bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Report cache freshness, errors, and the config in effect",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		st, err := serveClient().Status()
		if err != nil {
			return err
		}
		if statusJSON {
			return printJSON(cmd, st)
		}
		out := cmd.OutOrStdout()
		polled := "never"
		if !st.PolledAt.IsZero() {
			polled = fmt.Sprintf("%s (%s ago)",
				st.PolledAt.Local().Format("Mon Jan 2 15:04"),
				shortAge(time.Since(st.PolledAt)))
		}
		fmt.Fprintf(out, "last poll:     %s\n", polled)
		fmt.Fprintf(out, "poll interval: %s\n", st.PollInterval)
		fmt.Fprintf(out, "repos:         %d (%d open PRs)\n", st.Repos, st.OpenPRs)
		fmt.Fprintf(out, "hosts:         %v\n", st.Hosts)
		fmt.Fprintf(out, "config:        %s (loaded: %v)\n", st.ConfigPath, st.ConfigLoaded)
		if len(st.Dirs) > 0 {
			fmt.Fprintf(out, "dirs:          %v\n", st.Dirs)
		}
		for _, e := range st.Errors {
			fmt.Fprintf(out, "! %s/%s: %s\n", e.Host, e.Repo, e.Error)
		}
		return nil
	},
}

func init() {
	listCmd.Flags().StringVar(&listRepo, "repo", "", "exact org/name filter")
	listCmd.Flags().StringVar(&listAuthor, "author", "", "exact GitHub login filter")
	listCmd.Flags().
		StringVar(&listReview, "review", "", "review filter: APPROVED | CHANGES_REQUESTED")
	listCmd.Flags().StringVar(&listSort, "sort", "", "sort order: newest (default) | oldest")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "print the raw API response")
	refreshCmd.Flags().BoolVar(&refreshJSON, "json", false, "print the raw API response")
	statusCmd.Flags().BoolVar(&statusJSON, "json", false, "print the raw API response")
	rootCmd.AddCommand(listCmd, refreshCmd, statusCmd)
}

// serveClient builds the HTTP client for the resolved port.
func serveClient() *client.Client {
	return client.New(fmt.Sprintf("http://localhost:%d", flagPort))
}

func printJSON(cmd *cobra.Command, v any) error {
	enc := json.NewEncoder(cmd.OutOrStdout())
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// shortAge renders a duration as the compact 3d/5h/12m/45s form.
func shortAge(d time.Duration) string {
	switch {
	case d >= 24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d >= time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}

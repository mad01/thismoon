package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/deps/internal/api"
	"github.com/mad01/thismoon/services/deps/internal/client"
)

var flagCheckRepo string

// apiClient builds an HTTP client to the local serve instance. It always targets
// localhost:<port> (reachable without d-man); base-url is only for display.
func apiClient() *client.Client {
	return client.New(fmt.Sprintf("http://localhost:%d", flagPort))
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Discover dependencies across all repos (no advisory check)",
	Long: `Trigger 'deps serve' to discover every external dependency across the
registered repos and persist the result. Prints a per-ecosystem count. Run
'deps check' to additionally check them against OSV.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		res, err := apiClient().Scan()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Discovered %d dependencies\n", res.Total)
		for eco, n := range res.ByEcosystem {
			fmt.Fprintf(cmd.OutOrStdout(), "  %-5s %d\n", eco, n)
		}
		return nil
	},
}

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Discover dependencies and check them against OSV; print flagged",
	Long: `Trigger 'deps serve' to discover every dependency and check each version
against the OSV.dev advisory database. Prints any flagged package with its
advisory id, severity, and the version that fixes it.

With --repo, rescan a single repo (by path or basename) and merge the result.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		c := apiClient()
		var res client.CheckResult
		var err error
		if flagCheckRepo != "" {
			res, err = c.CheckRepo(flagCheckRepo)
		} else {
			res, err = c.Check()
		}
		if err != nil {
			return err
		}
		printFlagged(cmd, res.Total, res.Flagged)
		return nil
	},
}

var resolveCmd = &cobra.Command{
	Use:   "resolve <key>...",
	Short: "Acknowledge flagged advisories by key so they stop alerting",
	Long: `Acknowledge one or more flagged advisories by their key (shown by 'deps check').
A resolved advisory drops out of the active findings until the package version
changes or a new advisory appears on it.`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		res, err := apiClient().Resolve(args)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Resolved %d advisory(ies)\n", res.Resolved)
		return nil
	},
}

var notifyCmd = &cobra.Command{
	Use:   "notify",
	Short: "Fire notifications for flagged packages not yet notified",
	Long: `Tell 'deps serve' to fire a macOS notification for each flagged advisory it
has not already notified. Returns how many were delivered.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		res, err := apiClient().Notify()
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Delivered %d notification(s)\n", res.Delivered)
		return nil
	},
}

func printFlagged(cmd *cobra.Command, total int, flagged []api.Dependency) {
	if len(flagged) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No flagged dependencies (%d checked).\n", total)
		return
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%d flagged of %d checked:\n", len(flagged), total)
	for _, d := range flagged {
		for _, a := range d.Advisories {
			line := fmt.Sprintf("  %s %s@%s  %s", d.Ecosystem, d.Name, d.Version, a.ID)
			if a.Severity != "" {
				line += " [" + a.Severity + "]"
			}
			if a.FixedVersion != "" {
				line += " → fix " + a.FixedVersion
			}
			if a.Resolved {
				line += "  (resolved)"
			}
			line += "\n      key: " + a.Key
			fmt.Fprintln(cmd.OutOrStdout(), line)
		}
	}
}

func init() {
	checkCmd.Flags().
		StringVar(&flagCheckRepo, "repo", "", "rescan only this repo (path or basename)")
	rootCmd.AddCommand(scanCmd, checkCmd, notifyCmd, resolveCmd)
}

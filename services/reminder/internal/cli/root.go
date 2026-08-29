package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/reminder"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "reminder",
	Short: "Set, view, and get notified about reminders over localhost",
	Long: fmt.Sprintf(`reminder manages scheduled reminders that fire a macOS notification
at their due time, viewable at http://localhost:%d (reminder.this with d-man).

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) and the firing ticker.
  mcp     Run the MCP stdio server exposing reminder_* tools to Claude Code.`, reminder.DefaultPort),
	// A failed call is a diagnosis ("serve not reachable"), not a usage
	// mistake; main prints the error once and nothing dumps the help text.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("REMINDER_WORKDIR", reminder.DefaultWorkdir),
		"directory holding reminders.json (env REMINDER_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("REMINDER_PORT", reminder.DefaultPort),
		"port the HTTP server listens on / the MCP talks to (env REMINDER_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("REMINDER_BASE_URL", ""),
		"human-facing base URL the MCP links to (env REMINDER_BASE_URL); defaults to http://localhost:<port>")
	// Expand a leading ~ in the workdir before any subcommand runs: REMINDER_WORKDIR
	// reaches Go without shell expansion.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		workdir, err := confdir.Expand(flagWorkdir)
		if err != nil {
			return err
		}
		flagWorkdir = workdir
		return nil
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

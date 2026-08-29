package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/events"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "events",
	Short: "Record, browse, and query a local event/audit log over localhost",
	Long: fmt.Sprintf(`events is a local event/audit log: producers emit events, and you browse
the timeline at http://localhost:%d (events.this with d-man) or query it from
Claude. It is archive-only — events are recorded, never fired.

Subcommands:
  serve   Run the HTTP server (web timeline + JSON API) over the JSONL store.
  emit    Record a single event.
  list    List recent events.
  mcp     Run the MCP stdio server exposing events_* tools to Claude Code.`, events.DefaultPort),
	// A failed call is a diagnosis ("serve not reachable"), not a usage
	// mistake; main prints the error once and nothing dumps the help text.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("EVENTS_WORKDIR", events.DefaultWorkdir),
		"directory holding the sources/ JSONL store (env EVENTS_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("EVENTS_PORT", events.DefaultPort),
		"port the HTTP server listens on / the MCP and CLI talk to (env EVENTS_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("EVENTS_BASE_URL", ""),
		"base URL the MCP links to (env EVENTS_BASE_URL); defaults to http://localhost:<port>")
	// Expand a leading ~ in the workdir before any subcommand runs: EVENTS_WORKDIR
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

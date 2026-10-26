// Package cli is the moondb command tree. serve owns the store; record
// writes the spool the hooks feed; every other command is an HTTP client of
// serve, so the store keeps a single writer.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/services/moondb"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
	flagConfig  string
)

var rootCmd = &cobra.Command{
	Use:   "moondb",
	Short: "Record the edits agent sessions make between commits",
	Long: fmt.Sprintf(`moondb records every file edit a Claude Code session makes, grouped by
session, turn and agent and keyed by repository, worktree and branch, with the
file contents before and after each change. It is served at http://localhost:%d
(moondb.this with d-man). Ask it why a line looks the way it does and it
answers with the turn, the prompt and the commit the change landed in.

The hooks pipe their payloads into record, which writes a spool and returns.
serve is the single writer of the store: it drains the spool, derives the
before and after content, and links edits to commits by blob id. The MCP and
the query commands are thin HTTP clients of serve.

Subcommands:
  serve    Run the HTTP server (web UI + JSON API) that owns the store.
  record   Read one hook payload on stdin and spool it (wired by the hooks).
  import   Import past sessions from their transcripts into the spool.
  mcp      Run the MCP stdio server exposing moondb_* tools to Claude Code.
  why      Explain one line of one file.
  history  List the recorded edits of a path.
  inflight List the files other sessions edited that no commit has taken yet.
  sessions List recorded sessions.
  trace    Show what took time in a session and what it cost.`, moondb.DefaultPort),
	// A failed query is a diagnosis ("nothing recorded for that path"), not a
	// usage mistake; main prints the error once and nothing dumps the help.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("MOONDB_WORKDIR", moondb.DefaultWorkdir),
		"state directory: log, objects, spool (env MOONDB_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("MOONDB_PORT", moondb.DefaultPort),
		"port the HTTP server listens on / the clients talk to (env MOONDB_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("MOONDB_BASE_URL", ""),
		"human-facing base URL the MCP links to (env MOONDB_BASE_URL); "+
			"defaults to http://localhost:<port>")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config",
		envdefault.String("MOONDB_CONFIG", defaultConfigPath()),
		"config file (env MOONDB_CONFIG)")
	rootCmd.PersistentPreRunE = preRun
}

// preRun resolves the settings every command shares: the config file is
// read and lets its keys stand in for defaults, then a leading ~ in the
// workdir is expanded (MOONDB_WORKDIR reaches Go without shell expansion).
// A broken config is kept, not returned: record must still exit 0, so each
// command applies requireConfig itself.
func preRun(cmd *cobra.Command, _ []string) error {
	cfg, cfgErr = loadConfig(flagConfig)
	if cfgErr == nil {
		applyConfig(cmd.Flags(), cfg)
	}
	workdir, err := confdir.Expand(flagWorkdir)
	if err != nil {
		return err
	}
	flagWorkdir = workdir
	return nil
}

// apiURL is where the clients reach serve: always loopback on the resolved
// port. --base-url is for display only (ADR-0011).
func apiURL() string {
	return fmt.Sprintf("http://localhost:%d", flagPort)
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

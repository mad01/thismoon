package cli

import (
	"fmt"
	"os/user"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	wire "github.com/mad01/thismoon/services/wire"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
	flagFrom    string
)

var rootCmd = &cobra.Command{
	Use:   "wire",
	Short: "Talk between sessions over named channels",
	Long: fmt.Sprintf(`wire is a message bus for agent sessions, served at
http://localhost:%d (wire.this with d-man). One session opens a channel and
hands the name to another session — a second agent, a parallel run, a human at
a terminal — and from then on both post to it and read from it. A read can
block until the other side answers, so waiting for a reply costs one call
instead of a polling loop.

serve owns the store and is where a blocking read parks; the MCP and the CLI
are thin HTTP clients to it, so serve stays the single writer.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) that owns the store.
  mcp     Run the MCP stdio server exposing wire_* tools to Claude Code.`, wire.DefaultPort),
	// A failed call is a runtime answer ("channel is closed"), not a usage
	// mistake; main prints the error once and nothing dumps the help text.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("WIRE_WORKDIR", wire.DefaultWorkdir),
		"directory holding the channel and message logs (env WIRE_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("WIRE_PORT", wire.DefaultPort),
		"port the HTTP server listens on / the MCP and CLI talk to (env WIRE_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("WIRE_BASE_URL", ""),
		"base URL the MCP links to (env WIRE_BASE_URL); defaults to http://localhost:<port>")
	rootCmd.PersistentFlags().StringVar(&flagFrom, "from", defaultFrom(),
		"who your messages are signed as (env WIRE_FROM)")
	// Expand a leading ~ in the workdir before any subcommand runs:
	// WIRE_WORKDIR reaches Go without shell expansion.
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

// defaultFrom is the name a CLI message is signed with when the user does not
// pass one: WIRE_FROM, else the OS username. A message with no sender cannot
// be answered, so there is always a fallback.
func defaultFrom() string {
	name := "cli"
	if u, err := user.Current(); err == nil && u.Username != "" {
		name = u.Username
	}
	return envdefault.String("WIRE_FROM", name)
}

package cli

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const defaultPort = 7432

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
	flagFrom    string
)

var rootCmd = &cobra.Command{
	Use:   "wire",
	Short: "Talk between sessions over named channels",
	Long: `wire is a message bus for agent sessions, viewable at http://wire.this/.
One session opens a channel and hands the name to another session — a second
agent, a parallel run, a human at a terminal — and from then on both post to it
and read from it. A read can block until the other side answers, so waiting for
a reply costs one call instead of a polling loop.

serve owns the store and is where a blocking read parks; the MCP and the CLI
are thin HTTP clients to it, so serve stays the single writer.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) that owns the store.
  mcp     Run the MCP stdio server exposing wire_* tools to Claude Code.`,
	// A failed call is a runtime answer ("channel is closed"), not a usage
	// mistake; main prints the error once and nothing dumps the help text.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding the channel and message logs (env WIRE_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on / the MCP and CLI talk to (env WIRE_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", os.Getenv("WIRE_BASE_URL"),
		"base URL the MCP links to (env WIRE_BASE_URL); defaults to http://localhost:<port>")
	rootCmd.PersistentFlags().StringVar(&flagFrom, "from", defaultFrom(),
		"who your messages are signed as (env WIRE_FROM)")
	// Expand a leading ~ in the workdir before any subcommand runs:
	// WIRE_WORKDIR reaches Go without shell expansion.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		flagWorkdir = expandTilde(flagWorkdir)
		return nil
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// defaultWorkdir resolves the data directory, honoring WIRE_WORKDIR and
// falling back to ~/.local/share/wire.
func defaultWorkdir() string {
	if v := os.Getenv("WIRE_WORKDIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".wire"
	}
	return filepath.Join(home, ".local", "share", "wire")
}

// defaultFrom is the name a CLI message is signed with when the user does not
// pass one: WIRE_FROM, else the OS username. A message with no sender cannot
// be answered, so there is always a fallback.
func defaultFrom() string {
	if v := os.Getenv("WIRE_FROM"); v != "" {
		return v
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "cli"
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory.
func expandTilde(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func resolvedDefaultPort() int {
	if v := os.Getenv("WIRE_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultPort
}

package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const defaultPort = 7430

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "events",
	Short: "Record, browse, and query a local event/audit log over localhost",
	Long: `events is a local event/audit log: producers emit events, and you browse the
timeline at http://events.this/ or query it from Claude. It is archive-only —
events are recorded, never fired.

Subcommands:
  serve   Run the HTTP server (web timeline + JSON API) over the JSONL store.
  emit    Record a single event.
  list    List recent events.
  mcp     Run the MCP stdio server exposing events_* tools to Claude Code.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding the sources/ JSONL store (env EVENTS_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on / the MCP and CLI talk to (env EVENTS_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", os.Getenv("EVENTS_BASE_URL"),
		"base URL the MCP links to (env EVENTS_BASE_URL); defaults to http://localhost:<port>")
	// Expand a leading ~ in the workdir before any subcommand runs: EVENTS_WORKDIR
	// reaches Go without shell expansion.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		flagWorkdir = expandTilde(flagWorkdir)
		return nil
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// defaultWorkdir resolves the data directory, honoring EVENTS_WORKDIR and
// falling back to ~/.local/share/events.
func defaultWorkdir() string {
	if v := os.Getenv("EVENTS_WORKDIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".events"
	}
	return filepath.Join(home, ".local", "share", "events")
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
	if v := os.Getenv("EVENTS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultPort
}

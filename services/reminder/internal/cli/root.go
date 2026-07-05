package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

const defaultPort = 7428

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "reminder",
	Short: "Set, view, and get notified about reminders over localhost",
	Long: `reminder manages scheduled reminders that fire a macOS notification at
their due time, viewable at http://reminder.this/.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) and the firing ticker.
  mcp     Run the MCP stdio server exposing reminder_* tools to Claude Code.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding reminders.json (env REMINDER_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on / the MCP talks to (env REMINDER_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", os.Getenv("REMINDER_BASE_URL"),
		"base URL the MCP calls and links to (env REMINDER_BASE_URL); defaults to http://localhost:<port>")
	// Expand a leading ~ in the workdir before any subcommand runs: REMINDER_WORKDIR
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

// defaultWorkdir resolves the data directory, honoring REMINDER_WORKDIR and
// falling back to ~/.local/share/reminder.
func defaultWorkdir() string {
	if v := os.Getenv("REMINDER_WORKDIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".reminder"
	}
	return filepath.Join(home, ".local", "share", "reminder")
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
	if v := os.Getenv("REMINDER_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultPort
}

package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

const defaultPort = 7431

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "kof",
	Short: "Store assertions about how systems behave, pinned to evidence",
	Long: `kof (keeper-of-facts) is a local assertion store with evidence pins,
viewable at http://kof.this/. An agent session records a one-sentence assertion
and pins it to a line range in a repo working tree, hashed the moment it is
recorded; kof check re-hashes those pins and flips an assertion stale when the
code changed.

serve owns the store and resolves pins; the MCP and CLI mutations are thin HTTP
clients to it, so serve stays the single writer.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) that owns the store.
  mcp     Run the MCP stdio server exposing kof_* tools to Claude Code.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding the assertion log (env KOF_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on / the MCP talks to (env KOF_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", envFirst("KOF_BASE_URL", "KEEP_BASE_URL"),
		"base URL the MCP calls and links to (env KOF_BASE_URL); defaults to http://localhost:<port>")
	// Expand a leading ~ in the workdir before any subcommand runs: KOF_WORKDIR
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

// envFirst returns the first set variable among names. The KEEP_* names are
// the tool's pre-rename spelling, honored for one release so a half-updated
// machine keeps working during the keep → keeper-of-facts cutover (MAD-266).
func envFirst(names ...string) string {
	for _, name := range names {
		if v := os.Getenv(name); v != "" {
			return v
		}
	}
	return ""
}

// defaultWorkdir resolves the data directory, honoring KOF_WORKDIR (then the
// pre-rename KEEP_WORKDIR) and falling back to ~/.local/share/keep. The
// on-disk path deliberately stays the old one: the store migration is its own
// cutover step (MAD-269), so a freshly renamed binary reads the existing data.
func defaultWorkdir() string {
	if v := envFirst("KOF_WORKDIR", "KEEP_WORKDIR"); v != "" {
		return v
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".keep"
	}
	return filepath.Join(home, ".local", "share", "keep")
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
	if v := envFirst("KOF_PORT", "KEEP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultPort
}

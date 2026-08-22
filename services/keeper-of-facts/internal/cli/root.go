package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	kof "github.com/mad01/thismoon/services/keeper-of-facts"
)

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
		"human-facing base URL the MCP links to (env KOF_BASE_URL); defaults to http://localhost:<port>")
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
// pre-rename KEEP_WORKDIR) and falling back to kof.DefaultWorkdir — the same
// constant the operating doc renders, so the two cannot drift. The leading ~
// expands in PersistentPreRunE. serve auto-migrates a pre-rename
// ~/.local/share/keep store to the new path on start (MAD-269).
func defaultWorkdir() string {
	if v := envFirst("KOF_WORKDIR", "KEEP_WORKDIR"); v != "" {
		return v
	}
	return kof.DefaultWorkdir
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory. When
// the home directory cannot be resolved, the ~ prefix is stripped so the path
// degrades to cwd-relative instead of creating a literal "~" directory.
func expandTilde(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		if path == "~" {
			return "."
		}
		return path[2:]
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
	return kof.DefaultPort
}

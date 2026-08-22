package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	present "github.com/mad01/thismoon/services/present"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
)

var rootCmd = &cobra.Command{
	Use:   "present",
	Short: "Serve and manage scrollable briefing pages over localhost",
	Long: `present manages single-page HTML briefing pages (create / read / update /
list; delete is available only in the web index). Pages are authored as
structured JSON and compiled to an HTML fragment at authoring time; the server
serves that fragment and the browser assembles the full page client-side. The
chrome (header, theme, controls) comes from the shared webkit package.

Subcommands:
  serve   Run the local HTTP server that serves pages.
  mcp     Run the MCP stdio server exposing present_* tools to Claude Code.`,
	// Let main print the error once; cobra stays quiet on both usage and errors.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding pages/ (env PRESENT_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on / URLs point at (env PRESENT_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", os.Getenv("PRESENT_BASE_URL"),
		"base URL for page links (env PRESENT_BASE_URL); defaults to http://localhost:<port>")
	// Expand a leading ~ in the workdir before any subcommand runs. A literal
	// "~/..." reaches Go from PRESENT_WORKDIR or --workdir without shell
	// expansion; left unexpanded the MCP and serve processes resolve different
	// directories and updates land where nothing serves them.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		flagWorkdir = expandTilde(flagWorkdir)
		return nil
	}
}

// Execute runs the root command and returns any error for main to print.
func Execute() error {
	return rootCmd.Execute()
}

// defaultWorkdir resolves the pages directory, honoring PRESENT_WORKDIR and
// falling back to present.DefaultWorkdir — the same constant the operating
// doc renders, so the two cannot drift. The leading ~ expands in
// PersistentPreRunE.
func defaultWorkdir() string {
	if v := os.Getenv("PRESENT_WORKDIR"); v != "" {
		return v
	}
	return present.DefaultWorkdir
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory. Other
// paths (absolute or already-expanded) are returned unchanged. When the home
// directory cannot be resolved, the ~ prefix is stripped so the path degrades
// to cwd-relative instead of creating a literal "~" directory.
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
	if v := os.Getenv("PRESENT_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return present.DefaultPort
}

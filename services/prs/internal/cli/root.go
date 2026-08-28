package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	prs "github.com/mad01/thismoon/services/prs"
)

var (
	flagWorkdir string
	flagPort    int
	flagBaseURL string
	flagConfig  string
)

var rootCmd = &cobra.Command{
	Use:   "prs",
	Short: "See every open PR across your locally checked-out repos",
	Long: `prs is a local open-PR dashboard, viewable at http://prs.this/. It discovers
every git repo under the configured directories (the same discovery csl-style
tools use), polls each repo's GitHub host for open pull requests — github.com
and GitHub Enterprise alike, authenticated through your existing gh logins —
and answers "is there anything I need to act on?" in one place.

serve owns the cache and the polling; the MCP and CLI commands are thin HTTP
clients to it, so serve stays the single writer.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) that owns the cache and polls.
  mcp     Run the MCP stdio server exposing prs_* tools to Claude Code.`,
	// An error from a subcommand is a diagnosis, not a usage mistake; main
	// prints it once.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding the PR cache (env PRS_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on / the MCP talks to (env PRS_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", os.Getenv("PRS_BASE_URL"),
		"human-facing base URL the MCP links to (env PRS_BASE_URL); defaults to http://localhost:<port>")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", defaultConfigPath(),
		"path to the YAML config (env PRS_CONFIG)")
	// Expand a leading ~ before any subcommand runs: PRS_WORKDIR and
	// PRS_CONFIG reach Go without shell expansion under launchd.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		flagWorkdir = expandTilde(flagWorkdir)
		flagConfig = expandTilde(flagConfig)
		return nil
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// defaultWorkdir resolves the cache directory, honoring PRS_WORKDIR and
// falling back to prs.DefaultWorkdir — the same constant the operating doc
// renders, so the two cannot drift. The leading ~ expands in
// PersistentPreRunE.
func defaultWorkdir() string {
	if v := os.Getenv("PRS_WORKDIR"); v != "" {
		return v
	}
	return prs.DefaultWorkdir
}

// defaultConfigPath resolves the config path, honoring PRS_CONFIG and
// falling back to prs.DefaultConfigPath.
func defaultConfigPath() string {
	if v := os.Getenv("PRS_CONFIG"); v != "" {
		return v
	}
	return prs.DefaultConfigPath
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory. When
// the home directory cannot be resolved, the ~ prefix is stripped so the
// path degrades to cwd-relative instead of creating a literal "~" directory.
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
	if v := os.Getenv("PRS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return prs.DefaultPort
}

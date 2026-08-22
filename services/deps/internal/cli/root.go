package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	deps "github.com/mad01/thismoon/services/deps"
	"github.com/mad01/thismoon/services/deps/internal/config"
	"github.com/mad01/thismoon/services/deps/internal/registry"
)

var (
	flagWorkdir  string
	flagPort     int
	flagBaseURL  string
	flagRegistry string
	flagConfig   string
)

var rootCmd = &cobra.Command{
	Use:   "deps",
	Short: "Scan tool repos for external dependencies and supply-chain advisories",
	Long: `deps discovers every external dependency across the mad01 tool repos (Go,
npm, …), checks each version against the OSV.dev advisory database, and fires a
macOS notification when one is flagged. Findings are viewable at http://deps.this/.

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) and the periodic scan loop.
  scan    Discover dependencies across all repos (no advisory check).
  check   Discover + check against OSV; print flagged packages.
  notify  Fire notifications for any flagged packages not yet notified.
  mcp     Run the MCP stdio server exposing deps_* tools to Claude Code.

scan/check/notify and the MCP are thin clients to a running 'deps serve' — start
that agent first (it owns the scan store).`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir", defaultWorkdir(),
		"directory holding scan.json (env DEPS_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", resolvedDefaultPort(),
		"port the HTTP server listens on / the client talks to (env DEPS_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url", os.Getenv("DEPS_BASE_URL"),
		"base URL the client links to (env DEPS_BASE_URL); defaults to http://localhost:<port>")
	rootCmd.PersistentFlags().StringVar(&flagRegistry, "registry", defaultRegistry(),
		"catalog registry.yaml listing repos to scan (env DEPS_REGISTRY)")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", defaultConfig(),
		"discovery config: exclude_repos / exclude_paths (env DEPS_CONFIG)")
	// Expand a leading ~ before any subcommand runs: env vars reach Go without
	// shell expansion (launchd doesn't run through a shell).
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		flagWorkdir = expandTilde(flagWorkdir)
		flagRegistry = expandTilde(flagRegistry)
		flagConfig = expandTilde(flagConfig)
		return nil
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// defaultWorkdir resolves the data directory, honoring DEPS_WORKDIR and falling
// back to deps.DefaultWorkdir — the same constant the operating doc renders.
// The leading ~ is expanded by PersistentPreRunE before any subcommand runs.
func defaultWorkdir() string {
	if v := os.Getenv("DEPS_WORKDIR"); v != "" {
		return v
	}
	return deps.DefaultWorkdir
}

// defaultRegistry resolves the catalog registry path, honoring DEPS_REGISTRY.
func defaultRegistry() string {
	if v := os.Getenv("DEPS_REGISTRY"); v != "" {
		return v
	}
	return registry.DefaultPath
}

// defaultConfig resolves the discovery config path, honoring DEPS_CONFIG.
func defaultConfig() string {
	if v := os.Getenv("DEPS_CONFIG"); v != "" {
		return v
	}
	return config.DefaultPath
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
	if v := os.Getenv("DEPS_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return deps.DefaultPort
}

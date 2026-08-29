package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
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
	Long: fmt.Sprintf(`deps discovers every external dependency across the mad01 tool repos (Go,
npm, …), checks each version against the OSV.dev advisory database, and fires a
macOS notification when one is flagged. Findings are viewable at
http://localhost:%d (deps.this with d-man).

Subcommands:
  serve   Run the HTTP server (web UI + JSON API) and the periodic scan loop.
  scan    Discover dependencies across all repos (no advisory check).
  check   Discover + check against OSV; print flagged packages.
  notify  Fire notifications for any flagged packages not yet notified.
  mcp     Run the MCP stdio server exposing deps_* tools to Claude Code.

scan/check/notify and the MCP are thin clients to a running 'deps serve' — start
that agent first (it owns the scan store).`, deps.DefaultPort),
	// An error from a subcommand is a diagnosis, not a usage mistake; main
	// prints it once.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagWorkdir, "workdir",
		envdefault.String("DEPS_WORKDIR", deps.DefaultWorkdir),
		"directory holding scan.json (env DEPS_WORKDIR)")
	rootCmd.PersistentFlags().IntVar(&flagPort, "port",
		envdefault.Int("DEPS_PORT", deps.DefaultPort),
		"port the HTTP server listens on / the client talks to (env DEPS_PORT)")
	rootCmd.PersistentFlags().StringVar(&flagBaseURL, "base-url",
		envdefault.String("DEPS_BASE_URL", ""),
		"display-only base URL the MCP puts in tool responses (env DEPS_BASE_URL); "+
			"empty means http://localhost:<port>, and it never changes where requests go")
	rootCmd.PersistentFlags().StringVar(&flagRegistry, "registry",
		envdefault.String("DEPS_REGISTRY", defaultRegistryPath()),
		"catalog registry.yaml listing repos to scan (env DEPS_REGISTRY)")
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config",
		envdefault.String("DEPS_CONFIG", defaultConfigPath()),
		"discovery config: exclude_repos / exclude_paths (env DEPS_CONFIG)")
	// Expand a leading ~ before any subcommand runs: env vars reach Go without
	// shell expansion (launchd doesn't run through a shell).
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		var err error
		if flagWorkdir, err = confdir.Expand(flagWorkdir); err != nil {
			return err
		}
		if flagRegistry, err = confdir.Expand(flagRegistry); err != nil {
			return err
		}
		flagConfig, err = confdir.Expand(flagConfig)
		return err
	}
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}

// defaultConfigPath resolves the discovery config inside deps's own config
// directory, honoring XDG_CONFIG_HOME. A home directory that cannot be
// resolved leaves the ~-prefixed constant in place, so the failure surfaces in
// PersistentPreRunE — which can return an error — rather than here.
func defaultConfigPath() string {
	if path, err := confdir.Path("deps", config.FileName); err == nil {
		return path
	}
	return config.DefaultPath
}

// defaultRegistryPath resolves the catalog registry inside catalog's config
// directory. deps reads the file catalog owns, so the two have to agree about
// where XDG_CONFIG_HOME puts it.
func defaultRegistryPath() string {
	if path, err := confdir.Path("catalog", registry.FileName); err == nil {
		return path
	}
	return registry.DefaultPath
}

package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Version is injected at build time via -ldflags.
var Version = "dev"

var (
	flagConfig    string
	flagHostsFile string
)

// defaultHostsFile is the system hosts file; overridable for tests/dry-runs.
const defaultHostsFile = "/etc/hosts"

var rootCmd = &cobra.Command{
	Use:   "d-man",
	Short: "Local domain front door: *.this names -> localhost services",
	// Errors are printed once by main as "d-man: <err>"; cobra stays quiet so it
	// neither double-prints the error nor dumps usage on a runtime failure.
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `d-man maps friendly hostnames (csl.this, present.this) to local services.

It does two things, driven by a single routes config:
  1. syncs a managed block into /etc/hosts so <name>.<suffix> resolves to
     127.0.0.1 in every client (Safari, Chrome, curl) — this works on macOS 26
     where /etc/resolver custom-TLD DNS is broken;
  2. runs a reverse proxy on 127.0.0.1:80 that routes by Host header to each
     service's real port.

Subcommands:
  serve   Run the proxy + keep /etc/hosts in sync (the long-running daemon).
  sync    Write the managed /etc/hosts block once and exit.
  list    Print the configured host -> backend routes.`,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagConfig, "config", defaultConfig(),
		"path to routes.toml (env DMAN_CONFIG; ~/.config/d-man/routes.toml, then /etc/d-man/routes.toml)")
	rootCmd.PersistentFlags().StringVar(&flagHostsFile, "hosts-file", defaultHostsFile,
		"hosts file to sync the managed block into")
	// Expand a leading ~ before any subcommand runs. A literal "~/..." reaches
	// Go from DMAN_CONFIG or --config without shell expansion (e.g. when the
	// daemon plist passes $HOME/.config/...); left unexpanded the file open
	// fails.
	rootCmd.PersistentPreRunE = func(_ *cobra.Command, _ []string) error {
		flagConfig = expandTilde(flagConfig)
		return nil
	}
}

// Execute runs the root command tree and returns any error to main.
func Execute() error {
	return rootCmd.Execute()
}

// systemConfig is the root-context routes file. A root launchd daemon (e.g.
// `sudo brew services start d-man`) has no useful HOME, so the per-user
// default resolves to root's home where no routes live; this system path is
// the fallback that makes a bare `d-man serve` work there.
const systemConfig = "/etc/d-man/routes.toml"

// defaultConfig resolves the routes file, honoring DMAN_CONFIG and falling
// back to the first existing path.
func defaultConfig() string {
	return resolveConfig(os.Getenv("DMAN_CONFIG"), fileExists)
}

// resolveConfig picks the routes file: DMAN_CONFIG when set, then the
// per-user ~/.config path when the file exists, then the system /etc path
// when that file exists, else the per-user path so error messages name the
// place most users should create it.
func resolveConfig(env string, exists func(string) bool) string {
	if env != "" {
		return env
	}
	userPath := "routes.toml"
	if home, err := os.UserHomeDir(); err == nil {
		userPath = filepath.Join(home, ".config", "d-man", "routes.toml")
	}
	if exists(userPath) {
		return userPath
	}
	if exists(systemConfig) {
		return systemConfig
	}
	return userPath
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// expandTilde rewrites a leading ~ or ~/ to the user's home directory. Other
// paths (absolute or already-expanded) are returned unchanged.
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

package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/confdir"
	dman "github.com/mad01/thismoon/services/d-man"
	"github.com/mad01/thismoon/services/d-man/internal/config"
)

var (
	flagConfig    string
	flagHostsFile string
)

// defaultHostsFile is the system hosts file; overridable for tests/dry-runs.
const defaultHostsFile = "/etc/hosts"

// routesFile is the routes file's name inside the config directory.
const routesFile = "routes.toml"

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
		expanded, err := confdir.Expand(flagConfig)
		if err != nil {
			return err
		}
		flagConfig = expanded
		return nil
	}
}

// Execute runs the root command tree and returns any error to main.
func Execute() error {
	return rootCmd.Execute()
}

// defaultConfig resolves the routes file for the --config default, honoring
// DMAN_CONFIG. A home directory that cannot be resolved leaves only the
// system path — never a cwd-relative one, which would read a routes file
// nobody wrote. The root launchd daemon reaches the same system path the
// ordinary way, by having no routes file under root's home.
func defaultConfig() string {
	path, err := resolveConfig(os.Getenv("DMAN_CONFIG"), fileExists)
	if err != nil {
		return dman.SystemRoutesPath
	}
	return path
}

// resolveConfig picks the routes file: DMAN_CONFIG when set, then the
// per-user config path when the file exists, then the system /etc path when
// that file exists, else the per-user path so error messages name the place
// most users should create it. The per-user directory comes from kit/confdir,
// so XDG_CONFIG_HOME relocates it and the plain ~/.config default is
// unchanged.
func resolveConfig(env string, exists func(string) bool) (string, error) {
	if env != "" {
		return env, nil
	}
	userPath, err := confdir.Path(dman.Component, routesFile)
	if err != nil {
		return "", fmt.Errorf("resolve routes file: %w", err)
	}
	if exists(userPath) {
		return userPath, nil
	}
	if exists(dman.SystemRoutesPath) {
		return dman.SystemRoutesPath, nil
	}
	return userPath, nil
}

// loadRoutes loads and validates the routes file every subcommand works from,
// tagging failures with the pointer to the embedded operating doc.
func loadRoutes() (*config.Config, error) {
	cfg, err := config.Load(flagConfig)
	if err != nil {
		return nil, agentdoc.Hint(err, dman.Facts())
	}
	return cfg, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/d-man/internal/config"
)

// configReference documents every routes.toml key, shipped in the binary so a
// host that is not resolving can be debugged from the terminal.
const configReference = `# routes.toml — every key is optional; a file with no routes is valid.

suffix = "this"           # label appended to every route name: name "csl" plus
                          # suffix "this" gives csl.this. Defaults to "this".

games_dir = "~/my-games"  # optional directory of extra block-page games (*.js).
                          # Unset serves only the games embedded in the binary.

# Hosts served the local block page instead of being proxied. Each is written to
# /etc/hosts pointing at 127.0.0.1, compared lowercased with any trailing dot
# stripped, and must not also be a route host.
#
# ORDERING: blocklist, suffix and games_dir must all come BEFORE the first
# [[route]] table. TOML reads every key after a [[route]] header as a field of
# that table, so a blocklist at the bottom of the file blocks nothing.
blocklist = [
  "news.ycombinator.com",
  "reddit.com",
]

# One [[route]] per name. A route is either port-backed (port) or a CNAME alias
# (cname); setting both is an error, and so is setting neither.
[[route]]
  name = "csl"            # short label, becomes <name>.<suffix>; dots allowed
  port = 7420             # backend port on target, 1-65535
  target = "127.0.0.1"    # backend host, defaults to 127.0.0.1

[[route]]
  name = "search"         # alias: search.this serves whatever csl.this serves
  cname = "csl"           # another route's name; chains resolve, cycles are rejected
`

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current effective config",
	Long: `Print which routes file d-man loaded and the effective configuration it
produced — the file's own values with the built-in defaults applied.

The routes file is resolved in this order, first match winning:

  --config <path>
  $DMAN_CONFIG
  ~/.config/d-man/routes.toml   (when it exists)
  /etc/d-man/routes.toml        (when it exists, for the root launchd daemon)

With none of them present the per-user path is reported as missing and d-man
runs on defaults alone. A leading ~ is expanded before the file is opened.

Annotated example — every key d-man reads:

` + configReference + `
Pair it with doctor: doctor shows the state d-man resolved, config shows which
file and key to change.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return writeEffectiveConfig(cmd.OutOrStdout(), flagConfig)
	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}

// writeEffectiveConfig writes the resolution header for path followed by the
// effective config as TOML.
func writeEffectiveConfig(w io.Writer, path string) error {
	cfg, status := effectiveConfig(path)
	fmt.Fprintf(w, "config file: %s (%s)\n\n", path, status)
	if cfg.Blocklist == nil {
		// The encoder drops a nil list entirely; "blocklist = []" says the same
		// thing and names the key for anyone about to add one.
		cfg.Blocklist = []string{}
	}
	if err := toml.NewEncoder(w).Encode(cfg); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return nil
}

// effectiveConfig loads the routes file at path and reports how it resolved. A
// missing or unusable file is not an error here: the caller still gets the
// defaults d-man would fall back to, and the status says why.
func effectiveConfig(path string) (*config.Config, string) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return config.Default(), "missing, defaults in use"
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Default(), "parse error: " + err.Error()
	}
	return cfg, "loaded"
}

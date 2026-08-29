package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/deps/internal/config"
)

// configReference documents every discovery key, shipped in the binary so a
// repo that is scanned (or skipped) unexpectedly can be debugged from the
// terminal.
const configReference = `# deps discovery config — both keys optional, both lists of glob patterns.
# A pattern is matched a path segment at a time against every trailing run of
# segments: "thismoon", then "mad01/thismoon", then
# "github.com/mad01/thismoon", up to the absolute path. So "archive-old" names
# a repo by basename, "mad01/*" by org/repo, and "*/archive/*" by any deeper
# suffix — without hardcoding where this machine keeps its checkouts. "*" never
# crosses a "/"; "**" does. A pattern that does not compile is an error naming
# it, not a silently disabled exclusion.
#
# This file only TRIMS the repo set; it never adds to it. The set itself comes
# from the catalog registry (~/.config/catalog/registry.yaml), so a newly
# catalogued repo enrolls on its own. Edits take effect on the next scan — no
# restart of 'deps serve'.

# Skip a whole repo: by basename, by org/repo, or by a deeper path suffix.
exclude_repos = ["scratch-repo", "archive-*", "mad01/*", "*/archive/*"]

# Skip a directory while walking a repo, matched against the repo-relative path
# the same way, so "third_party" skips that directory at any depth. Generated
# trees and vendored examples belong here.
exclude_paths = ["third_party", "internal/gen/*"]

# Git worktrees, nested checkouts, and .git/vendor/node_modules/testdata are
# skipped by the walker regardless of what this file says.
`

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current effective config",
	Long: `Print which discovery config deps loaded and the effective configuration it
produced — the file's own values, or empty lists when there is no file.

The config is resolved in this order, first match winning:

  --config <path>
  $DEPS_CONFIG
  $XDG_CONFIG_HOME/deps/config.toml   (when that variable holds an absolute path)
  ~/.config/deps/config.toml          (where the recipe symlinks it)

A missing file is not an error: discovery then runs with no extra exclusions.
A file that is present but malformed — bad TOML, or a pattern that does not
compile — fails the scan naming the key and the pattern, rather than skipping
the exclusion in silence. A leading ~ is expanded before the file is opened.

Annotated example — every key deps reads:

` + configReference + `
Pair it with doctor: doctor shows the state deps resolved, config shows which
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
	if err := toml.NewEncoder(w).Encode(withEmptyLists(cfg)); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return nil
}

// withEmptyLists replaces nil lists with empty ones. The TOML encoder omits a
// nil list entirely, which would print an empty document for the common case of
// no exclusions; "exclude_repos = []" says the same thing and names the key.
func withEmptyLists(c config.Config) config.Config {
	if c.ExcludeRepos == nil {
		c.ExcludeRepos = []string{}
	}
	if c.ExcludePaths == nil {
		c.ExcludePaths = []string{}
	}
	return c
}

// effectiveConfig loads the discovery config at path and reports how it
// resolved. A missing or unparseable file is not an error here: the caller
// still gets the empty config deps scans with, and the status says why.
func effectiveConfig(path string) (config.Config, string) {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return config.Config{}, "missing, defaults in use"
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Config{}, "parse error: " + err.Error()
	}
	return cfg, "loaded"
}

package commands

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
)

// configReference documents every setting suspenders reads, shipped in the
// binary so a blocked commit can be debugged from the terminal: doctor shows
// what the guard decided, `config --help` shows which key changes it.
const configReference = `# Global config — every key optional.

dirs:                    # directories hook install --all discovers repos in
  - ~/code/src
exclude:                 # repo name globs (org/repo). An excluded repo is
  - you/dotfiles         # exempt from the guard RUNNING in it, but its name
                         # still contributes blocked names for other repos —
                         # that is deliberate.

scan:
  enabled: true          # built-in secret scanner (default on)
  exclude_rules: []      # rule IDs to suppress globally

guard:                   # internal-reference guard — the blocked-name source,
  enabled: false         # shared with belt's write-internal-names
  workspace_dirs:        # dirs scanned for repos; every discovered org/repo
    - ~/workspace        # name and dir basename becomes a blocked name.
                         # Repos inside these dirs are guard-exempt themselves.
  blocked_words:         # always-blocked terms; literal match, * matches a
    - internalname       # run of non-space characters (*.host.net)
  allowlist:             # safe references: names that must NOT be blocked
    - grpc/grpc-go       # even though discovery would collect them
  file_patterns: []      # file globs the staged-diff check inspects

watch: []                # custom secret-detection rules (id/pattern/severity)
allowlist: []            # exact-match secret VALUES that are known-safe
                         # (different thing from guard.allowlist above)

history:
  replace_table: {}      # old -> new strings for history clean
  redact_files: []       # path globs whose blobs are fully redacted

hooks:
  pre_commit: []         # external hook scripts (name/command/file_patterns)
  post_merge: []

# Per-repo overrides — .suspenders.yaml at a repo root:
#   rules: []            secret-scanner rule opt-ins/ignores for this repo
#   guard:
#     allowlist: []      appended to the global safe references
#     blocked_words: []  appended to the global blocked words`

var configDocCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current effective config",
	Long: `Show which config file suspenders loaded and the settings in effect after
defaults are applied — what the scanner and the guard run with, not what the
file happens to spell out.

The global config lives at ~/.config/suspenders/config.yaml (XDG_CONFIG_HOME
is respected; --config and $SUSPENDERS_CONFIG relocate it). It is optional:
with no file, the defaults below are what suspenders runs with, in memory —
loading never creates one. Run "suspenders config init" to write a starting
file. A repo can append to the guard lists and suppress scanner rules for
itself with a .suspenders.yaml at its root; those per-repo overrides are not
merged into the values below.

Every setting:

` + configReference + `

Pair it with doctor: doctor shows what the guard decided for a repo, config
shows which file and key changes it.`,
	Args: cobra.NoArgs,
	RunE: runConfigDoc,
}

var configInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Write a config file with the built-in defaults",
	Long: `Write the built-in defaults to the config file so they can be edited.

suspenders never creates this file on its own: it runs inside pre-commit
hooks, where an automatic write lands wherever git happened to put the
process. This command is the explicit way to get one, and it refuses to
overwrite a file that already exists.`,
	Args: cobra.NoArgs,
	RunE: runConfigInit,
}

func init() {
	configDocCmd.AddCommand(configInitCmd)
	rootCmd.AddCommand(configDocCmd)
}

func runConfigInit(cmd *cobra.Command, _ []string) error {
	path, err := configFilePath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s already exists — edit it, or delete it first", path)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check %s: %w", path, err)
	}
	if err := config.Write(path, config.DefaultConfig()); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s\n", path)
	return nil
}

// runConfigDoc prints the config location and the settings in effect. Unlike
// doctor, a broken config file is not fatal here: it is reported in the header
// status and the defaults suspenders would fall back to are printed anyway, so
// the command still works when the file it describes is the thing that is
// wrong.
func runConfigDoc(cmd *cobra.Command, _ []string) error {
	out := cmd.OutOrStdout()
	path, err := configFilePath()
	if err != nil {
		return err
	}
	_, statErr := os.Stat(path)

	cfg, loadErr := loadConfig()
	status := "loaded"
	switch {
	case loadErr != nil:
		status = "parse error: " + loadErr.Error()
		cfg = config.DefaultConfig()
	case statErr != nil:
		status = "missing, defaults in use"
	}

	fmt.Fprintf(out, "config file: %s (%s)\n", path, status)
	if statErr != nil && loadErr == nil {
		fmt.Fprintln(out, "             suspenders config init writes one with these defaults")
	}
	fmt.Fprintln(out, "per-repo:    .suspenders.yaml (or .yml) at a repo root")

	fmt.Fprintln(out)
	return encodeYAML(out, withResolvedDefaults(cfg))
}

// encodeYAML writes v at the 2-space indent the config file itself uses, so
// the output can go straight back into one.
func encodeYAML(w io.Writer, v any) error {
	enc := yaml.NewEncoder(w)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode effective config: %w", err)
	}
	return enc.Close()
}

// withResolvedDefaults copies cfg with its tri-state enable flags pinned to
// the value suspenders resolves them to, so an omitted `enabled` prints as
// true rather than null.
func withResolvedDefaults(cfg *config.Config) *config.Config {
	c := *cfg
	enabled := c.Scan.ScanEnabled()
	c.Scan.Enabled = &enabled
	c.Hooks.PreCommit = withResolvedHookDefaults(c.Hooks.PreCommit)
	c.Hooks.PostMerge = withResolvedHookDefaults(c.Hooks.PostMerge)
	return &c
}

func withResolvedHookDefaults(hooks []config.ExternalHook) []config.ExternalHook {
	if len(hooks) == 0 {
		return hooks
	}
	out := make([]config.ExternalHook, len(hooks))
	for i, h := range hooks {
		enabled := h.IsEnabled()
		h.Enabled = &enabled
		out[i] = h
	}
	return out
}

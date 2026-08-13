package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/suspenders/internal/config"
)

// configReference documents every setting suspenders reads, shipped in the
// binary so a blocked commit can be debugged from the terminal: doctor shows
// what the guard decided, config shows which file and key changes it.
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
	Short: "Print the config file location and an annotated reference of every setting",
	Long: `Print where the suspenders config lives and what every setting does, as an
annotated example. Pair it with doctor: doctor shows what the guard decided
for a repo, config shows which file and key changes it.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "config file: %s\n", config.Path())
		fmt.Fprintln(out, "per-repo:    .suspenders.yaml (or .yml) at a repo root")
		fmt.Fprintf(out, "\n%s\n", configReference)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configDocCmd)
}

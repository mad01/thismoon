package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// configReference documents every setting belt reads from its own config
// file. Shipped in the binary so an agent (or human) staring at a deny can go
// from `belt doctor` (what is the state) to `belt config` (which file and
// which key changes it) without hunting for repo docs.
const configReference = `# ~/.config/belt/config.yaml — every key optional; guards and hints default
# to enabled when the file or their entry is missing (fail closed, not silent).

guards:
  git-push-main:
    enabled: true
    # Exempt whole repos from this guard by canonical host/owner/repo,
    # matched against the push working dir's origin remote. This is how one
    # repo gets to push to its default branch while every other repo stays
    # fail-closed. Unknown profile or unresolved remote always denies.
    allow_repos:
      - github.com/you/yourrepo

  script-deny-list:
    enabled: true
    # Extra patterns denied inside scripts, beyond the Claude settings
    # permissions.deny Bash(...) entries (which are read live, never copied).
    extra_patterns:
      - rm -rf
    # Skip trusted script locations (substring match on the script path).
    exclude_paths:
      - /trusted/scripts/

  write-internal-names:
    enabled: true
    # Repos allowed to carry internal names despite a github.com remote
    # (canonical host/owner/repo, matched against the target file's origin
    # remote) — a private companion repo whose purpose is internal config.
    allow_repos:
      - github.com/you/private-companion
    # Paths where internal references are deliberate (substring match on the
    # target file path).
    exclude_paths:
      - /notes/
    # The blocked-name list itself is NOT configured here: it comes from the
    # guard: section of the suspenders config, shared with the pre-commit
    # guard. See suspenders config.

hints:
  prefer-csl:
    enabled: true
  keep-assertions:
    enabled: true`

func configDocCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print the config file locations and an annotated reference of every setting",
		Long: `Print where belt's config surfaces live and what every setting does, as an
annotated example config. Pair it with doctor: doctor shows the state belt
resolved, config shows which file and key to change.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := config.DefaultPaths()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "config file:  %s\n", p.BeltYAML)
			fmt.Fprintf(out, "              (legacy fallback %s, read only when the YAML file is absent)\n", p.BeltTOML)
			fmt.Fprintf(out, "also read:    %s  (guard: section = the blocked-name source)\n", p.Suspenders)
			fmt.Fprintf(out, "              %s  (profiles list = machine profile)\n", p.Ralph)
			fmt.Fprintf(out, "              %s  (permissions.deny Bash entries)\n", strings.Join(p.ClaudeSettings, " + "))
			fmt.Fprintf(out, "\n%s\n", configReference)
			return nil
		},
	}
}

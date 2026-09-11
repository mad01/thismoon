package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// shellFunctions are the `repo` and `repo-sync` shell functions, in a form
// both zsh and bash accept. The bodies are the same ones recipes/csl/recipe.toml
// installs on a ralph-managed machine ([shell.functions.repo] and
// [shell.functions.repo-sync]); TestShellInitMatchesRecipe keeps the two
// copies from drifting.
const shellFunctions = `repo() { local d=$(csl repo "$@"); [[ -n "$d" ]] && cd "$d"; }
repo-sync() { csl sync; }
`

var shellInitCmd = &cobra.Command{
	Use:   "shell-init <shell>",
	Short: "Print the repo and repo-sync shell functions for zsh or bash",
	Long: `Print two shell functions to stdout, for eval in your shell startup file:

  repo       jump to a repo: ` + "`repo <query>`" + ` cd's to the single match, bare
             ` + "`repo`" + ` opens the fuzzy picker (backed by ` + "`csl repo`" + `)
  repo-sync  pull every repo and reindex what changed (backed by ` + "`csl sync`" + `)

Add this line to ~/.zshrc or ~/.bashrc:

  eval "$(csl shell-init zsh)"

The functions cd in your shell, which a binary cannot do for you; that is the
only reason they exist. On a ralph-managed machine the csl recipe installs the
same two functions, so this command is for standalone installs.`,
	// The shell list lives once, in ValidArgs: it drives completion and, via
	// OnlyValidArgs, the rejection of anything else. Both shells get the same
	// text, so RunE has nothing to switch on.
	Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	ValidArgs: []string{"bash", "zsh"},
	RunE: func(cmd *cobra.Command, _ []string) error {
		fmt.Fprint(cmd.OutOrStdout(), shellFunctions)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(shellInitCmd)
}

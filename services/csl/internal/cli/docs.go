package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/services/csl"
)

// docsClaudeMDFlag holds --claude-md on the `docs` command.
var docsClaudeMDFlag bool

func init() {
	rootCmd.AddCommand(docsCommand())
}

// docsCommand is the shared agentcli `docs` command with one csl-only
// addition: --claude-md prints the CLAUDE.md snippet instead of the operating
// doc, so users can pipe it straight into their CLAUDE.md. The flag lives here
// rather than in kit/agentdoc because no other component has such a snippet.
func docsCommand() *cobra.Command {
	cmd := agentcli.DocsCommand(csl.OperatingDoc, csl.Facts())

	cmd.Flags().BoolVar(&docsClaudeMDFlag, "claude-md", false,
		"print the CLAUDE.md snippet that teaches an agent to use the csl_* MCP tools")

	cmd.Long += `

With --claude-md, print the CLAUDE.md snippet instead: the block that tells an
agent when to reach for the csl_* MCP tools. Append it to ~/.claude/CLAUDE.md
or a project CLAUDE.md:

  csl docs --claude-md >> ~/.claude/CLAUDE.md`

	operatingDoc := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		if docsClaudeMDFlag {
			// The leading newline keeps the `>>` append from gluing the
			// heading onto a file whose last line has no trailing newline.
			fmt.Fprint(cmd.OutOrStdout(), "\n"+csl.ClaudeMD)
			return nil
		}
		return operatingDoc(cmd, args)
	}

	return cmd
}

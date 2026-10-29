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
// addition: --claude-md prints the csl rules fragment instead of the operating
// doc, so users can pipe it straight into their CLAUDE.md. The flag lives here
// rather than in kit/agentdoc because no other component has such a snippet.
func docsCommand() *cobra.Command {
	cmd := agentcli.DocsCommand(csl.OperatingDoc, csl.Facts())

	cmd.Flags().BoolVar(&docsClaudeMDFlag, "claude-md", false,
		"print the CLAUDE.md fragment that tells an agent when to use csl, as MCP tools or CLI")

	cmd.Long += `

With --claude-md, print the csl rules fragment instead: the block that tells
an agent when to reach for csl, and when to use the MCP tools or the CLI.
Append it to ~/.claude/CLAUDE.md or a project CLAUDE.md:

  csl docs --claude-md >> ~/.claude/CLAUDE.md

The csl ralph recipe links the same file to ~/.claude/CLAUDE.csl.md, which a
CLAUDE.md can import with the line @~/.claude/CLAUDE.csl.md.`

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

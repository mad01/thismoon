package commands

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	suspenders "github.com/mad01/thismoon/tools/suspenders"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Print the operating doc for agents and humans debugging suspenders",
	Long: `Print the embedded operating doc: how the hook pipeline runs, where config
lives, the known failure modes when a commit is blocked, and the first moves.
Rendered from the same facts the binary runs with, so it cannot drift from
the code.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		doc, err := agentdoc.Render(suspenders.OperatingDoc, suspenders.Facts())
		if err != nil {
			return err
		}
		fmt.Fprint(cmd.OutOrStdout(), doc)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(docsCmd)
}

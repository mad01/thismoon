package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	deps "github.com/mad01/thismoon/services/deps"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Print the operating doc for agents and humans debugging deps",
	Long: `Print the embedded operating doc: how deps runs, where its state lives, the
known failure modes, and the first moves when something is off. Rendered from
the same defaults the binary runs with, so it cannot drift from the code.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		doc, err := agentdoc.Render(deps.OperatingDoc, deps.Facts())
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

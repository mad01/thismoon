package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/humanizer"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Print the operating doc for agents and humans debugging humanizer",
	Long: `Print the embedded operating doc: how humanizer runs, where the vale style
pack lives, the known failure modes, and the first moves when something is
off. Rendered from the same defaults the binary runs with, so it cannot
drift from the code.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		doc, err := agentdoc.Render(humanizer.OperatingDoc, humanizer.Facts())
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

package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	kof "github.com/mad01/thismoon/services/keeper-of-facts"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Print the operating doc for agents and humans debugging kof",
	Long: `Print the embedded operating doc: how kof runs, where its state lives, the
known failure modes, and the first moves when something is off. Rendered from
the same defaults the binary runs with, so it cannot drift from the code.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		doc, err := agentdoc.Render(kof.OperatingDoc, kof.Facts())
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

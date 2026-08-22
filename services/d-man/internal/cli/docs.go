package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	dman "github.com/mad01/thismoon/services/d-man"
)

var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Print the operating doc for agents and humans debugging d-man",
	Long: `Print the embedded operating doc: how d-man runs, where the routes file lives,
the reload semantics, the known failure modes, and the first moves when a .this
host stops resolving. Rendered from the same defaults the binary runs with, so
it cannot drift from the code.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		doc, err := agentdoc.Render(dman.OperatingDoc, dman.Facts())
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

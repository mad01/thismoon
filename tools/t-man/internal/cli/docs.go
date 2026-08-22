package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	tman "github.com/mad01/thismoon/tools/t-man"
)

// docsCmd represents the docs command
var docsCmd = &cobra.Command{
	Use:   "docs",
	Short: "Print the operating doc for agents and humans debugging services",
	Long: `Print the embedded operating doc: how t-man drives launchd, where plists and
service logs live, the known failure modes, and the first moves when a
supervised service is down. Rendered from the same defaults the binary runs
with, so it cannot drift from the code.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		doc, err := agentdoc.Render(tman.OperatingDoc, tman.Facts())
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

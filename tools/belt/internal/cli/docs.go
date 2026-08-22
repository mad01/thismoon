package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	belt "github.com/mad01/thismoon/tools/belt"
)

func docsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print the operating doc for agents and humans debugging belt",
		Long: `Print the embedded operating doc: how belt runs, where its config lives, the
known failure modes, and the first moves when something is off. Rendered from
the same defaults the binary runs with, so it cannot drift from the code.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := agentdoc.Render(belt.OperatingDoc, belt.Facts())
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), doc)
			return nil
		},
	}
}

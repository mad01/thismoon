package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/tools/worklog"
)

func docsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print the operating doc for agents and humans debugging worklog",
		Long: `Print the embedded operating doc: how worklog runs, where its store lives, the
known failure modes, and the first moves when something is off. Rendered from
the same defaults the binary runs with, so it cannot drift from the code.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			doc, err := agentdoc.Render(worklog.OperatingDoc, worklog.Facts())
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), doc)
			return nil
		},
	}
}

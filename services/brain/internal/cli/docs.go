package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/docs"
)

func newDocsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: "Print the agent-facing CLI guide",
		Long: `Print the brain CLI guide for agents: commands, flags, exit codes, JSON
shapes, zoekt query tips and the capture-then-promote workflow.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprint(cmd.OutOrStdout(), docs.CLI)
			return nil
		},
	}
}

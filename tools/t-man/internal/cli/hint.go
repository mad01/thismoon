package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	tman "github.com/mad01/thismoon/tools/t-man"
)

// hintRunE wraps a command's RunE so any error it returns carries the
// operating-doc pointer exactly once, instead of every return site inside the
// command wrapping its own.
func hintRunE(run func(*cobra.Command, []string) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, args []string) error {
		return agentdoc.Hint(run(cmd, args), tman.Facts())
	}
}

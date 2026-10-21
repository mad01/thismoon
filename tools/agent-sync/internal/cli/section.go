package cli

import (
	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/tools/agent-sync/internal/apply"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

// section describes one config section's command family: `agent-sync
// <name> sync|plan|check`.
type section struct {
	// name is the subcommand, the same word as the config section.
	name string
	// short and long are the family's help text; long says what the
	// section syncs and where its data comes from.
	short string
	long  string
	// run does the work for one mode. It prints its own report to out and
	// returns apply.Exit for drift, or an error for main to print.
	run func(file *config.File, mode apply.Mode, out apply.Streams) error
}

// modeHelp is each mode's one-line help, shared by every section so the
// three families read alike.
var modeHelp = []struct {
	mode  apply.Mode
	short string
}{
	{apply.Sync, "Apply the change to the live files"},
	{apply.Plan, "Print what sync would change; write nothing"},
	{apply.Check, "Exit 1 when sync would change anything; write nothing"},
}

// sectionCommand builds the family for s. Each subcommand loads the config
// when it runs, after flags resolve.
func (a *app) sectionCommand(s section) *cobra.Command {
	parent := &cobra.Command{Use: s.name, Short: s.short, Long: s.long, Args: cobra.NoArgs}
	for _, h := range modeHelp {
		mode := h.mode
		parent.AddCommand(&cobra.Command{
			Use:   mode.String(),
			Short: h.short,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				file, err := a.load()
				if err != nil {
					return err
				}
				return s.run(
					file,
					mode,
					apply.Streams{Out: cmd.OutOrStdout(), Err: cmd.ErrOrStderr()},
				)
			},
		})
	}
	return parent
}

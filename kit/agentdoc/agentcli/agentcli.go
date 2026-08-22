// Package agentcli builds the cobra commands behind a component's
// agent-facing surfaces: `docs` printing the embedded operating doc and
// `doctor` running the kit/doctor checks. It lives beside agentdoc so
// agentdoc itself stays free of the cobra dependency.
package agentcli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
)

// DocsCommand returns the `docs` command: it renders doc (the embedded
// operating.md template) against f and prints it.
func DocsCommand(doc string, f agentdoc.Facts) *cobra.Command {
	return &cobra.Command{
		Use:   "docs",
		Short: fmt.Sprintf("Print the operating doc for agents and humans debugging %s", f.Bin),
		Long: fmt.Sprintf(`Print the embedded operating doc: how %s runs, where its state lives, the
known failure modes, and the first moves when something is off. Rendered from
the same defaults the binary runs with, so it cannot drift from the code.`, f.Bin),
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out, err := agentdoc.Render(doc, f)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}

// DoctorCommand returns the `doctor` command: it runs checks through
// doctor.Run against f's component. checks is invoked at run time, after
// flags and env have resolved, so the probes hit the same targets the
// other subcommands use.
func DoctorCommand(f agentdoc.Facts, checks func(ctx context.Context) []doctor.Check) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: fmt.Sprintf("Run %s's diagnostic checks", f.Bin),
		Long: fmt.Sprintf(`Run %s's executable self-checks, one line per check: "ok" or "FAIL" with
the cause. Exits nonzero when any check fails.`, f.Bin),
		Args: cobra.NoArgs,
		// A failing check is a diagnosis, not a usage mistake; keep cobra
		// from printing help under the report, and from printing the error
		// a second time before main does.
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			return doctor.Run(ctx, cmd.OutOrStdout(), f, checks(ctx))
		},
	}
}

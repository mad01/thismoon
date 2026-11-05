package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain/internal/doctor"
)

func newDoctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config, vault, index and Ollama",
		Long: `Check that brain can work: the config resolves, the vault exists, the index
directory is writable, the literal index is fresh, and (only when
semantic.enabled is true) Ollama answers. Prints one line per check with the
fix for any failure. Exits 0 when every check passes, 1 otherwise. The
brain_doctor MCP tool returns the same report.`,
		Args: exactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			report := doctor.Run(cmd.Context(), a.configPath)
			err := a.emit(cmd.OutOrStdout(), report, func(w io.Writer) error {
				writeDoctorText(w, report)
				return nil
			})
			if err != nil {
				return err
			}
			if !report.OK {
				return &reportedError{err: fmt.Errorf("%d check(s) failed", report.Failed())}
			}
			return nil
		},
	}
}

func writeDoctorText(w io.Writer, r doctor.Report) {
	for _, c := range r.Checks {
		fmt.Fprintf(w, "%-4s  %-10s %s", c.Status, c.Name, c.Detail)
		if c.Fix != "" {
			fmt.Fprintf(w, " (fix: %s)", c.Fix)
		}
		fmt.Fprintln(w)
	}
}

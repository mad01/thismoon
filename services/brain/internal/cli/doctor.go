package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/doctor"
)

func newDoctorCmd(a *app) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check config, vaults, indexes, Ollama and a running brain serve",
		Long: `Check that brain can work: the config resolves (config), every vault exists
(vault), the index directory is writable (index-dir), the literal indexes are
fresh (index), no two notes share an id (ids), and, only when
semantic.enabled is true, Ollama answers (ollama).

Then check brain serve on 127.0.0.1 (port $` + brain.PortEnv + `, else ` + fmt.Sprint(brain.DefaultPort) + `): it
answers (serve-reachable), it runs this build (version-skew), it sees the
vaults this CLI sees (serve-vaults) and it shares this CLI's state
directory (serve-state-dir). serve is optional, so when nothing listens
these checks are skipped, not failed.

Prints one line per check with the fix for any failure. Exits 0 when every
check passes, 1 otherwise. The brain_doctor MCP tool returns the same report.`,
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

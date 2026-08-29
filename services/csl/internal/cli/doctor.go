package cli

import (
	"context"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/selfcheck"
)

var doctorRepairFlag bool

func init() {
	// Checks build at run time so --repair has resolved before the state
	// check decides whether to reset a corrupt file, and so the web probes
	// target the config and CSL_PORT this invocation resolved.
	doctorCmd := agentcli.DoctorCommand(csl.Facts(), func(context.Context) []doctor.Check {
		return selfcheck.Checks(doctorRepairFlag)
	})
	doctorCmd.Flags().
		BoolVar(&doctorRepairFlag, "repair", false, "back up a corrupt state file and reset it")
	rootCmd.AddCommand(doctorCmd)
}

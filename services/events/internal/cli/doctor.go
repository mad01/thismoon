package cli

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/events"
)

func init() {
	// Checks build at run time so they probe the resolved --port/--workdir
	// (env vars included), not the compile-time defaults.
	rootCmd.AddCommand(agentcli.DoctorCommand(events.Facts(), func(context.Context) []doctor.Check {
		baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
		return []doctor.Check{
			doctor.ServiceReachable(baseURL),
			doctor.StoreReadable(flagWorkdir),
			doctor.VersionSkew(baseURL),
		}
	}))
}

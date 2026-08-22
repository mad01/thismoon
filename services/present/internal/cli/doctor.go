package cli

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	present "github.com/mad01/thismoon/services/present"
)

func init() {
	// Checks build at run time so they probe the resolved --port/--workdir
	// (env vars included), not the compile-time defaults. Store first: the
	// MCP tools write the page store directly, so a readable store means the
	// tools work even with serve down — the reachability and skew checks
	// diagnose the URL-serving half only.
	rootCmd.AddCommand(agentcli.DoctorCommand(present.Facts(), func(context.Context) []doctor.Check {
		baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
		return []doctor.Check{
			doctor.StoreReadable(flagWorkdir),
			doctor.ServiceReachable(baseURL),
			doctor.VersionSkew(baseURL),
		}
	}))
}

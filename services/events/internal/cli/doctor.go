package cli

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/events"
)

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(events.Facts(), doctorChecks))
}

// doctorChecks builds the diagnostics at run time so they probe the resolved
// --port/--workdir (env vars included), not the compile-time defaults. The
// `doctor` command and the events_doctor MCP tool share this one set, so an
// agent with no shell gets the same diagnosis a human does.
func doctorChecks(context.Context) []doctor.Check {
	baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
	return []doctor.Check{
		doctor.ServiceReachable(baseURL),
		doctor.StoreReadable(flagWorkdir),
		doctor.VersionSkew(baseURL),
	}
}

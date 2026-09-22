package cli

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	present "github.com/mad01/thismoon/services/present"
)

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(present.Facts(), doctorChecks))
}

// doctorChecks builds the diagnostics at run time so they probe the resolved
// --port/--workdir (env vars included), not the compile-time defaults. Store
// first: the MCP tools write the page store directly, so a readable store
// means the tools work even with serve down — the reachability and skew
// checks diagnose the URL-serving half only. The `doctor` command and the
// present_doctor MCP tool share this one set, so an agent with no shell gets
// the same diagnosis a human does.
func doctorChecks(context.Context) []doctor.Check {
	baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
	return []doctor.Check{
		doctor.StoreReadable(flagWorkdir),
		doctor.ServiceReachable(baseURL),
		doctor.VersionSkew(baseURL),
		sharedInstanceCheck(),
	}
}

// sharedInstanceCheck asks the configured shared instance who this
// machine's author key is: one round trip proves the instance is reachable
// and the key arrives intact. Nothing configured is a skip, not a failure.
func sharedInstanceCheck() doctor.Check {
	return doctor.Check{
		Name: "shared-instance",
		Run: func(ctx context.Context) error {
			c := sharer()
			if c == nil {
				return doctor.Skip(
					"skipped: no shared instance configured (--shared-url, --author-key)",
				)
			}
			if _, err := c.WhoAmI(ctx); err != nil {
				return fmt.Errorf("shared instance %s: %w", c.BaseURL, err)
			}
			return nil
		},
	}
}

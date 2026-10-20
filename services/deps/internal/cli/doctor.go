package cli

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	deps "github.com/mad01/thismoon/services/deps"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(deps.Facts(), doctorChecks))
}

// doctorChecks builds the diagnostics at run time so they probe the resolved
// --port/--workdir (env vars included), not the compile-time defaults. The
// `doctor` command and the deps_doctor MCP tool share this one set, so an
// agent with no shell gets the same diagnosis a human does.
func doctorChecks(context.Context) []doctor.Check {
	baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
	return []doctor.Check{
		doctor.ServiceReachable(baseURL),
		doctor.StoreReadable(flagWorkdir),
		doctor.VersionSkew(baseURL),
		discoveryClean(flagWorkdir),
	}
}

// discoveryClean fails when the last scan serve saved could not read some repo
// and ecosystem. It reads the errors from the store serve wrote rather than
// re-running discovery here: doctor runs with the login shell's PATH, so a
// toolchain it finds says nothing about the launchd PATH serve scans with.
func discoveryClean(workdir string) doctor.Check {
	return doctor.Check{
		Name: "discovery-clean",
		Run: func(context.Context) error {
			sc, err := store.Read(workdir)
			if err != nil {
				return err
			}
			if sc.ScannedAt.IsZero() {
				return doctor.Skip("skipped: no scan has completed yet")
			}
			if len(sc.DiscoveryErrors) == 0 {
				return nil
			}
			first := sc.DiscoveryErrors[0]
			return fmt.Errorf(
				"last scan could not read %d repo/ecosystem pair(s), so their dependencies went unchecked; first: %s in %s: %s",
				len(sc.DiscoveryErrors),
				first.Ecosystem,
				first.Repo,
				first.Error,
			)
		},
	}
}

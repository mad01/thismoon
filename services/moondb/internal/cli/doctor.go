package cli

import (
	"context"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
)

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(moondb.Facts(), doctorChecks))
}

// doctorChecks builds the diagnostics at run time so they probe the resolved
// --port/--workdir (env vars and config file included), not the
// compile-time defaults. The `doctor` command and the moondb_doctor MCP tool
// share this one set, so an agent with no shell gets the same diagnosis a
// human does.
func doctorChecks(context.Context) []doctor.Check {
	api := apiURL()
	return []doctor.Check{
		{Name: "config", Run: func(context.Context) error { return cfgErr }},
		doctor.ServiceReachable(api),
		{Name: "store-ping", Run: client.New(api).Health},
		doctor.StoreReadable(flagWorkdir),
		doctor.VersionSkew(api),
	}
}

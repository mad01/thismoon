package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/client"
	"github.com/mad01/thismoon/services/moondb/internal/store"
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
		{Name: "config", Run: checkConfig},
		{Name: "hooks-wired", Run: hooksWired},
		doctor.ServiceReachable(api),
		{Name: "store-ping", Run: client.New(api).Health},
		doctor.StoreReadable(flagWorkdir),
		doctor.VersionSkew(api),
		{Name: "agent-briefs", Run: briefsCheck(client.New(api))},
	}
}

// briefsCheck warns about agents an import could still give a spawn
// brief or later messages: they were recorded before moondb kept them.
// The check passes with the count and the command that fills them as its
// note, since the edits and traces of those agents are whole; only the
// brief or the messages are missing. A serve it cannot ask is left to
// service-reachable and version-skew.
func briefsCheck(c *client.Client) func(context.Context) error {
	return func(ctx context.Context) error {
		v, err := c.Doctor(ctx)
		switch {
		case errors.Is(err, store.ErrNotFound):
			return doctor.Skip("serve predates this check; see version-skew")
		case err != nil:
			return doctor.Skip("serve did not answer; see service-reachable")
		case v.ToFill() > 0:
			return doctor.Skip(fmt.Sprintf(
				"%d agents without a spawn brief or later messages; run moondb import --since %s --force",
				v.ToFill(),
				moondb.DefaultImportSince,
			))
		}
		return nil
	}
}

// checkConfig fails on a config file that will not parse, and on a
// [prices] table serve would refuse to start with.
func checkConfig(context.Context) error {
	if cfgErr != nil {
		return cfgErr
	}
	_, err := priceOverrides(cfg.Prices)
	return err
}

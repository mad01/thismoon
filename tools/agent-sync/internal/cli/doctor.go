package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	agentsync "github.com/mad01/thismoon/tools/agent-sync"
	"github.com/mad01/thismoon/tools/agent-sync/internal/mcp"
	"github.com/mad01/thismoon/tools/agent-sync/internal/mods"
	"github.com/mad01/thismoon/tools/agent-sync/internal/settings"
)

// doctorCmd checks the config file first, then hands each section to the
// package that owns it. A config that does not load stops there: no section
// can be checked without it.
func (a *app) doctorCmd() *cobra.Command {
	return agentcli.DoctorCommand(agentsync.Facts(), func(context.Context) []doctor.Check {
		file, err := a.load()
		checks := []doctor.Check{{Name: "config", Run: func(context.Context) error {
			return err
		}}}
		if err != nil {
			return checks
		}
		checks = append(checks, settings.DoctorChecks(file.Settings)...)
		checks = append(checks, mods.DoctorChecks(file.Mods)...)
		return append(checks, mcp.DoctorChecks(file.MCP)...)
	})
}

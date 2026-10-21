package mcp

import (
	"context"
	"io"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
	"github.com/mad01/thismoon/tools/agent-sync/internal/gate"
)

// DoctorChecks returns the mcp section's checks: the section parses, the
// servers file and drop-ins load into a valid desired set for this machine,
// and the claude CLI is on PATH.
func DoctorChecks(s config.Section) []doctor.Check {
	if !s.Present() {
		return []doctor.Check{{Name: "mcp", Run: func(context.Context) error {
			return doctor.Skip("no mcp section in the config")
		}}}
	}
	cfg, err := ParseConfig(s)
	checks := []doctor.Check{{Name: "mcp-config", Run: func(context.Context) error {
		return err
	}}}
	if err != nil {
		return checks
	}
	return append(
		checks,
		doctor.Check{Name: "mcp-servers", Run: func(context.Context) error {
			m, err := gate.Current(cfg.ProfilesFile)
			if err != nil {
				return err
			}
			_, err = loadDesired(cfg, m, io.Discard)
			return err
		}},
		doctor.Check{Name: "mcp-claude", Run: func(context.Context) error {
			_, err := findClaude()
			return err
		}},
	)
}

package cli

import (
	"context"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	prs "github.com/mad01/thismoon/services/prs"
	"github.com/mad01/thismoon/services/prs/internal/config"
	"github.com/mad01/thismoon/services/prs/internal/github"
)

func init() {
	rootCmd.AddCommand(agentcli.DoctorCommand(prs.Facts(), doctorChecks))
}

// doctorChecks builds the diagnostics at run time so they probe the resolved
// --port/--workdir/--config (env vars included), not the compile-time
// defaults. The `doctor` command and the prs_doctor MCP tool share this one
// set, so an agent with no shell gets the same diagnosis a human does.
func doctorChecks(context.Context) []doctor.Check {
	baseURL := fmt.Sprintf("http://localhost:%d", flagPort)
	return []doctor.Check{
		doctor.ServiceReachable(baseURL),
		doctor.StoreReadable(flagWorkdir),
		doctor.VersionSkew(baseURL),
		configValid(flagConfig),
		ghAvailable(),
	}
}

// configValid checks the YAML config parses and names at least one dir to
// scan — the "why is the page empty" failure.
func configValid(path string) doctor.Check {
	return doctor.Check{
		Name: "config-valid",
		Run: func(context.Context) error {
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			if !cfg.Loaded {
				return fmt.Errorf("no config at %s — create it with a dirs: list", cfg.Path)
			}
			if len(cfg.Dirs) == 0 {
				return fmt.Errorf("%s sets no dirs — nothing will be polled", cfg.Path)
			}
			return nil
		},
	}
}

// ghAvailable checks the gh binary resolves — it mints every API token.
func ghAvailable() doctor.Check {
	return doctor.Check{
		Name: "gh-available",
		Run: func(context.Context) error {
			return github.GhAvailable()
		},
	}
}

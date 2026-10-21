package mods

import (
	"context"
	"fmt"
	"strings"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/agent-sync/internal/claudecode"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
)

// DoctorChecks returns the mods section's checks: the section parses, the
// mods files load, each marketplace on disk has a matching manifest,
// Claude Code's plugin state parses, and the claude CLI is on PATH.
func DoctorChecks(s config.Section) []doctor.Check {
	if !s.Present() {
		return []doctor.Check{{Name: "mods", Run: func(context.Context) error {
			return doctor.Skip("no mods section in the config")
		}}}
	}
	cfg, err := ParseConfig(s)
	checks := []doctor.Check{{Name: "mods-config", Run: func(context.Context) error {
		return err
	}}}
	if err != nil {
		return checks
	}
	return append(
		checks,
		doctor.Check{Name: "mods-files", Run: func(context.Context) error {
			_, _, err := loadSources(cfg)
			return err
		}},
		doctor.Check{Name: "mods-marketplaces", Run: func(context.Context) error {
			return checkMarketplaces(cfg)
		}},
		doctor.Check{Name: "mods-state", Run: func(context.Context) error {
			dir, err := claudeDir()
			if err != nil {
				return err
			}
			_, err = readState(dir)
			return err
		}},
		doctor.Check{Name: "mods-claude", Run: func(context.Context) error {
			if _, err := claudecode.LookPath(); err != nil {
				return fmt.Errorf("claude CLI not found on PATH; sync needs it to apply changes")
			}
			return nil
		}},
	)
}

// checkMarketplaces fails on a manifest sync would reject. A marketplace
// that is not on disk yet only gets a note: sync skips it the same way.
func checkMarketplaces(cfg Config) error {
	sources, _, err := loadSources(cfg)
	if err != nil {
		return doctor.Skip("mods files do not load; see mods-files")
	}
	var missing []string
	for _, f := range sources {
		_, onDisk, err := listing(f)
		if err != nil {
			return err
		}
		if !onDisk {
			missing = append(missing, fmt.Sprintf("%s at %s", f.marketplace, f.path))
		}
	}
	if len(missing) > 0 {
		return doctor.Skip("not on disk yet, sync skips them: " + strings.Join(missing, ", "))
	}
	return nil
}

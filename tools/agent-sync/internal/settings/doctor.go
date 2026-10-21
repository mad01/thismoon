package settings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/tools/agent-sync/internal/config"
	"github.com/mad01/thismoon/tools/agent-sync/internal/pyjson"
)

// DoctorChecks returns the settings section's checks: the section parses,
// the template is a readable JSON object with no prune conflict, the
// overlay (when there is one) is too, and sync could write the live path.
func DoctorChecks(s config.Section) []doctor.Check {
	if !s.Present() {
		return []doctor.Check{{Name: "settings", Run: func(context.Context) error {
			return doctor.Skip("no settings section in the config")
		}}}
	}
	cfg, err := ParseConfig(s)
	checks := []doctor.Check{{Name: "settings-config", Run: func(context.Context) error {
		return err
	}}}
	if err != nil {
		return checks
	}
	return append(
		checks,
		doctor.Check{Name: "settings-template", Run: func(context.Context) error {
			return checkTemplate(cfg)
		}},
		doctor.Check{Name: "settings-overlay", Run: func(context.Context) error {
			return checkOverlay(cfg.Overlay)
		}},
		doctor.Check{Name: "settings-live-writable", Run: func(context.Context) error {
			return checkLiveWritable(cfg.Live)
		}},
	)
}

func checkTemplate(cfg Config) error {
	template, err := readTemplate(cfg.Template)
	if err != nil {
		return err
	}
	if conflicts := pruneConflicts(template, cfg.Prune); len(conflicts) > 0 {
		return fmt.Errorf("settings.prune retires %s, which the template still sets", conflicts[0])
	}
	return nil
}

// checkOverlay fails on an overlay sync would skip: sync only warns, so
// doctor is where a broken overlay gets noticed.
func checkOverlay(path string) error {
	present, err := exists(path)
	if err != nil {
		return fmt.Errorf("overlay %s: %w", path, err)
	}
	if !present {
		return doctor.Skip(fmt.Sprintf("no overlay at %s; it is optional", path))
	}
	v, err := pyjson.ReadFile(path)
	if err != nil {
		return fmt.Errorf("overlay %s: %w", path, err)
	}
	_, err = asObject(v, "overlay", path)
	return err
}

// checkLiveWritable checks what sync needs. Sync writes a temp file beside
// the live path and renames it over, replacing a regular file, a hard link
// or a symlink (dangling or not) in place, so what must be writable is the
// live path's directory. When that directory does not exist yet, the
// nearest existing ancestor sync would create it in must be.
func checkLiveWritable(path string) error {
	if _, err := lexists(path); err != nil {
		return fmt.Errorf("live settings: %w", err)
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		info, err := os.Stat(dir)
		found, err := present(info, err)
		if err != nil {
			return fmt.Errorf("live settings: %w", err)
		}
		if found && !info.IsDir() {
			return fmt.Errorf("live settings: %s is not a directory", dir)
		}
		if found {
			return access(dir)
		}
		if filepath.Dir(dir) == dir {
			return fmt.Errorf("live settings: no existing directory above %s", path)
		}
	}
}

func access(path string) error {
	if err := unix.Access(path, unix.W_OK); err != nil {
		return fmt.Errorf("%s is not writable: %w", path, err)
	}
	return nil
}

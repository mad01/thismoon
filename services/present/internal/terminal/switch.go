package terminal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// SwitchFile is the state file under the workdir whose existence turns the
// live terminal on. A file rather than a flag, so `present terminal on` and
// `off` work while serve runs or is down, with no restart.
const SwitchFile = "terminal.enabled"

// SwitchPoll is how often serve checks that the switch is still on.
const SwitchPoll = 2 * time.Second

// SwitchPath is the switch file in workdir.
func SwitchPath(workdir string) string {
	return filepath.Join(workdir, SwitchFile)
}

// Enabled reports whether the switch file at path exists. Any error but
// its absence is returned with false, so a caller that cannot tell treats
// the terminal as off.
func Enabled(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("terminal: switch %s: %w", path, err)
}

// Enable creates the switch file at path, and its directory with it.
func Enable(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("terminal: switch directory: %w", err)
	}
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		return fmt.Errorf("terminal: switch on: %w", err)
	}
	return nil
}

// Disable removes the switch file at path; a switch already off is fine.
func Disable(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("terminal: switch off: %w", err)
	}
	return nil
}

// WatchSwitch checks the switch at path every interval until ctx ends and
// kills every session while it is off, so turning the terminal off ends
// running sessions within one interval. A switch it cannot read counts as
// off; logf reports why when that starts, and again once the switch reads.
func (m *Manager) WatchSwitch(
	ctx context.Context, path string, every time.Duration, logf func(string, ...any),
) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	unreadable := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		on, err := Enabled(path)
		switch {
		case err != nil && !unreadable:
			logf("present: %v; treating the live terminal as off", err)
		case err == nil && unreadable:
			logf("present: live terminal switch %s reads again", path)
		}
		unreadable = err != nil
		if !on {
			m.KillAll()
		}
	}
}

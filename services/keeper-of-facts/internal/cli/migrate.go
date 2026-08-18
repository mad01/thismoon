package cli

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// migrateWorkdir moves the pre-rename store directory (~/.local/share/keep)
// to the kof path (~/.local/share/kof), once, on serve start (MAD-269). It
// only acts when serve targets the new default — a custom --workdir or
// KOF_WORKDIR is someone's deliberate choice and never guessed about. An
// empty directory already at the new path (a crashed first start) is cleared;
// a non-empty one alongside the old store is ambiguous, and refusing loudly
// beats guessing which of two stores is real.
func migrateWorkdir(workdir string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // no home to derive the defaults from; store.New will cope
	}
	newDefault := filepath.Join(home, ".local", "share", "kof")
	oldDefault := filepath.Join(home, ".local", "share", "keep")
	if workdir != newDefault {
		return nil
	}
	if info, err := os.Stat(oldDefault); err != nil || !info.IsDir() {
		return nil // nothing to migrate
	}
	if entries, err := os.ReadDir(newDefault); err == nil {
		if len(entries) > 0 {
			return fmt.Errorf(
				"kof: both %s and %s hold data — refusing to guess which store is real; merge or remove one, then restart",
				oldDefault, newDefault,
			)
		}
		if err := os.Remove(newDefault); err != nil {
			return fmt.Errorf("kof: clear empty %s before migration: %w", newDefault, err)
		}
	}
	if err := os.Rename(oldDefault, newDefault); err != nil {
		return fmt.Errorf("kof: migrate store %s -> %s: %w", oldDefault, newDefault, err)
	}
	log.Printf("kof: migrated store %s -> %s", oldDefault, newDefault)
	return nil
}

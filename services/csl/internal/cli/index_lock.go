package cli

import (
	"errors"
	"fmt"

	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

// withIndexLock runs write while holding the cross-process sync lock on
// indexDir, so an index command never overlaps csl sync, the csl web
// refresher, or another index run on shards and state.json. The lock is
// released on every exit path, including a panic inside write. A live holder
// fails fast with lockedHint's message before write runs at all.
func withIndexLock(indexDir string, write func() error) error {
	unlock, err := syncer.Lock(indexDir)
	if err != nil {
		return lockedHint(err)
	}
	defer unlock()
	return write()
}

// lockedHint turns a held sync lock into the fail-fast error csl sync and the
// csl index writers share: who holds the lock and what to do next. Any other
// error passes through unchanged.
func lockedHint(err error) error {
	if !errors.Is(err, syncer.ErrLocked) {
		return err
	}
	return fmt.Errorf(
		"another sync or background refresh holds the index lock; wait for it to finish, or check 'csl doctor' if none is running: %w",
		err,
	)
}

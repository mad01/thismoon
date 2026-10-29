package replicate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/mad01/thismoon/services/moondb/internal/store/local"
)

// state is the cursor file: how far the log has been sent, and to which
// instance.
type state struct {
	Target string       `json:"target"`
	Cursor local.Cursor `json:"cursor"`
}

// loadCursor reads the cursor saved for target. No file, a file for
// another target, or a cursor the log under dir no longer reaches (the
// store was moved or wiped) starts from the beginning of the log; the last
// two say so through logf.
func loadCursor(path, target, dir string, logf func(string, ...any)) (local.Cursor, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return local.Cursor{}, nil
	}
	if err != nil {
		return local.Cursor{}, fmt.Errorf("replicate: read %s: %w", path, err)
	}
	var st state
	if err := json.Unmarshal(raw, &st); err != nil {
		return local.Cursor{}, fmt.Errorf("replicate: parse %s: %w", path, err)
	}
	if st.Target != target {
		logf("moondb: replicate: new target %s (was %s); sending the log from its start",
			target, st.Target)
		return local.Cursor{}, nil
	}
	if !reaches(dir, st.Cursor) {
		logf("moondb: replicate: the log no longer reaches %+v; sending it from its start",
			st.Cursor)
		return local.Cursor{}, nil
	}
	return st.Cursor, nil
}

// reaches reports whether the cursor's segment is there and holds at least
// its offset.
func reaches(dir string, c local.Cursor) bool {
	if c.Segment == 0 {
		return true
	}
	info, err := os.Stat(local.SegmentPath(dir, c.Segment))
	return err == nil && info.Size() >= c.Offset
}

// saveCursor writes the cursor for target whole: a temp file renamed into
// place, so a crash leaves the old cursor or the new one.
func saveCursor(path, target string, c local.Cursor) error {
	raw, err := json.Marshal(state{Target: target, Cursor: c})
	if err != nil {
		return fmt.Errorf("replicate: encode cursor: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".replicate-*")
	if err != nil {
		return fmt.Errorf("replicate: save cursor: %w", err)
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("replicate: save cursor: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("replicate: save cursor: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		_ = os.Remove(tmp.Name())
		return fmt.Errorf("replicate: save cursor: %w", err)
	}
	return nil
}

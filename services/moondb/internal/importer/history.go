package importer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/mad01/thismoon/services/moondb"
)

// history is the file-history versions a transcript lists, by file. The
// harness copies a file into <file-history>/<session>/<backupFileName>
// when a session first edits it and again at each later prompt when it
// changed, and lists each copy in a file-history-snapshot or
// file-history-delta entry with the time it was taken. A version that names
// no file means the file did not exist then.
//
// The copy taken last before an edit holds the file as the edit found it
// only while nothing else edited the file in between: a second edit of the
// same file in one turn finds the file as the first edit left it, which no
// copy holds. The emitter checks that (usable).
type history struct {
	byPath map[string][]version
}

// version is one copy of a file.
type version struct {
	name string // the backup file name; "" when the file did not exist
	at   time.Time
}

// add reads the versions a file-history entry lists. Relative paths are
// taken against the session's cwd.
func (h *history) add(e entry, cwd string) {
	switch e.Type {
	case typeSnapshot:
		var s snapshot
		if json.Unmarshal(e.Snapshot, &s) != nil {
			return
		}
		for path, b := range s.TrackedFileBackups {
			h.put(path, b, cwd)
		}
	case typeDelta:
		var b backup
		if json.Unmarshal(e.Backup, &b) != nil {
			return
		}
		h.put(e.TrackingPath, b, cwd)
	}
}

func (h *history) put(path string, b backup, cwd string) {
	at, err := time.Parse(time.RFC3339Nano, b.BackupTime)
	if path == "" || err != nil {
		return
	}
	v := version{at: at}
	if b.BackupFileName != nil {
		v.name = *b.BackupFileName
	}
	if h.byPath == nil {
		h.byPath = map[string][]version{}
	}
	for _, key := range trackedKeys(path, b.RealParentDir, cwd) {
		if !slices.ContainsFunc(h.byPath[key], v.same) {
			h.byPath[key] = append(h.byPath[key], v)
		}
	}
}

func (v version) same(o version) bool {
	return v.name == o.name && v.at.Equal(o.at)
}

// trackedKeys are the absolute paths a tracked file is known by: the path
// as listed, and the physical parent the harness noted joined with its
// base name.
func trackedKeys(path, realParent, cwd string) []string {
	var keys []string
	switch {
	case filepath.IsAbs(path):
		keys = append(keys, filepath.Clean(path))
	case cwd != "":
		keys = append(keys, filepath.Join(cwd, path))
	}
	if realParent != "" {
		if k := filepath.Join(realParent, filepath.Base(path)); !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	return keys
}

// latest returns the last version of the file, under any of its keys, taken
// at or before at.
func (h *history) latest(keys []string, at time.Time) (version, bool) {
	var best version
	found := false
	for _, k := range keys {
		for _, v := range h.byPath[k] {
			if v.at.After(at) || (found && !v.at.After(best.at)) {
				continue
			}
			best, found = v, true
		}
	}
	return best, found
}

// readVersion reads one backup file of a session, refusing a name that is
// not a plain file name and content past the chunk cap.
func readVersion(dir, sessionID, name string) ([]byte, bool) {
	if dir == "" || !plainName(sessionID) || !plainName(name) {
		return nil, false
	}
	path := filepath.Join(dir, sessionID, name)
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > moondb.MaxChunkBytes {
		return nil, false
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) > moondb.MaxChunkBytes {
		return nil, false
	}
	return data, true
}

// plainName reports whether s names a file in a directory and nothing
// else, so joining it cannot climb out of that directory.
func plainName(s string) bool {
	return s != "" && s != "." && s != ".." && filepath.Base(s) == s
}

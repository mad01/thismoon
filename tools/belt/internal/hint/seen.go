package hint

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/cache"
)

// seen suppresses repeat advice within one session. Without it a hint meant
// to speak once repeats on every matching call, which is how a hint becomes
// wallpaper and stops being read.
//
// State is a file per session under the cache dir, one id per line. Every
// failure path degrades to "not seen": showing advice twice is a much
// smaller cost than a hook erroring on its own bookkeeping.
var seen = sessionSeen{}

type sessionSeen struct{}

// path names the session's seen file, "" when the session cannot have one
// (no id, no home directory, or an id with nothing path-safe in it).
func (sessionSeen) path(sessionID string) string {
	return cache.SessionFile(cache.Dir(), "seen-", sessionID)
}

func (sessionSeen) load(path string) map[string]bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]bool{}
	}
	ids := map[string]bool{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			ids[line] = true
		}
	}
	return ids
}

func (sessionSeen) record(path string, ids []string) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.WriteString(strings.Join(ids, "\n") + "\n")
}

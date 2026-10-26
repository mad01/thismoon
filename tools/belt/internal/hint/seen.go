package hint

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/cache"
)

// seen suppresses repeat advice within one session. Without it a hint repeats
// the same assertions on every search in the same area, which is how a hint
// becomes wallpaper and stops being read.
//
// State is a file per session under the cache dir, one id per line. Every
// failure path degrades to "not seen": showing an assertion twice is a much
// smaller cost than a hook erroring on its own bookkeeping.
var seen = sessionSeen{}

type sessionSeen struct{}

// path names the session's seen file, "" when the session cannot have one
// (no id, no home directory, or an id with nothing path-safe in it).
func (sessionSeen) path(sessionID string) string {
	return cache.SessionFile(cache.Dir(), "seen-", sessionID)
}

// take drops the assertions already surfaced this session, keeps the first
// limit of the rest, and records only those as surfaced. Dedupe runs before
// the cap: once the top-ranked assertions have been shown, the next ones in
// rank order surface on a later fire instead of the hint going quiet while
// relevant assertions sit unshown below the cut.
func (s sessionSeen) take(sessionID string, as []assertion, limit int) []assertion {
	path := s.path(sessionID)
	already := map[string]bool{} // no session id: dedupe is not possible, advise anyway
	if path != "" {
		already = s.load(path)
	}
	var out []assertion
	for _, a := range as {
		if len(out) == limit {
			break
		}
		if !already[a.ID] {
			out = append(out, a)
		}
	}
	if path != "" && len(out) > 0 {
		s.record(path, assertionIDs(out))
	}
	return out
}

func assertionIDs(as []assertion) []string {
	ids := make([]string, 0, len(as))
	for _, a := range as {
		ids = append(ids, a.ID)
	}
	return ids
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

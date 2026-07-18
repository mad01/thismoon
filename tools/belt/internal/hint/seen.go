package hint

import (
	"os"
	"path/filepath"
	"strings"
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

func (sessionSeen) path(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	// Session ids come from the hook payload; keep only path-safe characters
	// so a malformed id cannot escape the cache directory.
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, sessionID)
	if safe == "" {
		return ""
	}
	return filepath.Join(home, ".cache", "belt", "seen-"+safe)
}

// filter drops assertions already surfaced this session and records the rest.
func (s sessionSeen) filter(sessionID string, as []assertion) []assertion {
	path := s.path(sessionID)
	if path == "" {
		return as // no session id: dedupe is not possible, advise anyway
	}
	already := s.load(path)
	var out []assertion
	var added []string
	for _, a := range as {
		if already[a.ID] {
			continue
		}
		out = append(out, a)
		added = append(added, a.ID)
	}
	if len(added) > 0 {
		s.record(path, added)
	}
	return out
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

// Package cache names belt's per-session state files under ~/.cache/belt:
// the once-per-session hint marks, the watcher's cursor, and its owner
// token. Session ids arrive in hook payloads, so a file name built from one
// keeps only path-safe characters and a malformed id cannot escape the
// directory.
package cache

import (
	"os"
	"path/filepath"
	"strings"
)

// Dir is belt's cache directory, ~/.cache/belt, or "" when there is no home
// directory to put it under.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".cache", "belt")
}

// SessionFile names a per-session file under dir: prefix followed by the
// session id reduced to letters, digits, - and _. It returns "" when dir is
// "" or nothing of the id survives, which callers treat as "no state can be
// kept for this session" and degrade from, never error on.
func SessionFile(dir, prefix, sessionID string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return -1
		}
	}, sessionID)
	if dir == "" || safe == "" {
		return ""
	}
	return filepath.Join(dir, prefix+safe)
}

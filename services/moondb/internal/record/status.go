package record

import (
	"strings"
)

// statusEntry is one path `git status --porcelain=v2 -z` reports, with its
// two-letter status: index then worktree, "??" for an untracked file and
// "!!" for an ignored one. orig is the path a rename moved the file from.
type statusEntry struct {
	path string
	xy   string
	orig string
}

// statusFields is how many space-separated fields each entry type has, the
// path last: ordinary (1), renamed or copied (2) and unmerged (u).
var statusFields = map[byte]int{'1': 9, '2': 10, 'u': 11}

// parseStatus reads `git status --porcelain=v2 -z` output. A renamed or
// copied entry is followed by its original path as a field of its own. A
// rename's original becomes the entry's orig; it is gone from the
// worktree, so it is never an entry of its own. Ignored files appear only
// when the status was asked for them. Submodules and entries of a shape it
// does not know are skipped.
func parseStatus(out []byte) []statusEntry {
	fields := strings.Split(string(out), "\x00")
	var entries []statusEntry
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 3 || f[1] != ' ' {
			continue
		}
		if f[0] == '?' || f[0] == '!' {
			entries = append(entries, statusEntry{path: f[2:], xy: f[:1] + f[:1]})
			continue
		}
		n, known := statusFields[f[0]]
		if !known {
			continue
		}
		var orig string
		if f[0] == '2' {
			i++
			if i < len(fields) {
				orig = fields[i]
			}
		}
		parts := strings.SplitN(f, " ", n)
		if len(parts) != n || !strings.HasPrefix(parts[2], "N") || parts[n-1] == "" {
			continue
		}
		e := statusEntry{path: parts[n-1], xy: parts[1]}
		if f[0] == '2' && strings.HasPrefix(parts[8], "R") {
			e.orig = orig
		}
		entries = append(entries, e)
	}
	return entries
}

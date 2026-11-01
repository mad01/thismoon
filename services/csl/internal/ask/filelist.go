package ask

import (
	"fmt"
	"strings"
)

// noMatchesLine is what `csl search` prints on stdout when nothing matched,
// so a pipe from an empty search reads as no files, not as a bad line.
const noMatchesLine = "no matches"

// ParseFileList reads org/repo/path lines, the format `csl search` prints in
// files mode, into hits in the order given. A repo name may itself hold
// slashes, so each line splits after the longest discovered repo name that
// prefixes it. Blank lines and the "no matches" line are ignored; any other
// line that names no discovered repo is an error, since it is usually
// content-mode output piped in by mistake.
func ParseFileList(lines []string, repoNames []string) ([]Hit, error) {
	known := make(map[string]bool, len(repoNames))
	for _, n := range repoNames {
		known[n] = true
	}
	var hits []Hit
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == noMatchesLine {
			continue
		}
		h, ok := splitRepoPath(line, known)
		if !ok {
			return nil, fmt.Errorf(
				"line %d %q names no discovered repo; the file list takes org/repo/path lines, "+
					"the format csl search prints without -o content",
				i+1, line,
			)
		}
		hits = append(hits, h)
	}
	return hits, nil
}

// splitRepoPath splits line after the longest known repo name followed by a
// slash and at least one more character.
func splitRepoPath(line string, known map[string]bool) (Hit, bool) {
	for i := strings.LastIndexByte(line, '/'); i > 0; i = strings.LastIndexByte(line[:i], '/') {
		if known[line[:i]] && i+1 < len(line) {
			return Hit{Repo: line[:i], Path: line[i+1:]}, true
		}
	}
	return Hit{}, false
}

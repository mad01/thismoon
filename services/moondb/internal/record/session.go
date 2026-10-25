package record

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/services/moondb/internal/store"
)

// Session state file suffixes and permissions (formats in the package doc).
const (
	turnExt       = ".turn"
	pathsExt      = ".paths"
	touchedExt    = ".touched"
	headsExt      = ".heads"
	stateDirPerm  = 0o700
	stateFilePerm = 0o600
)

// sessionState is one session's state under <workdir>/sessions. Every
// method is best effort: state that cannot be read reads as empty, and a
// write that fails costs a later event some context (a HEAD chunk, a
// transcript tail, a head-moved report), never the event being recorded.
// Hooks of one session run in parallel (subagents share the session id),
// so additions are single appended lines, which O_APPEND keeps whole; a
// line a race duplicated reads the same as one.
type sessionState struct {
	dir string
	id  string
}

// openTurn remembers the open turn and the transcript size it opened at.
func (s sessionState) openTurn(promptID string, offset int64) {
	s.replace(turnExt, fmt.Sprintf("%s %d\n", promptID, offset))
}

// turnOffset returns the transcript size the turn promptID opened at, and
// false when its open was not recorded.
func (s sessionState) turnOffset(promptID string) (int64, bool) {
	lines := s.lines(turnExt)
	if len(lines) == 0 {
		return 0, false
	}
	id, offset, ok := strings.Cut(lines[0], " ")
	if !ok || id != promptID {
		return 0, false
	}
	n, err := strconv.ParseInt(offset, 10, 64)
	if err != nil || n < 0 {
		return 0, false
	}
	return n, true
}

// edited reports whether the session has edited path in worktree before.
func (s sessionState) edited(worktree, path string) bool {
	return slices.Contains(s.lines(pathsExt), worktree+"\t"+path)
}

// noteEdit records an edit of path in loc's worktree. The first edit in a
// worktree also marks it touched and records its HEAD, so a commit made
// later in the same turn shows up as a moved HEAD at the close.
func (s sessionState) noteEdit(loc store.Location, path string) {
	if !s.edited(loc.Worktree, path) {
		s.appendLine(pathsExt, loc.Worktree+"\t"+path)
	}
	if loc.Worktree == "" || slices.Contains(s.touched(), loc.Worktree) {
		return
	}
	s.appendLine(touchedExt, loc.Worktree)
	if _, known := s.heads()[loc.Worktree]; !known && loc.Head != "" {
		s.appendLine(headsExt, loc.Worktree+"\t"+loc.Head)
	}
}

// touched lists the worktrees the session edited in, each once.
func (s sessionState) touched() []string {
	var out []string
	for _, wt := range s.lines(touchedExt) {
		if !slices.Contains(out, wt) {
			out = append(out, wt)
		}
	}
	return out
}

// heads maps each touched worktree to the HEAD last seen there.
func (s sessionState) heads() map[string]string {
	heads := map[string]string{}
	for _, line := range s.lines(headsExt) {
		// The head has no tab; the worktree path might.
		if i := strings.LastIndexByte(line, '\t'); i > 0 {
			heads[line[:i]] = line[i+1:]
		}
	}
	return heads
}

// saveHeads replaces the recorded HEADs with heads.
func (s sessionState) saveHeads(heads map[string]string) {
	var b strings.Builder
	for _, wt := range slices.Sorted(maps.Keys(heads)) {
		fmt.Fprintf(&b, "%s\t%s\n", wt, heads[wt])
	}
	s.replace(headsExt, b.String())
}

func (s sessionState) file(ext string) string {
	return filepath.Join(s.dir, safeName(s.id)+ext)
}

func (s sessionState) lines(ext string) []string {
	data, err := os.ReadFile(s.file(ext))
	if err != nil {
		return nil
	}
	return strings.FieldsFunc(string(data), func(r rune) bool { return r == '\n' })
}

func (s sessionState) appendLine(ext, line string) {
	if os.MkdirAll(s.dir, stateDirPerm) != nil {
		return
	}
	f, err := os.OpenFile(s.file(ext), os.O_WRONLY|os.O_APPEND|os.O_CREATE, stateFilePerm)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(line + "\n")
}

// replace swaps the file's content in one rename, so a reader in another
// hook sees the old state or the new, never half of it.
func (s sessionState) replace(ext, body string) {
	if os.MkdirAll(s.dir, stateDirPerm) != nil {
		return
	}
	f, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		return
	}
	_, werr := f.WriteString(body)
	cerr := f.Close()
	if werr != nil || cerr != nil || os.Rename(f.Name(), s.file(ext)) != nil {
		_ = os.Remove(f.Name())
	}
}

// safeName makes a session id safe for a file name: every byte outside
// [A-Za-z0-9_-] becomes '_', so no id can climb out of the directory.
func safeName(id string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '_'
	}, id)
}

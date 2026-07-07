package store

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// DetectRepo returns the git repository name for cwd (the basename of the work
// tree root), or "" when cwd is not inside a git repo — e.g. a tmp dir. That
// empty case is expected: such checkpoints land in CONTEXT.md, not a repo note.
func DetectRepo(cwd string) string {
	if cwd == "" {
		return ""
	}
	out, err := exec.Command("git", "-C", cwd, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimSpace(string(out)))
}

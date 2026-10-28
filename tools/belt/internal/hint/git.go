package hint

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// nearestDir walks up from the session cwd to the closest directory that
// exists, mirroring the write-internal-names guard: a stale cwd must degrade
// to silence, not an exec error.
func nearestDir(path string) string {
	dir := path
	for dir != "" {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return dir
		}
		dir = parent
	}
	return dir
}

// gitTopLevel shells out to git; "" when dir is outside a repo working tree.
func gitTopLevel(dir string) string {
	return gitLine(dir, "rev-parse", "--show-toplevel")
}

func gitLine(dir string, args ...string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

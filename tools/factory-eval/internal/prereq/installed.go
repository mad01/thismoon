package prereq

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Installed is the provenance of the factory pieces on the host when a run
// started: the sha256 of each installed skill and worker definition, keyed
// by its path under the home directory, and the belt version. A ralph up
// between two runs of a batch shows as a changed value.
type Installed struct {
	Files       map[string]string `json:"files"`
	BeltVersion string            `json:"belt_version"`
}

// installedFiles are the files Snapshot hashes, relative to the home dir.
func installedFiles() []string {
	files := []string{".claude/skills/factory/SKILL.md", ".claude/skills/loom/SKILL.md"}
	for _, w := range WorkerDefinitions {
		files = append(files, ".claude/agents/"+w+".md")
	}
	return files
}

// Snapshot hashes the installed factory pieces and asks belt its version.
// A file that cannot be read records why instead of a hash, so a missing
// piece is visible in the results rather than an error that stops a run.
func Snapshot(ctx context.Context, env Env) Installed {
	in := Installed{Files: map[string]string{}}
	for _, rel := range installedFiles() {
		in.Files["~/"+rel] = hashFile(filepath.Join(env.Home, rel))
	}
	v, err := env.BeltVersion(ctx)
	if err != nil {
		in.BeltVersion = "unavailable: " + err.Error()
	} else {
		in.BeltVersion = strings.TrimSpace(v)
	}
	return in
}

func hashFile(p string) string {
	raw, err := os.ReadFile(p)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "missing"
	case err != nil:
		return "unreadable: " + err.Error()
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

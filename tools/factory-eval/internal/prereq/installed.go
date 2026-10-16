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
// started: the sha256 of every file in the installed factory and loom
// skill dirs and of each worker definition, keyed by its path under the
// home directory, and the belt version. A ralph up between two runs of a
// batch shows as a changed value.
type Installed struct {
	Files       map[string]string `json:"files"`
	BeltVersion string            `json:"belt_version"`
}

// skillDirs are the skill dirs Snapshot hashes file by file, relative to
// the home dir. Every file there can reach the orchestrator or a worker,
// such as the factory skill's worker brief and routing table.
var skillDirs = []string{".claude/skills/factory", ".claude/skills/loom"}

// evalsDir is a skill's eval suite. Snapshot skips it: a run never reads
// it, and an eval run writes its results into it.
const evalsDir = "evals"

// Snapshot hashes the installed factory pieces and asks belt its version.
// A file that cannot be read records why instead of a hash, so a missing
// piece is visible in the results rather than an error that stops a run.
func Snapshot(ctx context.Context, env Env) Installed {
	in := Installed{Files: map[string]string{}}
	for _, rel := range skillDirs {
		hashDir(env.Home, rel, in.Files)
	}
	for _, w := range WorkerDefinitions {
		rel := ".claude/agents/" + w + ".md"
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

// hashDir records the hash of every file under home/rel in files, keyed
// by its path under the home dir, skipping the eval suite and dotfiles.
// The dir is resolved first, since skills are installed as symlinks. A
// dir that is missing or cannot be walked records why under its own path.
func hashDir(home, rel string, files map[string]string) {
	key := "~/" + rel + "/"
	root, err := filepath.EvalSymlinks(filepath.Join(home, rel))
	if err != nil {
		files[key] = describe(err)
		return
	}
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case p != root && strings.HasPrefix(d.Name(), "."):
			return skip(d)
		case d.IsDir() && d.Name() == evalsDir && filepath.Dir(p) == root:
			return fs.SkipDir
		case d.IsDir():
			return nil
		}
		sub, _ := filepath.Rel(root, p)
		files[key+filepath.ToSlash(sub)] = hashFile(p)
		return nil
	})
	if err != nil {
		files[key] = describe(err)
	}
}

// skip leaves out a dir entry the walk should not hash: a whole dir, or
// one file.
func skip(d fs.DirEntry) error {
	if d.IsDir() {
		return fs.SkipDir
	}
	return nil
}

func describe(err error) string {
	if errors.Is(err, fs.ErrNotExist) {
		return "missing"
	}
	return "unreadable: " + err.Error()
}

func hashFile(p string) string {
	raw, err := os.ReadFile(p)
	if err != nil {
		return describe(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

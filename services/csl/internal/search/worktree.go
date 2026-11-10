package search

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/csl"
)

// worktreeIndexDirName is the directory inside csl's state directory that
// holds one lexical index per linked git worktree. It sits beside the main
// index, never inside it: the daemon and every fleet-wide search open the main
// index only, so a worktree index is searched only by a call that names its
// worktree, and the whole tree can be deleted without losing anything.
const worktreeIndexDirName = "worktree-index"

// abandonedBuildAge is how old a worktree index directory that publishes
// no generation must be before PruneWorktreeIndexes removes it. A first
// build publishes its generation last, so a younger one may still be
// building.
const abandonedBuildAge = time.Hour

// WorktreeIndexRoot returns the directory holding every worktree index.
func WorktreeIndexRoot() (string, error) {
	return csl.StatePath(worktreeIndexDirName)
}

// WorktreeIndexDir returns the index directory under root for the worktree
// checked out at path: the worktree directory's base name, so a reader
// listing root can tell them apart, then a hash of the whole path, so two
// worktrees that share a base name never share an index. Leading dots are
// dropped from the base name, so no index ever reads as a tombstone.
func WorktreeIndexDir(root, path string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(path)))
	base := strings.TrimLeft(filepath.Base(path), ".")
	return filepath.Join(root, fmt.Sprintf("%s-%x", base, sum[:4]))
}

// PruneWorktreeIndexes removes the worktree indexes under root that are not
// worth keeping. An index whose worktree is gone goes: each path its current
// generation's state.json records has no .git entry left. So does a
// directory that publishes no generation and is older than
// abandonedBuildAge, the remains of a first build that never finished, and
// any tombstone a crash left. Past maxWorktreeIndexes the least recently
// used go too. An index a caller holds a lease on (LeaseWorktreeIndex) is
// never removed. It returns the removed index directories. A missing root
// is not an error.
func PruneWorktreeIndexes(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read worktree index root %s: %w", root, err)
	}
	var removed []string
	var kept []indexUse
	for _, e := range entries {
		dir := filepath.Join(root, e.Name())
		if strings.HasPrefix(e.Name(), tombstonePrefix) {
			if err := removeUnder(root, dir); err != nil {
				return removed, err
			}
			continue
		}
		if !e.IsDir() {
			continue
		}
		if worktreeIndexGone(dir) {
			ok, err := removeUnleased(root, dir)
			if err != nil {
				return removed, err
			}
			if ok {
				removed = append(removed, dir)
				continue
			}
		}
		kept = append(kept, indexUse{dir: dir, lastUse: lastUse(dir)})
	}
	evicted, err := evictLeastRecentlyUsed(root, kept, maxWorktreeIndexes)
	return append(removed, evicted...), err
}

// worktreeIndexGone reports whether the worktree index in dir has outlived
// its worktree, or is an abandoned first build.
func worktreeIndexGone(dir string) bool {
	name, err := currentGeneration(dir)
	var state *IndexState
	if err == nil && name != "" {
		state, err = LoadState(filepath.Join(dir, name))
	}
	if err != nil || state == nil || len(state.Repos) == 0 {
		info, statErr := os.Stat(dir)
		return statErr == nil && time.Since(info.ModTime()) > abandonedBuildAge
	}
	for path := range state.Repos {
		if _, err := os.Lstat(filepath.Join(path, ".git")); err == nil {
			return false
		}
	}
	return true
}

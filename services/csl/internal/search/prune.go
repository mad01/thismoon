package search

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/sourcegraph/zoekt/index"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// PruneOrphans removes the index entries of every repo state.json records
// whose path keep does not hold: its state entry, and each zoekt shard whose
// repositories were all indexed from such a path. A shard is attributed by the
// source path IndexRepo writes into its metadata, never by its file name, so a
// kept checkout that shares an org/repo name with a pruned one keeps its
// shard. A shard that cannot be read is left alone (RepairIndex owns those).
// A deleted shard takes its ".meta" sidecar with it. It returns the deleted
// shard paths.
//
// The caller holds the sync lock. Shards go first and state.json is loaded
// fresh from disk and saved last, so a failed delete leaves the entry for the
// next run to retry. A running search daemon stops serving a deleted shard on
// its own: zoekt's directory searcher watches the index directory.
func PruneOrphans(indexDir string, keep []finder.Repo) ([]string, error) {
	state, err := LoadState(indexDir)
	if err != nil {
		return nil, err
	}
	orphans := orphanPaths(state, keep)
	if len(orphans) == 0 {
		return nil, nil
	}

	removed, err := removeOrphanShards(indexDir, orphans)
	if err != nil {
		return removed, err
	}

	state.mu.Lock()
	for path := range orphans {
		delete(state.Repos, path)
	}
	state.mu.Unlock()
	if err := state.Save(indexDir); err != nil {
		return removed, fmt.Errorf("save pruned index state: %w", err)
	}
	return removed, nil
}

// orphanPaths returns the repo paths state records that keep does not hold.
func orphanPaths(state *IndexState, keep []finder.Repo) map[string]bool {
	kept := make(map[string]bool, len(keep))
	for _, r := range keep {
		kept[r.Path] = true
	}

	state.mu.RLock()
	defer state.mu.RUnlock()
	orphans := make(map[string]bool)
	for path := range state.Repos {
		if !kept[path] {
			orphans[path] = true
		}
	}
	return orphans
}

// removeOrphanShards deletes every shard in indexDir attributed to an orphan
// path and returns the deleted shard paths.
func removeOrphanShards(indexDir string, orphans map[string]bool) ([]string, error) {
	entries, err := os.ReadDir(indexDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read index dir %s: %w", indexDir, err)
	}

	var removed []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".zoekt") {
			continue
		}
		shard := filepath.Join(indexDir, e.Name())
		if !shardFromOrphans(shard, orphans) {
			continue
		}
		if err := removeShard(shard); err != nil {
			return removed, err
		}
		removed = append(removed, shard)
	}
	return removed, nil
}

// shardFromOrphans reports whether every repository a shard holds was indexed
// from an orphan path. An unreadable shard, or one holding no repository, is
// never attributed.
func shardFromOrphans(shard string, orphans map[string]bool) bool {
	repos, _, err := index.ReadMetadataPath(shard)
	if err != nil || len(repos) == 0 {
		return false
	}
	for _, r := range repos {
		if !orphans[r.Source] {
			return false
		}
	}
	return true
}

// removeShard deletes a shard's ".meta" sidecar, when one exists, and then the
// shard. The sidecar goes first so a failure part way never leaves one behind
// without the shard it overrides.
func removeShard(shard string) error {
	for _, p := range []string{shard + ".meta", shard} {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove shard file %s: %w", p, err)
		}
	}
	return nil
}

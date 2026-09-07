package search

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// healthSnapshotName is the git-health snapshot file inside csl's state dir.
const healthSnapshotName = "health.json"

// HealthSnapshot is a persisted git-health sweep: the entries plus when they
// were computed. It is written to the state dir so every csl surface — the web
// /health view and the MCP repo-health tool — can serve a recent sweep instead
// of re-walking every repo. A full sweep spawns git subprocesses per repo, so
// it grows costly as the number of checkouts does; the snapshot bounds that
// cost to once per TTL across all surfaces.
type HealthSnapshot struct {
	ComputedAt time.Time   `json:"computed_at"`
	Entries    []GitHealth `json:"entries"`
}

// healthSnapshotPath resolves the snapshot file under csl's state dir.
func healthSnapshotPath() (string, error) {
	return csl.StatePath(healthSnapshotName)
}

// LoadHealthSnapshot reads the persisted sweep. ok is false when no usable
// snapshot exists yet (first run, or an unreadable/partial file).
func LoadHealthSnapshot() (HealthSnapshot, bool) {
	path, err := healthSnapshotPath()
	if err != nil {
		return HealthSnapshot{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return HealthSnapshot{}, false
	}
	var s HealthSnapshot
	if err := json.Unmarshal(data, &s); err != nil || s.ComputedAt.IsZero() {
		return HealthSnapshot{}, false
	}
	return s, true
}

// SaveHealthSnapshot persists a sweep atomically (temp file + rename) so a
// concurrent reader never observes a half-written file.
func SaveHealthSnapshot(s HealthSnapshot) error {
	path, err := healthSnapshotPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), healthSnapshotName+".*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// CachedGitHealthSweep returns a sweep no older than ttl: the persisted snapshot
// when it is still fresh, otherwise a newly computed sweep that it also
// persists. It blocks on the sweep when a recompute is needed, so callers that
// must never block (the web UI) should read LoadHealthSnapshot and recompute in
// the background instead. A non-positive ttl always recomputes.
func CachedGitHealthSweep(
	ctx context.Context,
	repos []finder.Repo,
	ttl time.Duration,
) HealthSnapshot {
	if ttl > 0 {
		if s, ok := LoadHealthSnapshot(); ok && time.Since(s.ComputedAt) < ttl {
			return s
		}
	}
	s := HealthSnapshot{ComputedAt: time.Now(), Entries: GitHealthSweep(ctx, repos)}
	_ = SaveHealthSnapshot(s)
	return s
}

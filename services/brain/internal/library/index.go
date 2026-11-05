package library

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"time"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/chunk"
	"github.com/mad01/thismoon/services/brain/internal/lockfile"
	"github.com/mad01/thismoon/services/brain/internal/search"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// healthTimeout bounds the embedder probe before a semantic refresh.
const healthTimeout = 5 * time.Second

// Refresh reports one index check of one vault.
type Refresh struct {
	// Vault is the vault's name.
	Vault string
	// Built is true when the index was rebuilt; false means it was fresh.
	Built bool
	// Notes counts the notes indexed when Built.
	Notes int
	// Chunks counts the chunks embedded by a semantic refresh.
	Chunks int
	// Removed counts the vanished notes a semantic refresh purged.
	Removed int
}

// RefreshLiteral rebuilds the vault's literal index when the vault changed
// since it was built, or always when force is set. The build holds the
// vault's literal lock, so two brain processes never build one index at once;
// a process that waited re-checks and skips a build the other just did.
// progress, when set, sees each indexed note's path.
func (l *Library) RefreshLiteral(
	v *vault.Vault,
	force bool,
	progress func(path string),
) (res Refresh, err error) {
	res.Vault = v.Name
	dir, err := brain.IndexDir(v.Name)
	if err != nil {
		return res, err
	}
	if fresh, err := literalFresh(v, dir); err != nil || (fresh && !force) {
		return res, err
	}
	release, err := l.lock(v, lockLiteral)
	if err != nil {
		return res, err
	}
	defer func() { err = errors.Join(err, release()) }()

	fingerprint, files, err := search.ComputeFingerprint(v.Root)
	if err != nil {
		return res, fmt.Errorf("fingerprint %s: %w", v.Name, err)
	}
	state, err := search.LoadState(dir)
	if err != nil {
		return res, err
	}
	if !force && !search.IsStale(state, fingerprint) {
		return res, nil
	}
	target := search.Target{Name: v.Name, Root: v.Root, IndexDir: dir}
	err = search.IndexVault(target, func(path string) {
		res.Notes++
		if progress != nil {
			progress(path)
		}
	})
	if err != nil {
		return res, err
	}
	state.SetFingerprint(fingerprint, files, time.Now().UTC())
	if err := state.Save(dir); err != nil {
		return res, err
	}
	res.Built = true
	return res, nil
}

// literalFresh reports whether the literal index in dir matches the vault.
func literalFresh(v *vault.Vault, dir string) (bool, error) {
	fingerprint, _, err := search.ComputeFingerprint(v.Root)
	if err != nil {
		return false, fmt.Errorf("fingerprint %s: %w", v.Name, err)
	}
	state, err := search.LoadState(dir)
	if err != nil {
		return false, err
	}
	return !search.IsStale(state, fingerprint), nil
}

// literalDir refreshes the vault's literal index when stale and returns its
// directory.
func (l *Library) literalDir(v *vault.Vault) (string, error) {
	if _, err := l.RefreshLiteral(v, false, nil); err != nil {
		return "", err
	}
	return brain.IndexDir(v.Name)
}

// SemanticStale reports whether the vault changed since its semantic index
// was synced, or the index was made with other settings.
func (l *Library) SemanticStale(v *vault.Vault) (bool, error) {
	dir, err := l.cfg.Semantic.ResolvedIndexDir(v.Name)
	if err != nil {
		return false, err
	}
	fingerprint, _, err := search.ComputeFingerprint(v.Root)
	if err != nil {
		return false, fmt.Errorf("fingerprint %s: %w", v.Name, err)
	}
	m, _, err := semantic.LoadManifest(dir)
	if err != nil {
		return false, err
	}
	return !m.Fresh(fingerprint, l.semanticSettings()), nil
}

// RefreshSemantic syncs the vault's semantic index when the vault changed
// since the last sync, or rebuilds it when force is set, under the vault's
// semantic lock. A sync re-embeds only new and changed notes and purges the
// chunks of vanished ones. An index made with other settings, or one with
// chunks and no manifest (built before manifests existed), is rebuilt.
// progress, when set, sees each re-embedded note and its chunk count.
func (l *Library) RefreshSemantic(
	ctx context.Context,
	v *vault.Vault,
	force bool,
	progress func(path string, chunks int),
) (res Refresh, err error) {
	res.Vault = v.Name
	stale, err := l.SemanticStale(v)
	if err != nil || (!stale && !force) {
		return res, err
	}
	release, err := l.lock(v, lockSemantic)
	if err != nil {
		return res, err
	}
	defer func() { err = errors.Join(err, release()) }()

	dir, err := l.cfg.Semantic.ResolvedIndexDir(v.Name)
	if err != nil {
		return res, err
	}
	fingerprint, _, err := search.ComputeFingerprint(v.Root)
	if err != nil {
		return res, fmt.Errorf("fingerprint %s: %w", v.Name, err)
	}
	m, found, err := semantic.LoadManifest(dir)
	if err != nil {
		return res, err
	}
	settings := l.semanticSettings()
	if !force && m.Fresh(fingerprint, settings) {
		return res, nil
	}
	idx, err := l.openSemantic(v)
	if err != nil {
		return res, fmt.Errorf("open semantic index of %s: %w", v.Name, err)
	}
	held := m.Notes
	if force || (found && !m.Compatible(settings)) || (!found && idx.Count() > 0) {
		if err := idx.Reset(ctx); err != nil {
			return res, err
		}
		held = nil
	}
	report, err := semantic.Sync(ctx, idx, v.Root, semantic.SyncOptions{
		Chunk:    l.chunkOptions(),
		Held:     held,
		Progress: progress,
	})
	if err != nil {
		return res, fmt.Errorf("semantic index of %s: %w", v.Name, err)
	}
	m = semantic.Manifest{
		Version:     semantic.ManifestVersion,
		Settings:    settings,
		Fingerprint: fingerprint,
		IndexedAt:   time.Now().UTC(),
		Notes:       report.Notes,
	}
	if err := m.Save(dir); err != nil {
		return res, err
	}
	res.Built, res.Notes, res.Chunks, res.Removed = true, report.Embedded, report.Chunks, report.Removed
	return res, nil
}

func (l *Library) chunkOptions() chunk.Options {
	return chunk.FromTokens(l.cfg.Semantic.ChunkSize, l.cfg.Semantic.ChunkOverlap)
}

func (l *Library) semanticSettings() semantic.Settings {
	return semantic.SettingsFor(l.cfg.Semantic.EmbedModel, l.chunkOptions())
}

// Health probes the embedder the semantic layer uses. A library opened with
// a custom embedder and no health probe always answers nil.
func (l *Library) Health(ctx context.Context) error {
	if l.health == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, healthTimeout)
	defer cancel()
	return l.health(ctx)
}

// refreshSemantic refreshes the stale semantic indexes of vaults before a
// query. When the embedder does not answer, the stale indexes stay as they
// are and each one adds a warning instead of failing the query.
func (l *Library) refreshSemantic(ctx context.Context, vaults *vault.Set) ([]string, error) {
	var (
		warnings  []string
		probed    bool
		healthErr error
	)
	for _, v := range vaults.Vaults() {
		stale, err := l.SemanticStale(v)
		if err != nil {
			return warnings, err
		}
		if !stale {
			continue
		}
		if !probed {
			healthErr, probed = l.Health(ctx), true
		}
		if healthErr != nil {
			warnings = append(warnings, fmt.Sprintf(
				"%s: semantic index is stale and was not refreshed: %v", v.Name, healthErr,
			))
			continue
		}
		if _, err := l.RefreshSemantic(ctx, v, false, nil); err != nil {
			return warnings, err
		}
	}
	return warnings, nil
}

// Purge drops a withdrawn note from the vault's indexes now instead of at
// the next query: the literal index is rebuilt from the vault, and the
// note's chunks leave the semantic index. The semantic purge needs no
// embedder, so it works with Ollama down, and the next sync skips the
// tombstone (see semantic.Sync).
func (l *Library) Purge(ctx context.Context, v *vault.Vault, rel string) error {
	if _, err := l.RefreshLiteral(v, false, nil); err != nil {
		return err
	}
	return l.purgeSemantic(ctx, v, rel)
}

// purgeSemantic deletes the note's chunks and its manifest entry, under the
// vault's semantic lock. A vault with no semantic index has nothing to purge.
func (l *Library) purgeSemantic(ctx context.Context, v *vault.Vault, rel string) (err error) {
	dir, err := l.cfg.Semantic.ResolvedIndexDir(v.Name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	release, err := l.lock(v, lockSemantic)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, release()) }()
	idx, err := l.openSemantic(v)
	if err != nil {
		return fmt.Errorf("open semantic index of %s: %w", v.Name, err)
	}
	if err := idx.DeleteByPath(ctx, rel); err != nil {
		return err
	}
	m, found, err := semantic.LoadManifest(dir)
	if err != nil || !found {
		return err
	}
	delete(m.Notes, rel)
	return m.Save(dir)
}

// Lock kinds. The literal and semantic indexes of a vault build under
// separate locks, so a long embedding run never holds up a search.
const (
	lockLiteral  = "literal"
	lockSemantic = "semantic"
)

// lock takes one of the vault's index locks.
func (l *Library) lock(v *vault.Vault, kind string) (func() error, error) {
	path, err := lockPath(v, kind)
	if err != nil {
		return nil, err
	}
	return lockfile.Hold(path)
}

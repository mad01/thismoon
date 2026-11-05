package library

import (
	"context"
	"errors"
	"fmt"
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
}

// RefreshLiteral rebuilds the vault's literal index when the vault changed
// since it was built, or always when force is set. The build holds the
// vault's index lock, so two brain processes never build one index at once;
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
	release, err := l.lock(v)
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
// was built.
func (l *Library) SemanticStale(v *vault.Vault) (bool, error) {
	dir, err := brain.IndexDir(v.Name)
	if err != nil {
		return false, err
	}
	fingerprint, _, err := search.ComputeFingerprint(v.Root)
	if err != nil {
		return false, fmt.Errorf("fingerprint %s: %w", v.Name, err)
	}
	state, err := search.LoadState(dir)
	if err != nil {
		return false, err
	}
	return state.IsSemanticStale(fingerprint), nil
}

// RefreshSemantic re-embeds the vault into its semantic index when the
// vault changed since the last build, or always when force is set, under the
// vault's index lock. progress, when set, sees each note and its chunk count.
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
	release, err := l.lock(v)
	if err != nil {
		return res, err
	}
	defer func() { err = errors.Join(err, release()) }()

	dir, err := brain.IndexDir(v.Name)
	if err != nil {
		return res, err
	}
	fingerprint, _, err := search.ComputeFingerprint(v.Root)
	if err != nil {
		return res, fmt.Errorf("fingerprint %s: %w", v.Name, err)
	}
	state, err := search.LoadState(dir)
	if err != nil {
		return res, err
	}
	if !force && !state.IsSemanticStale(fingerprint) {
		return res, nil
	}
	idx, err := l.openSemantic(v)
	if err != nil {
		return res, fmt.Errorf("open semantic index of %s: %w", v.Name, err)
	}
	if force {
		if err := idx.Reset(ctx); err != nil {
			return res, err
		}
	}
	opts := chunk.FromTokens(l.cfg.Semantic.ChunkSize, l.cfg.Semantic.ChunkOverlap)
	err = semantic.IndexVault(ctx, idx, v.Root, opts, func(path string, n int) {
		res.Notes++
		res.Chunks += n
		if progress != nil {
			progress(path, n)
		}
	})
	if err != nil {
		return res, fmt.Errorf("semantic index of %s: %w", v.Name, err)
	}
	state.SetSemanticFingerprint(fingerprint, time.Now().UTC())
	if err := state.Save(dir); err != nil {
		return res, err
	}
	res.Built = true
	return res, nil
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

// lock takes the vault's index lock.
func (l *Library) lock(v *vault.Vault) (func() error, error) {
	path, err := lockPath(v, "index")
	if err != nil {
		return nil, err
	}
	return lockfile.Hold(path)
}

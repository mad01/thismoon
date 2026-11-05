// Package library opens the configured vaults together with their indexes.
// It is the layer the CLI and the MCP server share: it locates notes across
// vaults, keeps each vault's literal and semantic index fresh under a
// cross-process lock, and runs search, count, semantic and hybrid queries
// across vaults with every hit tagged by its vault.
package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/mad01/thismoon/services/brain"
	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/embedding"
	"github.com/mad01/thismoon/services/brain/internal/semantic"
	"github.com/mad01/thismoon/services/brain/internal/vault"
)

// ErrSemanticDisabled is returned by the semantic and hybrid queries when the
// config leaves the semantic layer off.
var ErrSemanticDisabled = errors.New(
	"semantic.enabled is false in the brain config, so the semantic layer is disabled",
)

// Options configures Open.
type Options struct {
	// Config is the loaded brain config.
	Config *config.Config
	// Embed vectorizes chunks and queries. Nil uses the Ollama the config
	// names.
	Embed semantic.EmbedFunc
	// Health probes the embedder before a semantic refresh. Nil probes the
	// configured Ollama when Embed is nil too, and skips the probe when a
	// custom Embed is given.
	Health func(context.Context) error
}

// Library is the configured vaults, opened, with their index locations.
type Library struct {
	cfg    *config.Config
	set    *vault.Set
	embed  semantic.EmbedFunc
	health func(context.Context) error
}

// Open opens every configured vault. A vault directory that does not exist
// fails the call, naming the vault, so a broken config is never half used.
func Open(opts Options) (*Library, error) {
	cfg := opts.Config
	vaults := make([]*vault.Vault, 0, len(cfg.Vaults))
	for _, c := range cfg.Vaults {
		v, err := vault.Open(c.Path)
		if err != nil {
			return nil, fmt.Errorf("vault %s: %w", c.Name, err)
		}
		v.Name, v.Shared, v.Default = c.Name, c.Shared, c.Default
		if v.WriteLock, err = lockPath(v, "write"); err != nil {
			return nil, err
		}
		vaults = append(vaults, v)
	}
	l := &Library{
		cfg:    cfg,
		set:    vault.NewSet(vaults...),
		embed:  opts.Embed,
		health: opts.Health,
	}
	if l.embed == nil {
		client := embedding.New(cfg.Semantic.OllamaURL, cfg.Semantic.EmbedModel)
		l.embed = client.EmbedFunc()
		if l.health == nil {
			l.health = client.Health
		}
	}
	return l, nil
}

// Config returns the config the library was opened with.
func (l *Library) Config() *config.Config { return l.cfg }

// Set returns every configured vault.
func (l *Library) Set() *vault.Set { return l.set }

// Vaults returns the named vaults, or every vault when names is empty.
func (l *Library) Vaults(names []string) (*vault.Set, error) {
	return l.set.Filter(names)
}

// Locate resolves a note ref (vault:path or a bare path) within the named
// vaults, or within every vault when names is empty.
func (l *Library) Locate(names []string, ref string) (vault.Ref, error) {
	set, err := l.set.Filter(names)
	if err != nil {
		return vault.Ref{}, err
	}
	return set.Locate(ref)
}

// WriteTarget returns the vault a new note goes to: the named vault, or the
// default vault when name is empty.
func (l *Library) WriteTarget(name string) (*vault.Vault, error) {
	if name == "" {
		return l.set.Default()
	}
	return l.set.Get(name)
}

// roots maps every vault name to its root, for reading notes behind hits.
func (l *Library) roots() map[string]string {
	out := make(map[string]string, len(l.set.Vaults()))
	for _, v := range l.set.Vaults() {
		out[v.Name] = v.Root
	}
	return out
}

// lockPath returns the lock file guarding one kind of work on a vault.
func lockPath(v *vault.Vault, kind string) (string, error) {
	dir, err := brain.LockDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, v.Name+"."+kind+".lock"), nil
}

// openSemantic opens the vault's chromem-go index.
func (l *Library) openSemantic(v *vault.Vault) (*semantic.Index, error) {
	dir, err := l.cfg.Semantic.ResolvedIndexDir(v.Name)
	if err != nil {
		return nil, err
	}
	return semantic.Open(dir, l.embed)
}

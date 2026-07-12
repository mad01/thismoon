package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

const configFileName = "config.yaml"

// Layout constants for session window arrangement.
const (
	LayoutSplit = "split"
	LayoutTab   = "tab"
)

// Config holds the repo finder configuration.
type Config struct {
	Dirs     []string       `yaml:"dirs"`
	Layout   string         `yaml:"layout"`
	Summary  bool           `yaml:"summary"`
	TmpDir   string         `yaml:"tmpdir"`
	Hooks    HooksConfig    `yaml:"hooks"`
	Sync     SyncConfig     `yaml:"sync"`
	Index    IndexConfig    `yaml:"index"`
	Semantic SemanticConfig `yaml:"semantic"`
	Daemon   DaemonConfig   `yaml:"daemon"`
}

// DaemonConfig controls the background search daemon.
type DaemonConfig struct {
	// IdleTimeoutMinutes is how long the daemon stays alive with no queries
	// before exiting. Higithostr values keep the zoekt shards and semantic stores
	// warm at the cost of resident memory. Zero or negative means the default
	// (10 minutes).
	IdleTimeoutMinutes int `yaml:"idle_timeout_minutes"`
}

// SemanticConfig controls the optional semantic (vector) search path. It is
// independent of the lexical zoekt index and off by default — building the
// embedding index (`csl index --semantic-all`) and loading the embedding model
// into the daemon only happen when explicitly opted in.
type SemanticConfig struct {
	// Enabled gates whether the search daemon loads the semantic index and
	// embedding model at startup. When false the daemon serves lexical search
	// only; the in-process CLI/MCP/web paths still work for ad-hoc queries.
	Enabled bool `yaml:"enabled"`
	// Sync controls whether `csl sync` also re-embeds the changed files of
	// changed repos (incrementally, best-effort). Default false: sync stays
	// lexical-only and embeddings refresh via the manual `csl index --semantic`
	// command. When true, the semantic pass runs after the lexical reindex and
	// never fails the sync if the model or backend is unavailable.
	Sync bool `yaml:"sync"`
	// OllamaURL is the base URL of the Ollama server that serves the embedding
	// model. Empty means http://localhost:11434.
	OllamaURL string `yaml:"ollama_url"`
	// EmbedModel is the Ollama embedding model. Empty means the compiled-in
	// default (jina-code-v2, see semantic.defaultOllamaModel).
	// Changing the model (or its dimensionality) triggers a full re-embed of
	// every store on the next index run.
	EmbedModel string `yaml:"embed_model"`
	// Dim is the embedding dimensionality of EmbedModel. Zero means 1024 (the
	// default model). Must match the model — stores are compared
	// against it to detect model swaps.
	Dim int `yaml:"dim"`
}

// IndexConfig holds configuration that controls which repos are included in the search index.
type IndexConfig struct {
	// Hosts is an allowlist of git hostnames. When non-empty, only repos whose
	// remote origin hostname matches one of the listed values are indexed.
	// An empty list means all repos are included (no filtering).
	// Example: ["github.com", "githost.example.com"]
	Hosts []string `yaml:"hosts"`
}

// SyncConfig holds configuration for `csl sync`.
type SyncConfig struct {
	Concurrency int `yaml:"concurrency"`
}

// EffectiveConcurrency returns the configured pull concurrency, defaulting to 8.
func (c *SyncConfig) EffectiveConcurrency() int {
	if c.Concurrency > 0 {
		return c.Concurrency
	}
	return 8
}

// HooksConfig holds configuration for git hooks managed by csl.
type HooksConfig struct {
	PostMerge PostMergeHook `yaml:"post_merge"`
}

// PostMergeHook configures the legacy post-merge hook installer.
//
// DEPRECATED: csl no longer manages post-merge hooks — suspenders is now the
// single git-hook manager and feeds ~/.config/csl/reindex.queue via a
// `csl-reindex` post_merge entry. csl still owns draining/indexing that queue.
// Enabled defaults to false (the zero value) for new configs; `csl hooks
// install` is retained only for backwards compatibility and prints a
// deprecation notice. Use `csl hooks uninstall` to remove any existing hooks.
//
// When enabled, `csl hooks install` writes .git/hooks/post-merge into every
// repo discovered via Dirs, except those listed in Exclude.
// Exclude entries are matched against both the repo's absolute path and its
// org/repo name (exact match, no globs).
type PostMergeHook struct {
	Enabled bool     `yaml:"enabled"`
	Exclude []string `yaml:"exclude"`
}

// IsExcluded reports whether the given repo (by absolute path or org/repo name)
// should be skipped by the hook installer.
func (h *PostMergeHook) IsExcluded(repoPath, repoName string) bool {
	if h == nil {
		return false
	}
	for _, e := range h.Exclude {
		e = expandTilde(e)
		if e == repoPath || e == repoName {
			return true
		}
	}
	return false
}

// FilterExcluded returns repos with the entries matching the exclude list
// removed. Every path that indexes (lexical or semantic) must run discovered
// repos through this so an excluded repo can never enter the index.
func (h *PostMergeHook) FilterExcluded(repos []finder.Repo) []finder.Repo {
	if h == nil || len(h.Exclude) == 0 {
		return repos
	}
	out := make([]finder.Repo, 0, len(repos))
	for _, r := range repos {
		if h.IsExcluded(r.Path, r.Name) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// SemanticEnabled reports whether the daemon should load the semantic index and
// embedding model. Safe to call on a nil receiver (returns false).
func (c *Config) SemanticEnabled() bool {
	return c != nil && c.Semantic.Enabled
}

// SemanticSyncEnabled reports whether `csl sync` should also refresh embeddings.
// Safe to call on a nil receiver (returns false).
func (c *Config) SemanticSyncEnabled() bool {
	return c != nil && c.Semantic.Sync
}

// DaemonIdleTimeout returns the configured daemon idle timeout, defaulting to
// 10 minutes when unset or non-positive. Safe to call on a nil receiver.
func (c *Config) DaemonIdleTimeout() time.Duration {
	if c != nil && c.Daemon.IdleTimeoutMinutes > 0 {
		return time.Duration(c.Daemon.IdleTimeoutMinutes) * time.Minute
	}
	return 10 * time.Minute
}

// SummaryEnabled returns true when the summary tab should be created.
// Requires summary: true AND layout: tab.
func (c *Config) SummaryEnabled() bool {
	return c != nil && c.Summary && c.EffectiveLayout() == LayoutTab
}

// EffectiveTmpDir returns the configured tmpdir for scratch sessions.
// Returns empty string when unset, meaning os.MkdirTemp default should be used.
func (c *Config) EffectiveTmpDir() string {
	if c != nil && c.TmpDir != "" {
		return expandTilde(c.TmpDir)
	}
	return ""
}

// EffectiveLayout returns the configured layout, defaulting to split.
// Safe to call on a nil receiver.
func (c *Config) EffectiveLayout() string {
	if c != nil && c.Layout == LayoutTab {
		return LayoutTab
	}
	return LayoutSplit
}

// Load reads config.yaml from ~/.config/csl/config.yaml.
func Load() (*Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}
	globalPath := filepath.Join(home, ".config", "csl", configFileName)
	cfg, err := loadFrom(globalPath)
	if err != nil {
		return nil, fmt.Errorf("no config found (checked %s): %w", globalPath, err)
	}
	return cfg, nil
}

// LoadFrom reads config from a specific path.
func LoadFrom(path string) (*Config, error) {
	return loadFrom(path)
}

func loadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	// Expand tildes in directory paths
	for i, d := range cfg.Dirs {
		cfg.Dirs[i] = expandTilde(d)
	}
	cfg.TmpDir = expandTilde(cfg.TmpDir)
	for i, e := range cfg.Hooks.PostMerge.Exclude {
		cfg.Hooks.PostMerge.Exclude[i] = expandTilde(e)
	}

	return &cfg, nil
}

func expandTilde(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return p
		}
		return filepath.Join(home, p[2:])
	}
	return p
}

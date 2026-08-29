package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/kit/confdir"
	"github.com/mad01/thismoon/kit/envdefault"
	"github.com/mad01/thismoon/kit/repofind"
	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

const configFileName = "config.yaml"

// PathEnv names the environment variable that relocates config.yaml. The
// --config flag wins over it; both beat the default location.
const PathEnv = "CSL_CONFIG"

// Config holds the repo finder configuration.
type Config struct {
	Dirs     []string       `yaml:"dirs"`
	Hooks    HooksConfig    `yaml:"hooks"`
	Sync     SyncConfig     `yaml:"sync"`
	Index    IndexConfig    `yaml:"index"`
	Semantic SemanticConfig `yaml:"semantic"`
	Daemon   DaemonConfig   `yaml:"daemon"`
	Web      WebConfig      `yaml:"web"`
	Refresh  RefreshConfig  `yaml:"refresh"`
}

// RefreshConfig controls the background index refresh loop that `csl web`
// runs: a periodic sync (pull + reindex changed repos) sharing the sync lock
// with the manual `csl sync` command.
type RefreshConfig struct {
	// Enabled gates the periodic background refresh. Unset means enabled — a
	// pointer distinguishes "not configured" from an explicit false. Manual
	// refresh from the web UI works either way.
	Enabled *bool `yaml:"enabled"`
	// IntervalMinutes is how often the background refresh runs. Zero or
	// negative means the default (15 minutes).
	IntervalMinutes int `yaml:"interval_minutes"`
}

// WebConfig tells the other csl surfaces where the web UI is reachable.
type WebConfig struct {
	// BaseURL is the web UI's base URL, used by csl_show_file to build the
	// links it opens. Empty derives it from the port csl resolves (CSL_PORT,
	// else 7424); set it to http://csl.this when the UI is fronted by d-man,
	// which no local port can describe.
	BaseURL string `yaml:"base_url"`
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
	// Dim is the embedding dimensionality of EmbedModel. Zero means 768, the
	// width of the default model (jina-code-v2); any other model needs its
	// own width set here. Stores record the dimensionality they were built
	// with, so a mismatch is detected as a model swap and re-embeds.
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
// single git-hook manager and feeds csl's reindex queue via a `csl-reindex`
// post_merge entry. csl still owns draining/indexing that queue.
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
		e = repofind.ExpandHome(e)
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

// EffectiveWebBaseURL returns the web UI base URL without a trailing slash.
// An explicit web.base_url always wins; otherwise the URL is derived from
// the port csl resolves (CSL_PORT, else the default), so moving the UI off
// 7424 also moves the links csl_show_file hands out. Safe to call on a nil
// receiver.
func (c *Config) EffectiveWebBaseURL() string {
	if c != nil && c.Web.BaseURL != "" {
		return strings.TrimRight(c.Web.BaseURL, "/")
	}
	return csl.BaseURLForPort(csl.ResolvedPort())
}

// RefreshEnabled reports whether `csl web` should run the periodic background
// refresh. Defaults to true when unset. Safe to call on a nil receiver.
func (c *Config) RefreshEnabled() bool {
	if c == nil || c.Refresh.Enabled == nil {
		return true
	}
	return *c.Refresh.Enabled
}

// RefreshInterval returns the configured background refresh interval,
// defaulting to 15 minutes when unset or non-positive. Safe to call on a nil
// receiver.
func (c *Config) RefreshInterval() time.Duration {
	if c != nil && c.Refresh.IntervalMinutes > 0 {
		return time.Duration(c.Refresh.IntervalMinutes) * time.Minute
	}
	return 15 * time.Minute
}

// DaemonIdleTimeout returns the configured daemon idle timeout, defaulting to
// 10 minutes when unset or non-positive. Safe to call on a nil receiver.
func (c *Config) DaemonIdleTimeout() time.Duration {
	if c != nil && c.Daemon.IdleTimeoutMinutes > 0 {
		return time.Duration(c.Daemon.IdleTimeoutMinutes) * time.Minute
	}
	return 10 * time.Minute
}

// pinnedPath is the config file Load reads, when something pinned one.
// Empty means "resolve from the environment". The cobra root sets it from
// --config before any subcommand runs, so every surface in the process —
// CLI, MCP server, search daemon — reads the same file.
var pinnedPath string

// SetPath pins the config file Load reads, overriding CSL_CONFIG and the
// default location. Passing "" restores the resolved default.
func SetPath(path string) { pinnedPath = path }

// Path returns the config file location: the path pinned by --config, else
// CSL_CONFIG, else config.yaml under the XDG config directory
// ($XDG_CONFIG_HOME/csl, or ~/.config/csl when that variable is unset). It
// is the only file Load reads; `csl config` prints it so a diagnosis names
// the file it is talking about.
func Path() (string, error) {
	if pinnedPath != "" {
		return confdir.Expand(pinnedPath)
	}
	if p := envdefault.String(PathEnv, ""); p != "" {
		return confdir.Expand(p)
	}
	return confdir.Path(csl.Component, configFileName)
}

// Load reads the config file Path resolves.
func Load() (*Config, error) {
	globalPath, err := Path()
	if err != nil {
		return nil, err
	}
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

	// Expand tildes once, here at the boundary, so everything downstream
	// works with real paths. An unresolvable home is an error rather than a
	// path relative to whatever directory csl happened to start in.
	if err := expandAll(cfg.Dirs); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := expandAll(cfg.Hooks.PostMerge.Exclude); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	return &cfg, nil
}

// expandAll expands a leading ~ in every entry of paths, in place.
func expandAll(paths []string) error {
	for i, p := range paths {
		expanded, err := confdir.Expand(p)
		if err != nil {
			return err
		}
		paths[i] = expanded
	}
	return nil
}

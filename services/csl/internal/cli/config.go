package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/semantic"
)

// configReference documents every setting csl reads, shipped in the binary so
// a fresh machine can be configured from the terminal without the repo.
const configReference = `# config.yaml: every key optional.

dirs:                        # directories walked for git repos; every repo
  - ~/code/src               # found under one of them is a candidate for the
  - ~/workspace              # index (~ expands)

index:
  hosts:                     # git remote host allowlist. Only repos whose
    - github.com             # origin remote matches a listed host are indexed.
                             # Empty means index every repo found.

sync:
  concurrency: 8             # parallel 'git pull' workers for 'csl sync'
                             # (0 or negative means 8)

semantic:                    # optional vector search, independent of the
  enabled: false             # lexical zoekt index. 'enabled' gates whether the
                             # search daemon loads the semantic index and the
                             # embedder at startup; the in-process CLI/MCP/web
                             # paths still answer ad-hoc semantic queries.
  sync: false                # whether 'csl sync' re-embeds the changed files
                             # of changed repos after the lexical reindex.
                             # Best-effort: never fails the sync.
  ollama_url: ""             # embedding server; empty means
                             # http://localhost:11434
  embed_model: ""            # embedding model; empty means the compiled-in
                             # default (jina-code-v2)
  dim: 0                     # the model's embedding width; 0 means the default
                             # model's. Must match the model: changing either
                             # re-embeds every store on the next index run.

daemon:
  idle_timeout_minutes: 10   # how long the search daemon stays alive with no
                             # queries. Higher keeps the zoekt shards and
                             # semantic stores warm at the cost of resident
                             # memory; 0 or negative means 10.

refresh:
  enabled: true              # whether 'csl web' runs a periodic background
                             # sync (pull + reindex changed repos). Omitted
                             # means enabled; manual refresh from the web UI
                             # works either way.
  interval_minutes: 15       # how often the background refresh runs; 0 or
                             # negative means 15. Keep it conservative: every
                             # cycle contacts every repo's remote.

web:
  base_url: ""               # where the csl web UI is reachable, for the links
                             # csl_show_file opens; empty derives it from the
                             # port (CSL_PORT, else 7424). Set http://csl.this
                             # when fronted by d-man.

hooks:
  post_merge:                # DEPRECATED installer: suspenders owns git hooks
    enabled: false           # now and feeds the reindex queue, which csl still
                             # drains. Only 'csl hooks install' reads this flag.
    exclude:                 # still live, and shared with every index and sync
      - you/huge-repo        # path: an excluded repo never enters the index.
      - ~/code/src/scratch   # Matched against the repo's absolute path or its
                             # org/repo name: exact, no globs.

# Environment (no YAML key of their own):
#   CSL_CONFIG   moves this file; --config beats it.
#   CSL_PORT     the port csl assumes the web UI listens on: 'csl web' binds
#                it and every other surface links to it. web.base_url wins.
#   EVENTS_BASE_URL  the events service sync runs archive to; best-effort.
#
# A repo can also carry a .cslignore at its root (one glob per line) which
# filters both indexes. That file is per-repo, not part of this config.`

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Show the current effective config",
	Long: `Print which config file csl reads and the settings in effect after defaults
are applied.

csl reads exactly one file per run: --config when given, else CSL_CONFIG, else
config.yaml in the XDG config directory ($XDG_CONFIG_HOME/csl, or ~/.config/csl
when that is unset). There is no per-repo or per-directory override. The file is
optional and so is every key in it: a missing file leaves csl on its defaults
with no repos configured, and the header line above the output says whether the
file was loaded, absent, or unparseable.

` + configReference + `

Pair it with doctor: doctor shows the state csl resolved, config shows which
file and key to change.`,
	Args: cobra.NoArgs,
	RunE: runConfig,
}

func init() {
	rootCmd.AddCommand(configCmd)
}

func runConfig(cmd *cobra.Command, args []string) error {
	path, err := config.Path()
	if err != nil {
		return err
	}

	cfg, loadErr := config.LoadFrom(path)
	if cfg == nil {
		cfg = &config.Config{}
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "config file: %s (%s)\n\n", path, configStatus(loadErr))

	b, err := yaml.Marshal(effectiveConfig(cfg))
	if err != nil {
		return fmt.Errorf("marshaling effective config: %w", err)
	}
	_, err = out.Write(b)
	return err
}

// effectiveConfig returns a copy of cfg with every defaulted field resolved
// through the accessor that owns its default, so the printed YAML shows the
// values csl runs on rather than the blanks the file left behind.
func effectiveConfig(cfg *config.Config) config.Config {
	eff := *cfg
	eff.Sync.Concurrency = cfg.Sync.EffectiveConcurrency()
	eff.Daemon.IdleTimeoutMinutes = int(cfg.DaemonIdleTimeout() / time.Minute)
	enabled := cfg.RefreshEnabled()
	eff.Refresh.Enabled = &enabled
	eff.Refresh.IntervalMinutes = int(cfg.RefreshInterval() / time.Minute)
	eff.Semantic.Enabled = cfg.SemanticEnabled()
	eff.Semantic.Sync = cfg.SemanticSyncEnabled()
	eff.Web.BaseURL = cfg.EffectiveWebBaseURL()

	// The embedding defaults live in the embedder, not in the config package;
	// building one is the only way to read them without duplicating them here.
	emb := semantic.NewOllamaEmbedder(
		cfg.Semantic.OllamaURL,
		cfg.Semantic.EmbedModel,
		cfg.Semantic.Dim,
	)
	eff.Semantic.OllamaURL = emb.Endpoint
	eff.Semantic.EmbedModel = emb.Model
	eff.Semantic.Dim = emb.Dim()

	return eff
}

// configStatus describes a config load for the header line. A broken file is
// not fatal here — the command names the problem and prints the defaults csl
// would fall back to.
func configStatus(err error) string {
	switch {
	case err == nil:
		return "loaded"
	case errors.Is(err, fs.ErrNotExist):
		return "missing, defaults in use"
	default:
		return fmt.Sprintf("parse error: %v", err)
	}
}

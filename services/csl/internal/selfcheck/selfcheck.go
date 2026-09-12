// Package selfcheck holds csl's self-diagnosis: the check list behind
// `csl doctor` and the csl_doctor MCP tool. It lives outside internal/cli
// because the MCP server serves the same checks, and internal/cli already
// imports the MCP server.
//
// The name says self, not repo: csl also reports git health for the repos it
// indexes (csl_repo_health), which is a different question from whether csl
// itself is working.
package selfcheck

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/repo/catalogspec"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// Checks returns the csl check list. The first six cover what csl needs to
// answer a query at all: config, what the config discovered, state file,
// index freshness, shard integrity, and the search server. The last two are
// the kit's web probes renamed to say which process they are about — `csl
// web` is the one long-lived process, and search works without it, so a web
// FAIL must not read as "csl is down".
//
// repair lets the state-file check reset a corrupt file; pass false from any
// read-only surface. The web probes target the base URL csl resolves
// (web.base_url, else the CSL_PORT-aware default), so a relocated UI is not
// reported as unreachable.
func Checks(repair bool) []doctor.Check {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return []doctor.Check{{
			Name: "index-dir",
			Run:  func(context.Context) error { return err },
		}}
	}
	s := newShared()
	baseURL := webBaseURL(s)
	webReachable := doctor.ServiceReachable(baseURL)
	webReachable.Name = "web-ui-reachable"
	webSkew := doctor.VersionSkew(baseURL)
	webSkew.Name = "web-ui-version-skew"
	return []doctor.Check{
		configLoads(s),
		reposDiscovered(s),
		catalogDescriptors(s),
		stateFileLoads(indexDir, repair),
		indexFreshness(indexDir, s),
		indexShardsValid(indexDir),
		searchServerResponsive(daemon.DefaultPIDPath(), daemon.DefaultSocketPath()),
		webReachable,
		webSkew,
	}
}

// shared is what several checks need and none should compute twice: the
// loaded config and the discovery walk. One doctor run loads the file once and
// walks the dirs once, however many checks ask, so adding a check that looks at
// discovery does not add a walk.
type shared struct {
	config   func() (*config.Config, error)
	discover func() (discovery, error)
}

// discovery is one walk's answer: the repos csl will index and the ones a
// filter removed.
type discovery struct {
	repos   []finder.Repo
	dropped []finder.Dropped
}

func newShared() shared {
	load := sync.OnceValues(config.Load)
	return shared{
		config: load,
		discover: sync.OnceValues(func() (discovery, error) {
			cfg, err := load()
			if err != nil {
				return discovery{}, fmt.Errorf("load config: %w", err)
			}
			repos, dropped, err := cfg.DiscoverReposReport()
			return discovery{repos: repos, dropped: dropped}, err
		}),
	}
}

// webBaseURL is where the web UI should answer. A config that fails to load
// still has an answer — the defaults — and the config check reports the load
// failure on its own line.
func webBaseURL(s shared) string {
	cfg, err := s.config()
	if err != nil {
		cfg = &config.Config{}
	}
	return cfg.EffectiveWebBaseURL()
}

// configLoads verifies the config file parses and names at least one
// directory to walk.
//
// The two empty states are not the same. No config file at all is a machine
// that has not been set up yet, which is a state csl runs in deliberately, so
// the check skips with the path to create rather than failing. A file that
// exists and sets no dirs is someone's unfinished configuration, and stays a
// failure naming the file: that is the "why does the UI show no repos" case.
func configLoads(s shared) doctor.Check {
	return doctor.Check{
		Name: "config-loads",
		Run: func(context.Context) error {
			cfg, err := s.config()
			if err != nil {
				return err
			}
			if !cfg.Loaded {
				return doctor.Skip("csl is running on defaults — " + cfg.EmptyResultHint())
			}
			if len(cfg.Dirs) == 0 {
				return fmt.Errorf("%s sets no dirs — nothing will be indexed", cfg.Path)
			}
			return nil
		},
	}
}

// reposDiscovered reports what the configured dirs actually yielded. The kept
// count answers the first question about a machine that searches empty, and
// the drops answer the second: a repo csl never sees was either off the
// index.hosts allowlist or on the exclude list, and nothing outside the config
// says so. Keeping nothing on a configured machine is the failure — that is a
// csl with nothing to search.
func reposDiscovered(s shared) doctor.Check {
	return doctor.Check{
		Name: "repos-discovered",
		Run: func(context.Context) error {
			cfg, err := s.config()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if !cfg.Loaded {
				return doctor.Skip("csl is running on defaults — " + cfg.EmptyResultHint())
			}
			if len(cfg.Dirs) == 0 {
				// config-loads already failed this file for setting no dirs.
				// A second FAIL here would claim repos were sought under dirs
				// that do not exist; step aside instead.
				return doctor.Skip("no dirs to walk; see config-loads")
			}
			d, err := s.discover()
			if err != nil {
				return err
			}
			if len(d.repos) == 0 {
				return errors.New(cfg.EmptyDiscoveryHint(d.dropped))
			}
			// Plain ok when every walked repo made it in. A Skip note only
			// when a filter dropped something, so the note carries the
			// counts: the same "a pass with something to say" config-loads
			// uses for a missing file, and csl_doctor consumers see
			// "skipped" only when there is something to read.
			summary := config.CountDrops(d.dropped).Summary()
			if summary == "" {
				return nil
			}
			return doctor.Skip(fmt.Sprintf("%d repos, %s", len(d.repos), summary))
		},
	}
}

// catalogDescriptors verifies the catalog descriptors discovery found can be
// read. Discovery drops a broken descriptor silently, because one repo's
// YAML must not take the walk down, so this is where the drop becomes
// visible: it fails naming every descriptor that exists and cannot be read
// or followed. A fleet where no repo carries one passes with a note, since
// that is the state where --owner and --system lookups match nothing and
// the reader may not know why.
func catalogDescriptors(s shared) doctor.Check {
	return doctor.Check{
		Name: "catalog-descriptors",
		Run: func(context.Context) error {
			d, err := s.discover()
			if err != nil || len(d.repos) == 0 {
				// repos-discovered owns that failure.
				return doctor.Skip("no repos to check")
			}
			total := len(d.repos)
			found := 0
			var broken []string
			for _, r := range d.repos {
				_, ok, err := catalogspec.Read(r.Path)
				switch {
				case err != nil:
					broken = append(broken, r.Name+": "+err.Error())
				case ok:
					found++
				}
			}
			if len(broken) > 0 {
				return fmt.Errorf("%d of %d repos have a catalog descriptor csl cannot read; "+
					"they list without owner or system:\n  %s",
					len(broken), total, strings.Join(broken, "\n  "))
			}
			if found == 0 {
				names := strings.Join(
					catalogspec.FileNames,
					", ",
				) + " or " + catalogspec.PointerFile
				return doctor.Skip(fmt.Sprintf(
					"none of %d repos carries a catalog descriptor (%s at the root), "+
						"so --owner/--system lookups match nothing", total, names,
				))
			}
			return nil
		},
	}
}

// stateFileLoads verifies state.json parses. LoadState renames a corrupt
// file to state.json.corrupt on the way out, so with repair the check
// completes the reset by writing a fresh empty state; the next index run
// rebuilds from scratch.
func stateFileLoads(indexDir string, repair bool) doctor.Check {
	return doctor.Check{
		Name: "state-file-loads",
		Run: func(context.Context) error {
			_, err := search.LoadState(indexDir)
			if err == nil {
				return nil
			}
			if !repair {
				return fmt.Errorf("%w; run 'csl doctor --repair' to reset state", err)
			}
			if saveErr := search.EmptyState().Save(indexDir); saveErr != nil {
				return fmt.Errorf("reset state: %w", saveErr)
			}
			return nil
		},
	}
}

// indexFreshness compares each discovered repo's fingerprint against the
// indexed state. Stale repos are not broken — searches answer from the old
// shards and reindex them in the background — but the count is the first
// thing to know when results look wrong, so it fails with the totals.
func indexFreshness(indexDir string, s shared) doctor.Check {
	return doctor.Check{
		Name: "index-freshness",
		Run: func(context.Context) error {
			cfg, err := s.config()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			if len(cfg.Dirs) == 0 {
				// Nothing configured to walk. configLoads has already
				// decided whether that is a fresh machine or an unfinished
				// config; either way there is no freshness to report.
				return nil
			}
			d, err := s.discover()
			if err != nil {
				return err
			}
			repos := d.repos
			if len(repos) == 0 {
				// Discovery yielded nothing. repos-discovered owns that
				// verdict and names the filter that emptied the list; a
				// second FAIL for the same cause only doubles the noise.
				return doctor.Skip("no repos to check")
			}
			state, err := search.LoadState(indexDir)
			if err != nil {
				return fmt.Errorf("load state: %w", err)
			}
			if indexedRepos(repos, state) == 0 {
				// None of the configured repos has been indexed, which is
				// where every machine starts. Calling that stale reads as
				// breakage on an install that is working exactly as designed.
				return doctor.Skip(fmt.Sprintf(
					"%d repos discovered, none of them indexed yet; "+
						"the first search or 'csl index' builds the index",
					len(repos),
				))
			}
			staleness, err := search.CheckStaleness(repos, state)
			if err != nil {
				return err
			}
			if n := len(staleness.Stale); n > 0 {
				return fmt.Errorf(
					"%d of %d repos stale, dirty, or unindexed; "+
						"the next search reindexes them in the background, 'csl index' does it now",
					n, len(repos),
				)
			}
			return nil
		},
	}
}

// indexedRepos counts the discovered repos the index state knows about. Zero
// means none of this machine's configured repos has been indexed, which reads
// differently from an index that has fallen behind. (An index built before a
// dirs change may still hold other repos; they are not the configured set.)
func indexedRepos(repos []finder.Repo, state *search.IndexState) int {
	n := 0
	for _, r := range repos {
		if _, ok := state.GetRepo(r.Path); ok {
			n++
		}
	}
	return n
}

// indexShardsValid opens every .zoekt shard and verifies it, the same scan
// `csl index --repair` uses to decide what to drop.
func indexShardsValid(indexDir string) doctor.Check {
	return doctor.Check{
		Name: "index-shards-valid",
		Run: func(context.Context) error {
			shards, corrupted, err := search.ValidateShards(indexDir)
			if err != nil {
				return err
			}
			if len(corrupted) > 0 {
				return fmt.Errorf(
					"%d of %d shards corrupted; run 'csl index --repair' to drop them, then 'csl index' to rebuild",
					len(corrupted),
					len(shards),
				)
			}
			return nil
		},
	}
}

// searchServerResponsive fails only when the PID file names a live process
// whose socket does not answer: every query then burns the fallback path,
// opening shards in-process. A stopped server is healthy — the next query
// starts one, and it idle-exits by design.
func searchServerResponsive(pidPath, socketPath string) doctor.Check {
	return doctor.Check{
		Name: "search-server-responsive",
		Run: func(context.Context) error {
			if !daemon.IsRunning(pidPath) {
				return nil
			}
			if err := daemon.Ping(socketPath); err != nil {
				return fmt.Errorf(
					"search server is alive but its socket does not answer: %w; "+
						"'csl search --stop' kills it and the next query starts a fresh one",
					err,
				)
			}
			return nil
		},
	}
}

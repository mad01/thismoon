package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/mad01/thismoon/kit/agentdoc/agentcli"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

var doctorRepairFlag bool

func init() {
	// Checks build at run time so --repair has resolved before the state
	// check decides whether to reset a corrupt file.
	doctorCmd := agentcli.DoctorCommand(csl.Facts(), func(context.Context) []doctor.Check {
		return doctorChecks(doctorRepairFlag)
	})
	doctorCmd.Flags().
		BoolVar(&doctorRepairFlag, "repair", false, "back up a corrupt state file and reset it")
	rootCmd.AddCommand(doctorCmd)
}

// doctorChecks is the csl check list. The first five carry over what the
// pre-kit doctor examined: config, state file, index freshness, shard
// integrity, and the search server. The last two are the kit's web probes
// renamed to say which process they are about — `csl web` is the one
// long-lived process, and search works without it, so a web FAIL must not
// read as "csl is down".
func doctorChecks(repair bool) []doctor.Check {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return []doctor.Check{{
			Name: "index-dir",
			Run:  func(context.Context) error { return err },
		}}
	}
	webReachable := doctor.ServiceReachable(csl.DefaultBaseURL)
	webReachable.Name = "web-ui-reachable"
	webSkew := doctor.VersionSkew(csl.DefaultBaseURL)
	webSkew.Name = "web-ui-version-skew"
	return []doctor.Check{
		configLoads(),
		stateFileLoads(indexDir, repair),
		indexFreshness(indexDir),
		indexShardsValid(indexDir),
		searchServerResponsive(daemon.DefaultPIDPath(), daemon.DefaultSocketPath()),
		webReachable,
		webSkew,
	}
}

// configLoads verifies config.yaml parses. A missing file fails too: without
// it csl has no dirs to walk, so nothing downstream can work.
func configLoads() doctor.Check {
	return doctor.Check{
		Name: "config-loads",
		Run: func(context.Context) error {
			_, err := config.Load()
			return err
		},
	}
}

// stateFileLoads verifies state.json parses. LoadState renames a corrupt
// file to state.json.corrupt on the way out, so with --repair the check
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
func indexFreshness(indexDir string) doctor.Check {
	return doctor.Check{
		Name: "index-freshness",
		Run: func(context.Context) error {
			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			repos, err := finder.FilteredWalk(cfg.Dirs, cfg.Index.Hosts)
			if err != nil {
				return err
			}
			if len(repos) == 0 {
				return errors.New("no git repos found under the configured dirs; check 'csl config'")
			}
			state, err := search.LoadState(indexDir)
			if err != nil {
				return fmt.Errorf("load state: %w", err)
			}
			staleness, err := search.CheckStaleness(repos, state)
			if err != nil {
				return err
			}
			if n := len(staleness.Stale); n > 0 {
				return fmt.Errorf(
					"%d of %d repos stale, dirty, or unindexed; "+
						"the next search reindexes them in the background, 'csl index' does it now",
					n, len(repos))
			}
			return nil
		},
	}
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
					len(corrupted), len(shards))
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
					err)
			}
			return nil
		},
	}
}

package syncer

import (
	"fmt"
	"io"
	"maps"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// indexInput is what the index phase needs from the pull phase.
type indexInput struct {
	indexDir    string
	results     []PullResult  // pull outcome per evaluated repo, sorted by name
	queued      []finder.Repo // repos drained from the reindex queue
	hiddenDirs  []string      // hidden directories the walk may enter
	concurrency int           // parallel fingerprint probes
	errw        io.Writer     // progress lines
}

// indexChanged rebuilds the shards for every repo the sync must refresh and
// records their fingerprints in one state.json write. That set is the repos
// the pull updated, the ok repos the state has never seen, the queued repos,
// and every evaluated repo whose fingerprint no longer matches the state, so
// a commit made outside sync or an index format bump heals on the next run.
// It returns the indexed repos in that order, deduplicated.
func indexChanged(in indexInput) ([]finder.Repo, error) {
	state, err := search.LoadState(in.indexDir)
	if err != nil {
		return nil, err
	}
	stale, current, err := staleRepos(in.results, state, in.concurrency)
	if err != nil {
		return nil, err
	}
	toIndex := unionRepos(reposToIndex(in.results, state, in.queued), stale)
	if len(toIndex) == 0 {
		return nil, nil
	}

	fmt.Fprintf(in.errw, "\nIndexing %d repo(s)...\n", len(toIndex))
	progress := func(i, total int, repo finder.Repo) {
		fmt.Fprintf(in.errw, "  [%d/%d] %s\n", i+1, total, repo.Name)
	}
	if err := search.IndexRepos(in.indexDir, toIndex, in.hiddenDirs, progress); err != nil {
		return nil, fmt.Errorf("indexing failed: %w", err)
	}

	recordFingerprints(state, toIndex, current, time.Now())
	if err := state.Save(in.indexDir); err != nil {
		return nil, fmt.Errorf("save state: %w", err)
	}
	return toIndex, nil
}

// staleRepos fingerprints every evaluated repo that has a git tree (all but
// notgit, whatever the pull did) in parallel. It returns the repos whose
// fingerprint differs from the stored one, in results order, plus every
// fingerprint it computed. A repo git cannot fingerprint is left out: it
// could never be recorded as fresh, so indexing it would repeat every run.
func staleRepos(
	results []PullResult,
	state *search.IndexState,
	concurrency int,
) ([]finder.Repo, map[string]search.RepoState, error) {
	checks := make([]*search.StalenessResult, len(results))
	var g errgroup.Group
	g.SetLimit(max(concurrency, 1))
	for i, r := range results {
		if r.Status == "notgit" {
			continue
		}
		g.Go(func() error {
			// Probe git now rather than trust a fingerprint another caller
			// cached seconds ago: sync is the authoritative refresh.
			search.InvalidateFingerprint(r.Repo.Path)
			res, err := search.CheckStaleness([]finder.Repo{r.Repo}, state)
			checks[i] = res
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, nil, fmt.Errorf("check index staleness: %w", err)
	}

	var stale []finder.Repo
	current := make(map[string]search.RepoState, len(results))
	for _, res := range checks {
		if res == nil {
			continue
		}
		maps.Copy(current, res.Current)
		for _, repo := range res.Stale {
			if _, ok := res.Current[repo.Path]; ok {
				stale = append(stale, repo)
			}
		}
	}
	return stale, current, nil
}

// unionRepos returns base followed by the extra repos it lacks, keyed by
// path and kept in first-seen order so the index order is stable.
func unionRepos(base, extra []finder.Repo) []finder.Repo {
	seen := make(map[string]bool, len(base)+len(extra))
	out := make([]finder.Repo, 0, len(base)+len(extra))
	for _, list := range [][]finder.Repo{base, extra} {
		for _, r := range list {
			if seen[r.Path] {
				continue
			}
			seen[r.Path] = true
			out = append(out, r)
		}
	}
	return out
}

// recordFingerprints stores each indexed repo's fingerprint in state. It
// prefers the fingerprint taken before indexing: if the tree changed while
// the shards were built, the stored value stays older than the shard and the
// next run reindexes, where a fresh probe would mark the change as indexed.
// Repos the check did not cover (queued repos outside this run) are probed
// now; one git cannot describe keeps its old entry.
func recordFingerprints(
	state *search.IndexState,
	repos []finder.Repo,
	current map[string]search.RepoState,
	now time.Time,
) {
	for _, repo := range repos {
		fp, ok := current[repo.Path]
		if !ok {
			probed, err := search.Fingerprint(repo.Path)
			if err != nil {
				continue
			}
			fp = probed
		}
		fp.IndexedAt = now
		state.SetRepo(repo.Path, fp)
	}
}

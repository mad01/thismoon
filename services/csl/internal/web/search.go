// Package web serves a localhost code-search UI and JSON API over the same
// search backend used by the CLI and MCP server. It is an imperative shell:
// search and read logic live in the search/daemon/finder packages, and this
// package only wires them to HTTP.
package web

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
	"github.com/mad01/thismoon/services/csl/internal/syncer"
)

// Service runs searches and file reads against the local index. It mirrors the
// CLI's daemon-first-with-in-process-fallback behaviour (see
// internal/cli/search.go) so the web UI shares the same warm index and results.
type Service struct {
	cfg        *config.Config
	indexDir   string
	socketPath string
	health     healthCache
}

// healthCache coalesces background git-health refreshes so only one sweep runs
// at a time within this process. The sweep result is persisted to disk by the
// search package (search.SaveHealthSnapshot); this guard only prevents piling
// up concurrent sweeps when many requests arrive while the snapshot is stale.
type healthCache struct {
	mu      sync.Mutex
	running bool
}

// NewService builds a Service from the given config.
func NewService(cfg *config.Config) (*Service, error) {
	indexDir, err := search.DefaultIndexDir()
	if err != nil {
		return nil, err
	}
	return &Service{
		cfg:        cfg,
		indexDir:   indexDir,
		socketPath: daemon.DefaultSocketPath(),
	}, nil
}

// Repos returns the discovered repositories, filtered by the configured host
// allowlist and exclude list.
func (s *Service) Repos() ([]finder.Repo, error) {
	return s.cfg.DiscoverRepos()
}

// SemanticEnabled reports whether semantic (and therefore hybrid) search is
// turned on in config; the web UI disables those modes when it is off.
func (s *Service) SemanticEnabled() bool {
	return s.cfg.SemanticEnabled()
}

// Search runs a query and returns flat matches. It tries the daemon first
// (auto-starting it) and falls back to an in-process search that indexes when
// no index exists yet.
func (s *Service) Search(ctx context.Context, opts search.SearchOptions) ([]search.Match, error) {
	repos, err := s.Repos()
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no repos found in configured directories")
	}

	repoNames := make(map[string]string, len(repos))
	for _, r := range repos {
		repoNames[r.Name] = r.Path
	}

	// Try the daemon first (auto-starts if not running).
	if err := daemon.EnsureDaemon(s.indexDir, s.socketPath); err == nil {
		matches, err := daemon.SearchVia(ctx, s.socketPath, opts, repoNames)
		if err == nil {
			return matches, nil
		}
		// Daemon failed — fall through to in-process search.
	}

	return s.searchInProcess(ctx, repos, repoNames, opts)
}

// searchInProcess indexes when necessary and searches without the daemon. It is
// the same fallback the CLI uses, minus progress output.
func (s *Service) searchInProcess(
	ctx context.Context,
	repos []finder.Repo,
	repoNames map[string]string,
	opts search.SearchOptions,
) ([]search.Match, error) {
	state, err := search.LoadState(s.indexDir)
	if err != nil {
		return nil, err
	}

	staleness, err := search.CheckStaleness(repos, state)
	if err != nil {
		return nil, err
	}

	// If no index exists at all, do a full index before searching.
	if len(state.Repos) == 0 {
		if err := s.buildInitialIndex(repos, staleness, state); err != nil {
			return nil, err
		}
	}

	return search.Search(ctx, s.indexDir, opts, repoNames)
}

// buildInitialIndex does the first-ever full index build, holding the
// cross-process sync lock for the write. When csl sync or the background
// refresher already holds the lock — the refresher's startup catch-up cycle
// lands exactly here — the holder is producing the same shards, so the build
// is skipped and the caller searches whatever exists.
func (s *Service) buildInitialIndex(
	repos []finder.Repo,
	staleness *search.StalenessResult,
	state *search.IndexState,
) error {
	unlock, err := syncer.Lock(s.indexDir)
	if errors.Is(err, syncer.ErrLocked) {
		return nil
	}
	if err != nil {
		return err
	}
	defer unlock()

	if err := search.IndexRepos(s.indexDir, repos, s.cfg.AllowedHiddenDirs(), nil); err != nil {
		return fmt.Errorf("indexing failed: %w", err)
	}
	for _, repo := range repos {
		if fp, ok := staleness.Current[repo.Path]; ok {
			fp.IndexedAt = time.Now()
			state.SetRepo(repo.Path, fp)
		}
	}
	_ = state.Save(s.indexDir)
	return nil
}

// FileLine is a single numbered line of a file.
type FileLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// ReadResult holds a slice of file lines for inline expansion and the file
// view page.
type ReadResult struct {
	Repo string `json:"repo"`
	Path string `json:"path"`
	// LocalPath is the absolute on-disk path with the home directory collapsed
	// to "~" (see collapseHome); it powers the copy-path action.
	LocalPath string     `json:"localPath,omitempty"`
	FileURL   string     `json:"fileURL,omitempty"`
	Lines     []FileLine `json:"lines"`
	// TotalLines is the file's full line count, so a capped read can say
	// "showing X of Y lines".
	TotalLines int  `json:"totalLines"`
	Truncated  bool `json:"truncated"`
}

// maxReadLines caps how many lines a single read returns, protecting the server
// from a request for a huge range.
const maxReadLines = 2000

// ReadFile returns a line range of a file in the named repo. start/end are
// 1-based and inclusive; 0 means start/end of file. It resolves the repo by
// substring match, matching the `csl read` command.
func (s *Service) ReadFile(repoName, relPath string, start, end int) (*ReadResult, error) {
	repos, err := s.Repos()
	if err != nil {
		return nil, err
	}

	// An exact name wins before the substring fallback: csl_show_file puts the
	// canonical org/repo name in its deep links, and a bare substring pick
	// would send "mad01/thismoon" to "mad01/thismoon-arcade" when the walk
	// returns the latter first.
	var matched *finder.Repo
	for i := range repos {
		if repos[i].Name == repoName {
			matched = &repos[i]
			break
		}
	}
	if matched == nil {
		for i := range repos {
			if strings.Contains(repos[i].Name, repoName) {
				matched = &repos[i]
				break
			}
		}
	}
	if matched == nil {
		return nil, fmt.Errorf("no repo matching %q found", repoName)
	}

	// Resolve and confine the path to the repo root to avoid traversal.
	clean := filepath.Clean("/" + relPath)
	absPath := filepath.Join(matched.Path, clean)
	if !strings.HasPrefix(absPath, matched.Path+string(os.PathSeparator)) {
		return nil, fmt.Errorf("invalid path %q", relPath)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open %s: %w", relPath, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var lines []FileLine
	lineNum := 0
	truncated := false
	for scanner.Scan() {
		lineNum++
		if start > 0 && lineNum < start {
			continue
		}
		// Past the range or the cap: keep counting for TotalLines but stop
		// collecting.
		if end > 0 && lineNum > end {
			continue
		}
		if len(lines) >= maxReadLines {
			truncated = true
			continue
		}
		lines = append(lines, FileLine{Number: lineNum, Text: scanner.Text()})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", relPath, err)
	}

	res := &ReadResult{
		Repo:       matched.Name,
		Path:       relPath,
		LocalPath:  absPath,
		FileURL:    finder.FileURL(*matched, strings.TrimPrefix(clean, "/"), 0),
		Lines:      lines,
		TotalLines: lineNum,
		Truncated:  truncated,
	}
	if home, err := os.UserHomeDir(); err == nil {
		res.LocalPath = collapseHome(res.LocalPath, home)
	}
	return res, nil
}

// healthTTL is how long a persisted git-health snapshot is served before the
// web layer triggers a background refresh.
const healthTTL = 5 * time.Minute

// GitHealthResult is a git-health sweep plus its freshness for the caller.
// Computing is true when a background refresh is in flight, meaning Entries is
// a stale snapshot (or empty on a cold start) that the UI should re-poll.
type GitHealthResult struct {
	Entries    []search.GitHealth
	ComputedAt time.Time
	Computing  bool
}

// GitHealth returns the fleet git-health sweep without blocking on git. It
// serves the persisted snapshot and, when that snapshot is missing or older
// than healthTTL, kicks off a single background refresh (stale-while-
// revalidate). A full sweep spawns git subprocesses per repo, so on a large
// fleet it is far too slow to run inside a request; callers that must have a
// fresh, blocking sweep use search.CachedGitHealthSweep directly.
func (s *Service) GitHealth(_ context.Context) (GitHealthResult, error) {
	snap, ok := search.LoadHealthSnapshot()
	if ok && time.Since(snap.ComputedAt) < healthTTL {
		return GitHealthResult{Entries: snap.Entries, ComputedAt: snap.ComputedAt}, nil
	}
	s.refreshHealthAsync()
	if ok {
		return GitHealthResult{
			Entries:    snap.Entries,
			ComputedAt: snap.ComputedAt,
			Computing:  true,
		}, nil
	}
	return GitHealthResult{Entries: []search.GitHealth{}, Computing: true}, nil
}

// refreshHealthAsync recomputes the git-health snapshot in the background,
// coalescing concurrent callers so only one sweep runs. It detaches from any
// request context so a client disconnect cannot cancel a sweep others depend
// on, and persists the result for every csl surface to share.
func (s *Service) refreshHealthAsync() {
	s.health.mu.Lock()
	if s.health.running {
		s.health.mu.Unlock()
		return
	}
	s.health.running = true
	s.health.mu.Unlock()

	go func() {
		defer func() {
			s.health.mu.Lock()
			s.health.running = false
			s.health.mu.Unlock()
		}()
		repos, err := s.Repos()
		if err != nil {
			return
		}
		snap := search.HealthSnapshot{
			ComputedAt: time.Now(),
			Entries:    search.GitHealthSweep(context.Background(), repos),
		}
		_ = search.SaveHealthSnapshot(snap)
	}()
}

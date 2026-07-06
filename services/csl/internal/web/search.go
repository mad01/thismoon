// Package web serves a localhost code-search UI and JSON API over the same
// search backend used by the CLI and MCP server. It is an imperative shell:
// search and read logic live in the search/daemon/finder packages, and this
// package only wires them to HTTP.
package web

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// Service runs searches and file reads against the local index. It mirrors the
// CLI's daemon-first-with-in-process-fallback behaviour (see
// internal/cli/search.go) so the web UI shares the same warm index and results.
type Service struct {
	cfg        *config.Config
	indexDir   string
	socketPath string
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
// allowlist.
func (s *Service) Repos() ([]finder.Repo, error) {
	return finder.FilteredWalk(s.cfg.Dirs, s.cfg.Index.Hosts)
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
		if err := search.IndexRepos(s.indexDir, repos, nil); err != nil {
			return nil, fmt.Errorf("indexing failed: %w", err)
		}
		for _, repo := range repos {
			if fp, ok := staleness.Current[repo.Path]; ok {
				fp.IndexedAt = time.Now()
				state.SetRepo(repo.Path, fp)
			}
		}
		_ = state.Save(s.indexDir)
	}

	return search.Search(ctx, s.indexDir, opts, repoNames)
}

// FileLine is a single numbered line of a file.
type FileLine struct {
	Number int    `json:"number"`
	Text   string `json:"text"`
}

// ReadResult holds a slice of file lines for inline expansion.
type ReadResult struct {
	Repo  string     `json:"repo"`
	Path  string     `json:"path"`
	Lines []FileLine `json:"lines"`
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

	var matched *finder.Repo
	for i := range repos {
		if strings.Contains(repos[i].Name, repoName) {
			matched = &repos[i]
			break
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
	for scanner.Scan() {
		lineNum++
		if start > 0 && lineNum < start {
			continue
		}
		if end > 0 && lineNum > end {
			break
		}
		lines = append(lines, FileLine{Number: lineNum, Text: scanner.Text()})
		if len(lines) >= maxReadLines {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", relPath, err)
	}

	return &ReadResult{Repo: matched.Name, Path: relPath, Lines: lines}, nil
}

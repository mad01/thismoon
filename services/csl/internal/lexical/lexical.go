// Package lexical holds the rules both lexical search front ends apply
// around a zoekt query: the csl search command and the csl_search MCP tool.
// It refuses a malformed query with the fix, widens a repo filter to match
// case-insensitively, and explains an empty result: per-term file counts,
// the one relaxed rerun without the terms that match no file, and the
// zero-result hint. Each front end keeps its own output shape and decides
// where the hint goes; the decisions and their wording live here once.
package lexical

import (
	"context"
	"errors"

	"github.com/mad01/thismoon/services/csl/internal/daemon"
	"github.com/mad01/thismoon/services/csl/internal/search"
)

// RepoFilter prepends (?i) to a zoekt repo: filter so the lexical backend
// matches repo names case-insensitively, the way the repo tools and
// finder.CompileMatcher do. An empty filter stays empty.
func RepoFilter(repo string) string {
	if repo == "" {
		return ""
	}
	return "(?i)" + repo
}

// Backend is the query surface the zero-result diagnosis needs: a search and
// a count over the same index. Local and Daemon are the production ones;
// tests substitute a fake.
type Backend interface {
	Search(ctx context.Context, opts search.SearchOptions) ([]search.Match, error)
	Count(ctx context.Context, opts search.CountOptions) ([]search.CountResult, int, error)
}

// Local searches the zoekt shards in this process, opening them per call.
type Local struct {
	IndexDir  string
	RepoNames map[string]string // org/repo name to checkout path
}

// Search runs one search against the shards in IndexDir.
func (l Local) Search(ctx context.Context, opts search.SearchOptions) ([]search.Match, error) {
	return search.Search(ctx, l.IndexDir, opts, l.RepoNames)
}

// Count runs one count against the shards in IndexDir.
func (l Local) Count(
	ctx context.Context,
	opts search.CountOptions,
) ([]search.CountResult, int, error) {
	return search.Count(ctx, l.IndexDir, opts)
}

// Daemon asks the search daemon first, starting it when nothing answers, and
// falls back to Local when the daemon is not running. Any other daemon error
// is returned, not retried in-process.
type Daemon struct {
	Local
	SocketPath string
}

// Search runs one search through the daemon, or in-process without one.
func (d Daemon) Search(ctx context.Context, opts search.SearchOptions) ([]search.Match, error) {
	if err := daemon.EnsureDaemon(d.IndexDir, d.SocketPath); err == nil {
		matches, err := daemon.SearchVia(ctx, d.SocketPath, opts, d.RepoNames)
		if err == nil {
			return matches, nil
		}
		if !errors.Is(err, daemon.ErrDaemonNotRunning) {
			return nil, err
		}
	}
	return d.Local.Search(ctx, opts)
}

// Count runs one count through the daemon, or in-process without one.
func (d Daemon) Count(
	ctx context.Context,
	opts search.CountOptions,
) ([]search.CountResult, int, error) {
	if err := daemon.EnsureDaemon(d.IndexDir, d.SocketPath); err == nil {
		results, total, err := daemon.CountVia(ctx, d.SocketPath, opts)
		if err == nil {
			return results, total, nil
		}
		if !errors.Is(err, daemon.ErrDaemonNotRunning) {
			return nil, 0, err
		}
	}
	return d.Local.Count(ctx, opts)
}

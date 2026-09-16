package mcpserver

import (
	"fmt"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
	"github.com/mad01/thismoon/services/csl/internal/repo/finder"
)

// Repo-name matching across the csl_* tools is unified on case-insensitive
// regex (finder.CompileMatcher). Tools that need exactly one repo resolve it
// with resolveRepo; tools that pass a repo filter to zoekt widen it with
// insensitiveRepoFilter so the lexical side matches the same way. The one
// exception is the semantic store, which matches by case-sensitive substring —
// its filter is shared with the daemon, web UI, and CLI, so the tool
// descriptions call that out instead of translating.

// resolveRepo finds a single repo by name among the discovered repos; the
// matching rule and its errors are finder.MatchOne's.
func resolveRepo(name string) (finder.Repo, error) {
	cfg, err := config.Load()
	if err != nil {
		return finder.Repo{}, fmt.Errorf("load csl config: %w", err)
	}
	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return finder.Repo{}, fmt.Errorf("walk repos: %w", err)
	}
	return finder.MatchOne(repos, name)
}

// insensitiveRepoFilter prepends (?i) to a zoekt repo: filter so the lexical
// backend matches repo names case-insensitively, like the repo tools do.
func insensitiveRepoFilter(repo string) string {
	if repo == "" {
		return ""
	}
	return "(?i)" + repo
}

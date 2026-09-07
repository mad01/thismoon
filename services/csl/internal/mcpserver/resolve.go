package mcpserver

import (
	"fmt"
	"strings"

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

// resolveRepo finds a single repo by name. Returns an error if no match or ambiguous.
func resolveRepo(name string) (finder.Repo, error) {
	re, err := finder.CompileMatcher(name)
	if err != nil {
		return finder.Repo{}, err
	}
	normalized := finder.NormalizeQuery(name)

	cfg, err := config.Load()
	if err != nil {
		return finder.Repo{}, fmt.Errorf("load csl config: %w", err)
	}

	repos, err := cfg.DiscoverRepos()
	if err != nil {
		return finder.Repo{}, fmt.Errorf("walk repos: %w", err)
	}

	var matches []finder.Repo
	for _, r := range repos {
		if re.MatchString(r.Name) {
			matches = append(matches, r)
		}
	}

	if len(matches) == 0 {
		return finder.Repo{}, fmt.Errorf("no repo matching %q found locally", normalized)
	}
	if len(matches) > 1 {
		// Try exact match first
		for _, m := range matches {
			if strings.EqualFold(m.Name, normalized) {
				return m, nil
			}
		}
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = m.Name
		}
		return finder.Repo{}, fmt.Errorf(
			"ambiguous: %d repos match %q: %s — use the full org/repo name to disambiguate (e.g. %q)",
			len(matches),
			normalized,
			strings.Join(names, ", "),
			matches[0].Name,
		)
	}

	return matches[0], nil
}

// insensitiveRepoFilter prepends (?i) to a zoekt repo: filter so the lexical
// backend matches repo names case-insensitively, like the repo tools do.
func insensitiveRepoFilter(repo string) string {
	if repo == "" {
		return ""
	}
	return "(?i)" + repo
}

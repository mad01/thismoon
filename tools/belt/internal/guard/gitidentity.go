package guard

import (
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/notify"
)

// GitIdentityID identifies the git-user-email guard.
const GitIdentityID = "git-identity"

// GitIdentity blocks `git commit` when the repo's effective user.email does
// not match the email the config expects for that repo. Rules are matched by
// repo pattern (first match wins), so one fleet-shared config keeps the
// personal email on personal-org repos and the work email on work-host repos
// regardless of which machine the commit happens on. No git_identity config,
// an unresolved email, and a repo no rule covers all fail open — this guard
// exists to stop identity leaking across contexts, not to block commits on
// missing config.
type GitIdentity struct {
	cfg config.Config
	// resolveRepo returns the canonical host/owner/repo of the repo at dir,
	// or "" when it cannot be determined. Injectable for tests.
	resolveRepo func(dir string) string
	// resolveEmail returns the effective git user.email for the repo at dir,
	// or "" when git has none. Injectable for tests.
	resolveEmail func(dir string) string
	// emit sends the soft-mode warning to the events service. Injectable for
	// tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewGitIdentity builds the guard with the real git-backed resolvers.
func NewGitIdentity(cfg config.Config) *GitIdentity {
	return &GitIdentity{
		cfg:          cfg,
		resolveRepo:  func(dir string) string { return canonicalRepo(gitRemoteURL(dir)) },
		resolveEmail: gitUserEmail,
		emit:         notify.EmitEvent,
	}
}

func (g *GitIdentity) ID() string    { return GitIdentityID }
func (g *GitIdentity) Event() string { return EventBash }

// Check scans every git commit in the command and denies (or warns, in soft
// mode) when the repo's email does not match the first rule covering it.
func (g *GitIdentity) Check(in Input) *Denial {
	if len(g.cfg.GitIdentity) == 0 {
		return nil
	}
	for _, c := range findGitCommands(in.Command, "commit") {
		dir := c.dir
		if dir == "" {
			dir = in.Cwd
		}
		repo := g.resolveRepo(dir)
		rule, ok := g.matchRule(repo)
		if !ok {
			continue
		}
		email := g.resolveEmail(dir)
		if email == "" || email == rule.Email {
			continue
		}
		if rule.Soft() {
			g.emit("belt", "warn", "git-identity mismatch (soft)",
				"repo "+repo+" expects git user.email "+rule.Email+" but has "+email+" — commit allowed (soft mode)",
				map[string]string{"guard": GitIdentityID, "repo": repo})
			continue
		}
		return Reasonf(GitIdentityID,
			"Commit blocked. This repo (%s) expects git user.email %q but has %q — "+
				"committing with the wrong identity leaks it across contexts and is painful to fix after push. "+
				"Fix: git config user.email %q",
			repo, rule.Email, email, rule.Email)
	}
	return nil
}

// matchRule returns the first git_identity rule whose repos list (when set)
// matches the canonical repo. A rule with no repos list covers every repo,
// resolved or not.
func (g *GitIdentity) matchRule(repo string) (config.GitIdentity, bool) {
	for _, rule := range g.cfg.GitIdentity {
		if len(rule.Repos) == 0 || config.RepoMatches(rule.Repos, repo) {
			return rule, true
		}
	}
	return config.GitIdentity{}, false
}

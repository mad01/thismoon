package hint

import (
	"fmt"

	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/guard"
)

// CommitPolicyID identifies the commit-policy hint.
const CommitPolicyID = "commit-policy"

// mainBranches are the branches the hint watches — the same set the
// git-push-main guard denies pushes to.
var mainBranches = map[string]bool{"main": true, "master": true}

// CommitPolicy advises when a `git commit` lands on a repo's default branch.
// It is the post-commit half of the branch-discipline pair: the git-push-main
// guard denies the push, this hint catches the mistake earlier — right after
// the commit, while moving it to a branch is still a two-command fix. Repos
// where committing straight to main is the norm (a dotfiles repo, say) are
// exempted via hints.commit-policy.allow_repos, the same canonical
// host/owner/repo patterns the guard reads.
type CommitPolicy struct {
	cfg config.Config
	// resolveBranch returns the current branch of the repo at dir, or ""
	// when it cannot be determined. Injectable for tests.
	resolveBranch func(dir string) string
	// resolveRepo returns the canonical host/owner/repo identity of the
	// repo at dir, or "" when it has no resolvable origin. Injectable for
	// tests.
	resolveRepo func(dir string) string
}

// NewCommitPolicy builds the hint with the real git-backed resolvers.
func NewCommitPolicy(cfg config.Config) *CommitPolicy {
	return &CommitPolicy{
		cfg:           cfg,
		resolveBranch: gitCurrentBranch,
		resolveRepo:   guard.CanonicalRepoAt,
	}
}

func (h *CommitPolicy) ID() string    { return CommitPolicyID }
func (h *CommitPolicy) Event() string { return EventBash }

// Check fires when a commit in the command ran on main or master of a repo
// that is not allowlisted. A repo with no resolvable origin stays silent —
// unlike the push guard there is nothing upstream to protect, and a scratch
// `git init` repo lives its whole life on its default branch.
func (h *CommitPolicy) Check(in Input) *Advice {
	if in.Command == "" {
		return nil
	}
	allow := h.cfg.Hints[CommitPolicyID].AllowRepos
	for _, dir := range guard.GitCommitDirs(in.Command) {
		if dir == "" {
			dir = in.Cwd
		}
		branch := h.resolveBranch(dir)
		if !mainBranches[branch] {
			continue
		}
		repo := h.resolveRepo(dir)
		if repo == "" {
			continue
		}
		if config.RepoMatches(allow, repo) {
			continue
		}
		return &Advice{Hint: CommitPolicyID, Text: commitAdvice(repo, branch)}
	}
	return nil
}

// commitAdvice names the violation and hands back the exact recovery, so the
// model can act without guessing: the branch switch carries the commit along
// and the branch -f drops the local default branch back onto the remote.
func commitAdvice(repo, branch string) string {
	return fmt.Sprintf(
		"this commit landed on %s of %s, and %s is not on the direct-%s allowlist — "+
			"the commit policy there is feature branch + PR. Move it before pushing: "+
			"git switch -c <branch> (the commit comes along), then git branch -f %s origin/%s. "+
			"If direct commits are actually the norm in this repo, add it to "+
			"hints.commit-policy.allow_repos instead.",
		branch, repo, repo, branch, branch, branch)
}

// gitCurrentBranch shells out to git; "" when dir is empty, not a repo, or
// git fails — all of which degrade to silence.
func gitCurrentBranch(dir string) string {
	return gitLine(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

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
// opted out via hints.commit-policy.exclude_repos, the same canonical
// host/owner/repo patterns the guard's allow_repos reads.
type CommitPolicy struct {
	cfg config.Config
	// resolveBranch returns the current branch of the repo at dir, or ""
	// when it cannot be determined. Injectable for tests.
	resolveBranch func(dir string) string
	// resolveRepo returns the canonical host/owner/repo identity of the
	// repo at dir, or "" when it has no resolvable origin. Injectable for
	// tests.
	resolveRepo func(dir string) string
	// resolveRoot returns the working-tree root of the repo at dir, where
	// the .belt.yaml overlay lives, or "" outside a repo. Injectable for
	// tests.
	resolveRoot func(dir string) string
}

// NewCommitPolicy builds the hint with the real git-backed resolvers.
func NewCommitPolicy(cfg config.Config) *CommitPolicy {
	return &CommitPolicy{
		cfg:           cfg,
		resolveBranch: gitCurrentBranch,
		resolveRepo:   guard.CanonicalRepoAt,
		resolveRoot:   gitTopLevel,
	}
}

func (h *CommitPolicy) ID() string    { return CommitPolicyID }
func (h *CommitPolicy) Event() string { return EventBash }

// Check fires when a commit in the command ran on a protected branch of a
// repo that is not opted out. The default protected set is main/master; a
// repo-local .belt.yaml overlay can replace the set, opt the repo out (or
// back in over the machine's exclude_repos), and append its own message
// (docs/adr/0012). A repo with no resolvable origin stays silent — unlike
// the push guard there is nothing upstream to protect, and a scratch
// `git init` repo lives its whole life on its default branch.
func (h *CommitPolicy) Check(in Input) *Advice {
	if in.Command == "" {
		return nil
	}
	for _, dir := range guard.GitCommitDirs(in.Command) {
		if dir == "" {
			dir = in.Cwd
		}
		overlay, err := loadRepoOverlay(h.resolveRoot(dir))
		if err != nil {
			// Fail visible, never deny: a broken repo file draws one
			// line on every commit there until it is fixed.
			return &Advice{Hint: CommitPolicyID, Text: fmt.Sprintf(
				"could not evaluate this repo's commit policy: %v — fix or remove the file.", err)}
		}
		local := overlay.Hints[CommitPolicyID]
		branch := h.resolveBranch(dir)
		if len(local.ProtectedBranches) > 0 {
			if !branchMatches(local.ProtectedBranches, branch) {
				continue
			}
		} else if !mainBranches[branch] {
			continue
		}
		repo := h.resolveRepo(dir)
		if repo == "" {
			continue
		}
		if local.Exclude != nil {
			if *local.Exclude {
				continue
			}
		} else if h.cfg.DirectMain(repo) || h.cfg.HintRepoExcluded(CommitPolicyID, repo) {
			// direct_main_repos states the workflow this hint advises
			// about, so it silences the advice; commit-policy is the
			// list's only hint-side reader (docs/adr/0013).
			continue
		}
		return &Advice{Hint: CommitPolicyID, Text: commitAdvice(repo, branch, local.Message)}
	}
	return nil
}

// commitAdvice names the violation and hands back the exact recovery, so the
// model can act without guessing: the branch switch carries the commit along
// and the branch -f drops the local default branch back onto the remote. A
// repo-authored overlay message is appended when present.
func commitAdvice(repo, branch, message string) string {
	s := fmt.Sprintf(
		"this commit landed on %s of %s, and %s is not opted out of the commit policy — "+
			"changes there go feature branch + PR. Move it before pushing: "+
			"git switch -c <branch> (the commit comes along), then git branch -f %s origin/%s. "+
			"If direct commits are actually this repo's workflow, add it to "+
			"the top-level direct_main_repos list instead.",
		branch, repo, repo, branch, branch)
	if message != "" {
		s += " Repo policy: " + message
	}
	return s
}

// gitCurrentBranch shells out to git; "" when dir is empty, not a repo, or
// git fails — all of which degrade to silence.
func gitCurrentBranch(dir string) string {
	return gitLine(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

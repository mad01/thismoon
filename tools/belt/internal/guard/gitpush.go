package guard

import (
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// GitPushMainID identifies the git-push-to-default-branch guard.
const GitPushMainID = "git-push-main"

var defaultBranches = map[string]bool{"main": true, "master": true}

// GitPushMain blocks `git push` to main/master unless the target repository
// is on the guard's allow_repos allowlist. A machine class where direct
// pushes are fine (a personal machine, say) disables the guard in its
// rendered config; everywhere else, and on a machine with no config at all,
// pushes to the default branch are denied (docs/adr/0010).
type GitPushMain struct {
	cfg config.Config
	// resolveBranch returns the current branch of the repo at dir, or ""
	// when it cannot be determined. Injectable for tests.
	resolveBranch func(dir string) string
	// resolveRepo returns the canonical host/owner/repo of the repo at dir
	// (e.g. github.com/mad01/dotfiles), or "" when it cannot be determined.
	// Injectable for tests.
	resolveRepo func(dir string) string
}

// NewGitPushMain builds the guard with the real git-backed resolvers.
func NewGitPushMain(cfg config.Config) *GitPushMain {
	return &GitPushMain{
		cfg:           cfg,
		resolveBranch: gitCurrentBranch,
		resolveRepo:   CanonicalRepoAt,
	}
}

func (g *GitPushMain) ID() string    { return GitPushMainID }
func (g *GitPushMain) Event() string { return EventBash }

// Check scans every git push in the command (compound commands included) and
// denies when one targets main or master.
func (g *GitPushMain) Check(in Input) *Denial {
	for _, push := range findGitPushes(in.Command) {
		dir := push.dir
		if dir == "" {
			dir = in.Cwd
		}
		branch := push.targetBranch(func(string) string { return g.resolveBranch(dir) })
		if !defaultBranches[branch] {
			continue
		}
		if g.pushExempt(dir) {
			continue
		}
		return Reasonf(GitPushMainID,
			"pushing to %q is blocked on this machine (direct pushes to the default branch are never allowed here). "+
				"Create a feature branch and open a PR instead: git checkout -b <branch> && git push -u origin <branch>.",
			branch)
	}
	return nil
}

// pushExempt reports whether the repo at dir may take a direct default-
// branch push: the shared direct_main_repos list (this guard is one of its
// two readers, docs/adr/0013) or the guard's own allow_repos. Empty lists
// and unresolved repos fail closed: only an explicit, successfully-resolved
// match exempts a push. Patterns match as they do everywhere else in the
// config — exactly, or by trailing "/*" org wildcard.
func (g *GitPushMain) pushExempt(dir string) bool {
	repo := g.resolveRepo(dir)
	if repo == "" {
		return false
	}
	return g.cfg.DirectMain(repo) ||
		config.RepoMatches(g.cfg.Guards[GitPushMainID].AllowRepos, repo)
}

// gitPush is one parsed `git push` invocation.
type gitPush struct {
	dir      string   // from git -C <dir>, if given
	refspecs []string // positional args after the remote
	explicit bool     // true when at least one refspec was given
}

// targetBranch resolves which branch the push lands on. For explicit refspecs
// the destination side (after ':') decides; bare pushes and HEAD resolve to
// the current branch of the repo.
func (p gitPush) targetBranch(currentBranch func(dir string) string) string {
	if !p.explicit {
		return currentBranch(p.dir)
	}
	for _, spec := range p.refspecs {
		spec = strings.TrimPrefix(spec, "+")
		if i := strings.LastIndex(spec, ":"); i >= 0 {
			spec = spec[i+1:]
		}
		spec = strings.TrimPrefix(spec, "refs/heads/")
		if spec == "HEAD" {
			spec = currentBranch(p.dir)
		}
		if defaultBranches[spec] {
			return spec
		}
	}
	return ""
}

// pushFlagsWithValue are `git push` flags that consume the next token, so the
// token must not be mistaken for a remote or refspec. --force-with-lease is
// absent on purpose: it takes the = form only.
var pushFlagsWithValue = map[string]bool{
	"-o": true, "--push-option": true, "--receive-pack": true, "--exec": true,
	"--repo": true,
}

// findGitPushes extracts git push invocations from a shell command via the
// shared token-based git parser (see findGitCommands).
func findGitPushes(command string) []gitPush {
	var pushes []gitPush
	for _, c := range findGitCommands(command, "push") {
		pushes = append(pushes, parsePushArgs(c))
	}
	return pushes
}

// parsePushArgs reads the remote and refspecs out of a parsed push.
func parsePushArgs(c gitInvocation) gitPush {
	push := gitPush{dir: c.dir}
	var positional []string
	for i := 0; i < len(c.args); i++ {
		tok := c.args[i]
		if strings.HasPrefix(tok, "-") {
			if pushFlagsWithValue[tok] {
				i++
			}
			continue
		}
		positional = append(positional, tok)
	}
	// First positional is the remote; the rest are refspecs.
	if len(positional) > 1 {
		push.refspecs = positional[1:]
		push.explicit = true
	}
	return push
}

// segment is one piece of a compound shell command, with its byte offset in
// the original command so callers can scan from the segment onward.
type segment struct {
	text string
	off  int
}

// splitSegments splits a compound shell command on &&, ||, ;, | and newlines.
// Separators are replaced by same-length filler so offsets stay valid.
func splitSegments(command string) []segment {
	replaced := command
	for _, sep := range []string{"&&", "||", ";", "|", "\n"} {
		replaced = strings.ReplaceAll(replaced, sep, strings.Repeat("\x00", len(sep)))
	}
	var segs []segment
	pos := 0
	for _, piece := range strings.Split(replaced, "\x00") {
		if strings.TrimSpace(piece) != "" {
			segs = append(segs, segment{text: piece, off: pos})
		}
		pos += len(piece) + 1
	}
	return segs
}

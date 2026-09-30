package guard

import (
	"path"
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
// denies when one targets main or master. It walks the command's segments in
// order, tracking the shell's working directory across `cd`/`pushd`, so a bare
// push (no `git -C`) is judged against the directory the push runs in rather
// than the session cwd. Without that, `cd <other-repo> && git push origin
// main` from an exempt repo's cwd would be checked against the exempt repo and
// wrongly allowed. A cd whose target cannot be resolved without a shell leaves
// the directory unknown, and checkPush then fails closed instead of guessing.
func (g *GitPushMain) Check(in Input) *Denial {
	cwd := in.Cwd
	for _, seg := range splitSegments(in.Command) {
		tokens := strings.Fields(seg.text)
		if dir, ok := parseCd(tokens); ok {
			cwd = resolveDir(cwd, dir)
			continue
		}
		for _, push := range segmentPushes(seg) {
			dir := push.dir
			if dir == "" {
				dir = cwd
			}
			if d := g.checkPush(push, dir); d != nil {
				return d
			}
		}
	}
	return nil
}

// checkPush judges one push against the directory it runs in. An empty dir
// means that directory could not be determined, and the push is handed to
// unknownDirDenial rather than resolved against a guessed location.
func (g *GitPushMain) checkPush(push gitPush, dir string) *Denial {
	if dir == "" {
		return unknownDirDenial(push)
	}
	branch := push.targetBranch(func() string { return g.resolveBranch(dir) })
	if !defaultBranches[branch] || g.pushExempt(dir) {
		return nil
	}
	return Reasonf(
		GitPushMainID,
		"pushing to %q is blocked on this machine (direct pushes to the default branch are never allowed here). "+
			"Create a feature branch and open a PR instead: git checkout -b <branch> && git push -u origin <branch>.",
		branch,
	)
}

// unknownDirDenial decides a push whose directory could not be determined:
// the cd before it had a target only a shell can resolve. Neither the current
// branch nor the repo can be resolved without running git somewhere it does
// not belong, so the push is denied unless its refspecs name a non-default
// branch outright, which is safe wherever it runs.
func unknownDirDenial(push gitPush) *Denial {
	needsBranch := false
	branch := push.targetBranch(func() string {
		needsBranch = true
		return ""
	})
	if !needsBranch && !defaultBranches[branch] {
		return nil
	}
	return Reasonf(
		GitPushMainID,
		"cannot tell which repo this push runs in: the cd before it has a target only a shell can "+
			"resolve (a variable, cd -, a substitution). Use an absolute path, ~, or $HOME in the cd, "+
			"or git -C <dir> on the push, so the target repo can be checked.",
	)
}

// parseCd reports whether a segment's tokens are a `cd`/`pushd` invocation and,
// if so, returns its target as written. A cd with no argument goes home, so it
// returns "~"; resolveDir decides what any target means.
func parseCd(tokens []string) (string, bool) {
	if len(tokens) == 0 || (tokens[0] != "cd" && tokens[0] != "pushd") {
		return "", false
	}
	// Skip cd option flags (`cd -P`, `cd -L`); the first non-flag token is the
	// target.
	for _, tok := range tokens[1:] {
		if tok == "-P" || tok == "-L" || tok == "-e" || tok == "-@" {
			continue
		}
		return tok, true
	}
	return "~", true
}

// unresolvableChars are the shell characters a cd target may still carry
// after belt's own expansion. Any of them means a shell would rewrite the
// target into something belt cannot see, so the destination is unknown.
const unresolvableChars = "$*?~`\"'"

// resolveDir applies a `cd` target to the current directory and returns the
// new one, or "" for a destination it cannot determine statically: `cd -`, a
// target expandCdTarget rejects, or a relative target from an unknown cwd.
// An unknown cwd is what makes a later push fail closed (see checkPush).
func resolveDir(cwd, target string) string {
	if target == "-" {
		return ""
	}
	target = expandCdTarget(target)
	switch {
	case target == "":
		return ""
	case path.IsAbs(target):
		return path.Clean(target)
	case cwd == "":
		return ""
	}
	return path.Clean(path.Join(cwd, target))
}

// expandCdTarget performs the expansions the shell would on a cd target, as
// far as they can be done without a shell: a surrounding quote pair is
// dropped, a leading $HOME or ${HOME} becomes the home directory unless
// single-quoted, and so does an unquoted leading ~. A target that still
// carries a shell metacharacter or a stray quote afterwards returns "".
func expandCdTarget(target string) string {
	quote, inner := unquote(target)
	if quote != '\'' {
		inner = expandHomeVar(inner)
	}
	if quote == 0 {
		inner = config.ExpandHome(inner)
	}
	if strings.ContainsAny(inner, unresolvableChars) {
		return ""
	}
	return inner
}

// unquote strips one pair of matching surrounding quotes and reports which
// quote it was, 0 for none.
func unquote(s string) (byte, string) {
	n := len(s)
	if n >= 2 && (s[0] == '"' || s[0] == '\'') && s[n-1] == s[0] {
		return s[0], s[1 : n-1]
	}
	return 0, s
}

// expandHomeVar rewrites a leading $HOME or ${HOME} to the home directory,
// through the same helper a leading ~ goes through. A longer variable
// ($HOMEBREW_PREFIX, say) is left alone for the metacharacter check to
// reject.
func expandHomeVar(target string) string {
	for _, v := range []string{"${HOME}", "$HOME"} {
		rest, ok := strings.CutPrefix(target, v)
		if ok && (rest == "" || rest[0] == '/') {
			return config.ExpandHome("~" + rest)
		}
	}
	return target
}

// segmentPushes extracts the git push invocations from a single segment,
// reusing the shared token parser on just that segment's text.
func segmentPushes(seg segment) []gitPush {
	return findGitPushes(seg.text)
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
	remote   string   // first positional; "" for a bare push
	refspecs []string // positional args after the remote
	explicit bool     // true when at least one refspec was given
	// The sweep and delete flags decide what publish-internal-names scans:
	// --all/--mirror push every branch, --tags every tag, and --delete
	// removes a ref rather than publishing one.
	all, tags, mirror, del bool
}

// targetBranch resolves which branch the push lands on. For explicit refspecs
// the destination side (after ':') decides; bare pushes and HEAD ask
// currentBranch for the branch checked out in the push directory.
func (p gitPush) targetBranch(currentBranch func() string) string {
	if !p.explicit {
		return currentBranch()
	}
	for _, spec := range p.refspecs {
		spec = strings.TrimPrefix(spec, "+")
		if i := strings.LastIndex(spec, ":"); i >= 0 {
			spec = spec[i+1:]
		}
		spec = strings.TrimPrefix(spec, "refs/heads/")
		if spec == "HEAD" {
			spec = currentBranch()
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
			switch tok {
			case "--all", "--branches":
				push.all = true
			case "--tags":
				push.tags = true
			case "--mirror":
				push.mirror = true
			case "--delete", "-d":
				push.del = true
			}
			if pushFlagsWithValue[tok] {
				i++
			}
			continue
		}
		positional = append(positional, tok)
	}
	// First positional is the remote; the rest are refspecs.
	if len(positional) > 0 {
		push.remote = positional[0]
	}
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

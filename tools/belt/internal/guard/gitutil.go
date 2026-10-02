package guard

import (
	"os/exec"
	"path"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// gitInvocation is one parsed git invocation from a shell command: the
// directory it runs in, the subcommand, and the tokens after it. dir is ""
// when that directory cannot be told without a shell (see parseGitCmd).
type gitInvocation struct {
	dir  string
	sub  string
	args []string
}

// gitCommandsAt extracts bare `git <sub>` invocations from a shell command,
// compound commands included, each with the directory it runs in. cwd is
// where a bare invocation runs; a `git -C` target is resolved against it.
// The parser is deliberately token-based (see splitSegments): quoted strings
// containing the words do not tokenize to a bare `git` and are ignored.
func gitCommandsAt(command, cwd, sub string) []gitInvocation {
	var cmds []gitInvocation
	for _, seg := range splitSegments(command) {
		if c, ok := segmentGitCommand(strings.Fields(seg.text), cwd); ok && c.sub == sub {
			cmds = append(cmds, c)
		}
	}
	return cmds
}

// segmentGitCommand parses the first bare `git` in one segment's tokens: one
// invocation per segment.
func segmentGitCommand(tokens []string, cwd string) (gitInvocation, bool) {
	for i, tok := range tokens {
		if tok == "git" {
			return parseGitCmd(tokens[i+1:], cwd)
		}
	}
	return gitInvocation{}, false
}

// parseGitCmd parses the tokens after `git`: global flags first, then the
// subcommand and its arguments. cwd is the directory the shell is in when
// git runs. A -C target is resolved against it exactly as a cd target would
// be (resolveDir), so `git -C ~/repo`, `git -C $HOME/repo`, and the absolute
// path all name one directory, and a target only a shell can expand leaves
// dir "" rather than a path git would never see. Repeated -C flags chain the
// way git chains them. git takes -C only as a separate token, so there is no
// = form to parse. ok is false when no subcommand follows the flags.
func parseGitCmd(tokens []string, cwd string) (gitInvocation, bool) {
	c := gitInvocation{dir: cwd}
	i := 0
	for i < len(tokens) {
		switch {
		case tokens[i] == "-C" && i+1 < len(tokens):
			c.dir = resolveDir(c.dir, tokens[i+1])
			i += 2
		case tokens[i] == "-c" && i+1 < len(tokens):
			i += 2
		case tokens[i] == "--git-dir" || tokens[i] == "--work-tree":
			i += 2 // separate-value form consumes the path token too
		case strings.HasPrefix(tokens[i], "--git-dir=") || strings.HasPrefix(tokens[i], "--work-tree="):
			i++
		default:
			c.sub = tokens[i]
			c.args = tokens[i+1:]
			return c, true
		}
	}
	return gitInvocation{}, false
}

// unresolvableChars are the shell characters a directory target may still
// carry after belt's own expansion. Any of them means a shell would rewrite
// the target into something belt cannot see, so the destination is unknown.
const unresolvableChars = "$*?~`\"'"

// resolveDir applies a `cd` or `git -C` target to the current directory and
// returns the new one, or "" for a destination it cannot determine
// statically: `cd -`, a target expandCdTarget rejects, or a relative target
// from an unknown cwd. An unknown directory is what makes a later push fail
// closed (see GitPushMain.checkPush) and a commit guard or hint stay quiet.
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

// expandCdTarget performs the expansions the shell would on a directory
// target, as far as they can be done without a shell: a surrounding quote
// pair is dropped, a leading $HOME or ${HOME} becomes the home directory
// unless single-quoted, and so does an unquoted leading ~. A target that
// still carries a shell metacharacter or a stray quote afterwards returns "".
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

// canonicalRepo normalizes a git remote URL to "host/owner/repo"
// (github.com/mad01/dotfiles for both the SSH and HTTPS forms). It returns ""
// for anything it cannot resolve — an empty remote, a bare local path — so an
// unresolved repo fails closed against the allowlist.
func canonicalRepo(remote string) string {
	remote = strings.TrimSpace(remote)
	remote = strings.TrimSuffix(remote, ".git")
	switch {
	case strings.Contains(remote, "://"):
		// scheme://[user@]host[:port]/owner/repo
		remote = remote[strings.Index(remote, "://")+3:]
		if i := strings.LastIndex(remote, "@"); i >= 0 {
			remote = remote[i+1:]
		}
	case strings.Contains(remote, "@") && strings.Contains(remote, ":"):
		// scp-like: [user@]host:owner/repo
		remote = remote[strings.Index(remote, "@")+1:]
		remote = strings.Replace(remote, ":", "/", 1)
	default:
		return ""
	}
	// Drop a :port from the host segment.
	if i := strings.Index(remote, "/"); i >= 0 {
		if j := strings.Index(remote[:i], ":"); j >= 0 {
			remote = remote[:j] + remote[i:]
		}
	}
	// Need at least host/owner/repo.
	if strings.Count(remote, "/") < 2 {
		return ""
	}
	return remote
}

// gitCurrentBranch shells out to git; "" when dir is not a repo or git fails.
func gitCurrentBranch(dir string) string {
	return gitOutput(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// gitRemoteURL shells out to git; "" when dir is outside a repo or the repo
// has no origin remote.
func gitRemoteURL(dir string) string {
	return gitOutput(dir, "remote", "get-url", "origin")
}

// gitUserEmail returns the effective user.email for the repo at dir (local
// config or global fallback, exactly what a commit there would record); ""
// when git has no identity configured or dir is not a repo.
func gitUserEmail(dir string) string {
	return gitOutput(dir, "config", "user.email")
}

// gitOutput runs one git command in dir and returns its trimmed stdout, or ""
// on any failure. A guard helper degrades to "unknown", never errors, and
// never guesses: an empty dir means the caller could not tell where the
// command runs, and running git in belt's own process directory instead would
// judge the session's repo in place of the target one. That is how a `cd` with
// a target only a shell can resolve once let a push into a non-exempt repo
// ride an exempt session cwd.
func gitOutput(dir string, args ...string) string {
	if dir == "" {
		return ""
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GitCommitDirs returns the directory every `git commit` in a shell command
// runs in: the `git -C` target when one is given, resolved as parseGitCmd
// does, else cwd. An entry is "" when the directory cannot be told without a
// shell. Exported for the commit-policy and lint-policy hints, which watch
// the same invocations the commit guards check.
func GitCommitDirs(command, cwd string) []string {
	var dirs []string
	for _, c := range gitCommandsAt(command, cwd, "commit") {
		dirs = append(dirs, c.dir)
	}
	return dirs
}

// CanonicalRepoAt resolves the repo at dir to the canonical host/owner/repo
// identity allow_repos patterns match, or "" when there is no resolvable
// origin remote.
func CanonicalRepoAt(dir string) string {
	return canonicalRepo(gitRemoteURL(dir))
}

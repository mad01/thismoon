package guard

import (
	"os/exec"
	"strings"
)

// gitInvocation is one parsed git invocation from a shell command: which repo dir it
// runs against (from git -C, if given), the subcommand, and the tokens after
// it.
type gitInvocation struct {
	dir  string
	sub  string
	args []string
}

// findGitCommands extracts bare `git <sub>` invocations from a shell command,
// compound commands included. The parser is deliberately token-based (see
// splitSegments): quoted strings containing the words do not tokenize to a
// bare `git` and are ignored.
func findGitCommands(command, sub string) []gitInvocation {
	var cmds []gitInvocation
	for _, seg := range splitSegments(command) {
		tokens := strings.Fields(seg.text)
		for i := 0; i < len(tokens); i++ {
			if tokens[i] != "git" {
				continue
			}
			if c, ok := parseGitCmd(tokens[i+1:]); ok && c.sub == sub {
				cmds = append(cmds, c)
			}
			break // one git invocation per segment
		}
	}
	return cmds
}

// parseGitCmd parses the tokens after `git`: global flags first (-C captures
// the repo dir), then the subcommand and its arguments. ok is false when no
// subcommand follows the flags.
func parseGitCmd(tokens []string) (gitInvocation, bool) {
	var c gitInvocation
	i := 0
	for i < len(tokens) {
		switch {
		case tokens[i] == "-C" && i+1 < len(tokens):
			c.dir = tokens[i+1]
			i += 2
		case tokens[i] == "-c" && i+1 < len(tokens):
			i += 2
		case tokens[i] == "--git-dir" || tokens[i] == "--work-tree":
			i += 2 // separate-value form consumes the path token too
		case strings.HasPrefix(tokens[i], "--git-dir=") || strings.HasPrefix(tokens[i], "--work-tree="):
			i++
		default:
			if i >= len(tokens) {
				return gitInvocation{}, false
			}
			c.sub = tokens[i]
			c.args = tokens[i+1:]
			return c, true
		}
	}
	return gitInvocation{}, false
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
// on any failure — a guard helper must degrade to "unknown", never error.
func gitOutput(dir string, args ...string) string {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// GitCommitDirs returns the `git -C` directory of every `git commit` in a
// shell command, "" for invocations that run in the caller's cwd. Exported
// for the commit-policy hint, which watches the same invocations the commit
// guards check.
func GitCommitDirs(command string) []string {
	var dirs []string
	for _, c := range findGitCommands(command, "commit") {
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

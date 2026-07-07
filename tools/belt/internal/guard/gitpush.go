package guard

import (
	"os/exec"
	"strings"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// GitPushMainID identifies the git-push-to-default-branch guard.
const GitPushMainID = "git-push-main"

var defaultBranches = map[string]bool{"main": true, "master": true}

// GitPushMain blocks `git push` to main/master unless the machine runs the
// personal ralph profile. On the work profile (or when the profile is
// unknown — fail closed) direct pushes to the default branch are denied.
type GitPushMain struct {
	cfg config.Config
	// resolveBranch returns the current branch of the repo at dir, or ""
	// when it cannot be determined. Injectable for tests.
	resolveBranch func(dir string) string
}

// NewGitPushMain builds the guard with the real git-backed branch resolver.
func NewGitPushMain(cfg config.Config) *GitPushMain {
	return &GitPushMain{cfg: cfg, resolveBranch: gitCurrentBranch}
}

func (g *GitPushMain) ID() string    { return GitPushMainID }
func (g *GitPushMain) Event() string { return EventBash }

// Check scans every git push in the command (compound commands included) and
// denies when one targets main or master.
func (g *GitPushMain) Check(in Input) *Denial {
	if g.cfg.HasProfile("personal") {
		return nil
	}
	for _, push := range findGitPushes(in.Command) {
		branch := push.targetBranch(func(dir string) string {
			if dir == "" {
				dir = in.Cwd
			}
			return g.resolveBranch(dir)
		})
		if defaultBranches[branch] {
			return Reasonf(GitPushMainID,
				"pushing to %q is blocked on this machine (work profile — direct pushes to the default branch are never allowed here). "+
					"Create a feature branch and open a PR instead: git checkout -b <branch> && git push -u origin <branch>.",
				branch)
		}
	}
	return nil
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

// findGitPushes extracts git push invocations from a shell command. The
// parser is deliberately token-based: it splits on shell separators and
// matches bare `git ... push` sequences. Quoted strings containing the words
// do not tokenize to a bare `git` and are ignored.
func findGitPushes(command string) []gitPush {
	var pushes []gitPush
	for _, seg := range splitSegments(command) {
		tokens := strings.Fields(seg.text)
		for i := 0; i < len(tokens); i++ {
			if tokens[i] != "git" {
				continue
			}
			push, ok := parseGitInvocation(tokens[i+1:])
			if ok {
				pushes = append(pushes, push)
			}
			break // one git invocation per segment
		}
	}
	return pushes
}

// parseGitInvocation parses the tokens after `git`; ok is false when the
// subcommand is not push.
func parseGitInvocation(tokens []string) (gitPush, bool) {
	var push gitPush
	i := 0
	// Global git flags before the subcommand.
	for i < len(tokens) {
		switch {
		case tokens[i] == "-C" && i+1 < len(tokens):
			push.dir = tokens[i+1]
			i += 2
		case tokens[i] == "-c" && i+1 < len(tokens):
			i += 2
		case tokens[i] == "--git-dir" || tokens[i] == "--work-tree":
			i += 2 // separate-value form consumes the path token too
		case strings.HasPrefix(tokens[i], "--git-dir=") || strings.HasPrefix(tokens[i], "--work-tree="):
			i++
		default:
			goto subcommand
		}
	}
subcommand:
	if i >= len(tokens) || tokens[i] != "push" {
		return gitPush{}, false
	}
	i++
	var positional []string
	for ; i < len(tokens); i++ {
		tok := tokens[i]
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
	return push, true
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

// gitCurrentBranch shells out to git; "" when dir is not a repo or git fails.
func gitCurrentBranch(dir string) string {
	cmd := exec.Command("git", "rev-parse", "--abbrev-ref", "HEAD")
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

package guard

import (
	"os/exec"
	"slices"
	"strings"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// GitDiscardID identifies the guard against discarding uncommitted work.
const GitDiscardID = "git-discard"

// gitDiscardReason is what the model reads on a deny. It names the one
// situation the command is fine in and the two ways forward otherwise.
const gitDiscardReason = "this discards uncommitted work. Only run it when the user's latest " +
	"message explicitly asked for it; otherwise ask first, or let the user run it with `! <command>`."

// GitDiscard blocks git commands that throw away uncommitted work: a stash
// push, `reset --hard`, a checkout or restore over the worktree, a forced
// clean. A session audit found agents running these over the user's own
// edits without being asked, and the edits are not in any reflog. A clean
// working tree has nothing to lose, so the guard allows there; allow_repos
// exempts repos by canonical identity, and `mode: soft` warns instead of
// denying.
type GitDiscard struct {
	cfg config.Config
	// resolveRepo returns the canonical host/owner/repo of the repo at dir,
	// or "" when it cannot be determined. Injectable for tests.
	resolveRepo func(dir string) string
	// status returns `git status --porcelain` output for the repo at dir
	// (extra args appended) and whether git succeeded. Injectable for tests.
	status func(dir string, extra ...string) (string, bool)
	// emit sends the soft-mode warning to the events service. Injectable
	// for tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewGitDiscard builds the guard with the real git and events backends.
func NewGitDiscard(cfg config.Config) *GitDiscard {
	return &GitDiscard{
		cfg:         cfg,
		resolveRepo: CanonicalRepoAt,
		status:      gitStatusPorcelain,
		emit:        notify.EmitEventSync,
	}
}

func (g *GitDiscard) ID() string    { return GitDiscardID }
func (g *GitDiscard) Event() string { return EventBash }

// Check judges every discarding git invocation in the command in the
// directory it runs in (the same cd/pushd and `git -C` tracking the other
// git guards use). An invocation whose directory only a shell can name is
// denied: there is no tree to check for changes.
func (g *GitDiscard) Check(in Input) *Denial {
	var denial *Denial
	walkSegments(in.Command, in.Cwd, func(tokens []string, dir string) {
		if denial != nil {
			return
		}
		c, ok := segmentGitCommand(tokens, dir)
		if !ok || !discards(c) {
			return
		}
		denial = g.checkDiscard(c)
	})
	return denial
}

// checkDiscard decides one discarding invocation: exempt repo or clean tree
// allows, soft mode warns, anything else denies.
func (g *GitDiscard) checkDiscard(c gitInvocation) *Denial {
	if c.dir != "" {
		if g.cfg.RepoAllowed(GitDiscardID, g.resolveRepo(c.dir)) {
			return nil
		}
		if out, ok := g.status(c.dir, statusExtra(c)...); ok && out == "" {
			return nil
		}
	}
	if g.cfg.Guards[GitDiscardID].Soft() {
		g.emit(
			"belt",
			"warn",
			"git-discard (soft)",
			"belt["+GitDiscardID+"]: git "+c.sub+" "+strings.Join(c.args, " ")+
				" would discard uncommitted work. Allowed (soft mode).",
			map[string]string{"guard": GitDiscardID, "dir": c.dir},
		)
		return nil
	}
	return Reasonf(GitDiscardID, "%s", gitDiscardReason)
}

// statusExtra widens the clean-tree check for `git clean -x`/`-X`, which
// also deletes ignored files that plain `git status --porcelain` never
// lists.
func statusExtra(c gitInvocation) []string {
	if c.sub == "clean" && slices.ContainsFunc(c.args, func(a string) bool {
		return shortFlagHas(a, 'x') || shortFlagHas(a, 'X')
	}) {
		return []string{"--ignored"}
	}
	return nil
}

// discards reports whether one git invocation throws away uncommitted work.
func discards(c gitInvocation) bool {
	switch c.sub {
	case "stash":
		return stashDiscards(c.args)
	case "reset":
		return slices.Contains(c.args, "--hard")
	case "checkout":
		return checkoutDiscards(c.args)
	case "switch":
		return hasFlag(c.args, "--discard-changes", "--force", "-f")
	case "restore":
		return restoreDiscards(c.args)
	case "clean":
		return cleanDiscards(c.args)
	}
	return false
}

// stashKeeps are the stash subcommands that leave the working tree alone
// or only add to it. drop and clear remove stash entries, not worktree
// changes, and are out of this guard's scope.
var stashKeeps = []string{"list", "show", "apply", "pop", "branch", "drop", "clear", "create", "store"}

// stashDiscards: a bare `git stash`, a flag-led one (`git stash -u`), and
// the push/save subcommands all move worktree changes off the tree.
func stashDiscards(args []string) bool {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return true
	}
	return !slices.Contains(stashKeeps, args[0])
}

// checkoutDiscards: a path checkout (`checkout -- <path>`, `checkout <ref>
// -- <path>`, `checkout .`) overwrites the worktree copy, and a forced
// branch switch drops local changes on the way.
func checkoutDiscards(args []string) bool {
	if i := slices.Index(args, "--"); i >= 0 && i+1 < len(args) {
		return true
	}
	return slices.Contains(args, ".") || hasFlag(args, "--force", "-f")
}

// restoreDiscards: restore touches the worktree unless only --staged is
// given; `--staged --worktree` touches both.
func restoreDiscards(args []string) bool {
	staged := hasFlag(args, "--staged", "-S")
	worktree := hasFlag(args, "--worktree", "-W")
	return !staged || worktree
}

// cleanDiscards: clean deletes only with -f (git refuses otherwise under
// the default clean.requireForce), and a dry run deletes nothing.
func cleanDiscards(args []string) bool {
	force, dry := false, false
	for _, a := range args {
		force = force || a == "--force" || shortFlagHas(a, 'f')
		dry = dry || a == "--dry-run" || shortFlagHas(a, 'n')
	}
	return force && !dry
}

// shortFlagHas reports whether a short-flag token (`-f`, `-fdx`) carries
// the flag letter.
func shortFlagHas(tok string, flag byte) bool {
	return len(tok) > 1 && tok[0] == '-' && tok[1] != '-' && strings.IndexByte(tok[1:], flag) >= 0
}

// gitStatusPorcelain runs `git status --porcelain` in dir. ok is false when
// dir is empty or git fails (not a repo), which the guard treats as dirty.
func gitStatusPorcelain(dir string, extra ...string) (string, bool) {
	if dir == "" {
		return "", false
	}
	cmd := exec.Command("git", append([]string{"status", "--porcelain"}, extra...)...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

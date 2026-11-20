package guard

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/kit/gitloc"
	"github.com/mad01/thismoon/tools/belt/internal/config"
	"github.com/mad01/thismoon/tools/belt/internal/cslindex"
)

// MainCheckoutID identifies the guard that keeps a repo's main checkout
// read-only.
const MainCheckoutID = "main-checkout"

// MainCheckout keeps a repo's main checkout on its default branch with no
// local changes, and points the change at a linked worktree instead. An edit
// made in a main checkout collides with every other session on that repo,
// and a checkout left dirty or on another branch is one the next sync skips.
// On the write event it judges an Edit or Write of a file inside a main
// checkout; on the bash event, a git command that changes one
// (changesCheckout). Read-only git, `git worktree add` and a fast-forward
// pull of the default branch (pullSyncs) still run there, so sync keeps
// working. A linked worktree is never judged. One id on two events, like
// publish-internal-names.
//
// The rule is a working habit, not lost work: in hint mode (the default) the
// call goes through and the reason reaches the model as advice; in deny mode
// it is refused. allow_repos (canonical host/owner/repo) and exclude_paths
// (a checkout path, for a local store with no remote) exempt the repos where
// direct work in the main checkout is the workflow. The reason hands over
// `git worktree add`, or, for the checkouts under a worktree_commands path,
// the command of the tool that makes their worktrees. A path or directory
// the guard cannot place, and a layout that is not a git checkout, pass.
type MainCheckout struct {
	cfg   config.Config
	event string
	// resolveRepo returns the canonical host/owner/repo of the checkout at
	// dir, "" when it has no resolvable origin remote. Injectable for tests.
	resolveRepo func(dir string) string
	// currentBranch returns the branch checked out at dir ("HEAD" when
	// detached, "" when git fails). Injectable for tests.
	currentBranch func(dir string) string
	// advise receives the advice the guard gives in hint mode (noteAdvice);
	// nil drops it, as a caller that never asked for advice wants.
	advise func(Advice)
}

// NewMainCheckout builds the guard for one event (EventBash or EventWrite)
// with the real git backends.
func NewMainCheckout(cfg config.Config, event string) *MainCheckout {
	return &MainCheckout{
		cfg:           cfg,
		event:         event,
		resolveRepo:   CanonicalRepoAt,
		currentBranch: checkoutBranch,
	}
}

func (g *MainCheckout) ID() string    { return MainCheckoutID }
func (g *MainCheckout) Event() string { return g.event }

func (g *MainCheckout) noteAdvice(note func(Advice)) { g.advise = note }

// hintTail is what hint mode adds to the reason: the call has run by the
// time the model reads it.
const hintTail = " belt let this call through (main-checkout is in hint mode); make the next change in the worktree."

// Check judges the input on the guard's event and, when it lands in a main
// checkout, denies in deny mode or gives advice in hint mode.
func (g *MainCheckout) Check(in Input) *Denial {
	var reason string
	switch g.event {
	case EventWrite:
		reason = g.writeReason(in.FilePath)
	case EventBash:
		reason = g.bashReason(in.Command, in.Cwd)
	}
	switch {
	case reason == "":
		return nil
	case g.toggle().Denies():
		return Reasonf(MainCheckoutID, "refused: %s", reason)
	}
	if g.advise != nil {
		g.advise(advicef(MainCheckoutID, "%s%s", reason, hintTail))
	}
	return nil
}

func (g *MainCheckout) toggle() config.Toggle { return g.cfg.Guards[MainCheckoutID] }

// writeReason explains why a write of file lands in a main checkout, or ""
// when it does not.
func (g *MainCheckout) writeReason(file string) string {
	if !filepath.IsAbs(file) {
		return ""
	}
	root, ok := g.mainCheckout(file)
	if !ok {
		return ""
	}
	rel, err := filepath.Rel(root, physical(file))
	if err != nil {
		return ""
	}
	h := g.handover(root)
	return fmt.Sprintf(
		"%s is in the main checkout %s, which stays on its default branch with no changes. "+
			"Make a linked worktree and edit the file there: `%s`, then edit %s.",
		file, root, h.command, h.file(rel),
	)
}

// bashReason explains why the first git command in command that changes a
// main checkout does so, or "" when none does. Each invocation is judged in
// the directory it runs in, read by the walk the other git guards share: a
// tracked cd or pushd, a `git -C` target, a subshell, an `sh -c` script.
// One whose directory only a shell can tell, and one in text the reader
// could not follow, passes.
func (g *MainCheckout) bashReason(command, cwd string) string {
	var reason string
	walkCommands(command, cwd, func(s walkStep) {
		if reason != "" || s.prose || s.unread {
			return
		}
		c, ok := segmentGitCommand(s.tokens, s.dir)
		if !ok || !filepath.IsAbs(c.dir) || !changesCheckout(c) {
			return
		}
		root, ok := g.mainCheckout(c.dir)
		if !ok || g.keepsCheckout(c, root) {
			return
		}
		h := g.handover(root)
		reason = fmt.Sprintf(
			"git %s changes the main checkout %s, which stays on its default branch with no changes. "+
				"Run it in a linked worktree: `%s`, then cd %s. Read-only git and a fast-forward pull "+
				"of the default branch still run in the main checkout.",
			c.sub,
			root,
			h.command,
			h.dir(),
		)
	})
	return reason
}

// mainCheckout returns the root of the main checkout p sits in, ok only
// when it is one the guard judges: a linked worktree and an exempt repo are
// not.
func (g *MainCheckout) mainCheckout(p string) (string, bool) {
	root, ok := mainCheckoutRoot(physical(p))
	if !ok || g.exempt(p, root) {
		return "", false
	}
	return root, true
}

// mainCheckoutRoot returns the root of the git checkout p sits in when it
// is a main checkout, read from files alone with cslindex, the lookup the
// csl index checks use to tell a linked worktree from its main checkout.
// The test is git's own: in a linked worktree `git rev-parse --git-dir`
// and `--git-common-dir` differ, and cslindex.LinkedWorktree is the file
// read of that, so every other checkout, a submodule included, counts as
// main.
func mainCheckoutRoot(p string) (string, bool) {
	root := cslindex.RepoRoot(p)
	if root == "" {
		return "", false
	}
	if _, _, linked := cslindex.LinkedWorktree(root); linked {
		return "", false
	}
	return root, true
}

// exempt reports whether the guard's exclude_paths cover p or the checkout
// root, or its allow_repos the checkout's origin remote. git runs only when
// allow_repos lists something.
func (g *MainCheckout) exempt(p, root string) bool {
	t := g.toggle()
	if t.ExcludesPath(p) || t.ExcludesPath(root) {
		return true
	}
	return len(t.AllowRepos) > 0 && g.cfg.RepoAllowed(MainCheckoutID, g.resolveRepo(root))
}

// handover is the command the reason hands over to make a linked worktree
// of a main checkout, and the path of that worktree when belt knows it. A
// worktree_commands entry's tool picks the folder itself, so worktree is ""
// for one, and the reason names the new worktree without a path.
type handover struct {
	command, worktree string
}

// file names the file at rel in the new worktree.
func (h handover) file(rel string) string {
	if h.worktree == "" {
		return rel + " in the new worktree"
	}
	return filepath.Join(h.worktree, rel)
}

// dir names the new worktree as the place to cd to.
func (h handover) dir() string {
	if h.worktree == "" {
		return "into the new worktree"
	}
	return h.worktree
}

// handover picks the worktree command for the main checkout at root: the
// worktree_commands entry whose path holds it, else `git worktree add`.
func (g *MainCheckout) handover(root string) handover {
	if w, ok := g.worktreeCommand(root); ok {
		return handover{command: w.Render(root)}
	}
	worktree := g.worktreePath(root)
	return handover{command: worktreeAdd(root, worktree), worktree: worktree}
}

// worktreeCommand returns the worktree_commands entry whose path is root or
// a directory above it, the longest such path when several are. A path is
// resolved through its symlinks first, as root already is (physical).
func (g *MainCheckout) worktreeCommand(root string) (config.WorktreeCommand, bool) {
	var match config.WorktreeCommand
	longest := -1
	for _, w := range g.toggle().WorktreeCommands {
		dir := physical(w.Dir())
		if len(dir) > longest && pathUnder(root, dir) {
			match, longest = w, len(dir)
		}
	}
	return match, longest >= 0
}

// worktreePath is where the handed-over git command puts the new worktree:
// worktree_root, the checkout's directory name, and a slug for the agent to
// fill in.
func (g *MainCheckout) worktreePath(root string) string {
	return g.toggle().WorktreeRootOrDefault() + "/" + filepath.Base(root) + "/<slug>"
}

// worktreeAdd is the command that makes a linked worktree of the main
// checkout at root. The guard never refuses it, since `worktree` is not a
// subcommand that changes the checkout.
func worktreeAdd(root, worktree string) string {
	return fmt.Sprintf("git -C %s worktree add %s -b <branch>", root, worktree)
}

// checkoutChangers are the git subcommands that change a checkout's files,
// index, branch or the commits on it whatever their arguments.
var checkoutChangers = wordSet(
	"add am bisect cherry-pick commit merge mv rebase reset restore revert rm update-index",
)

// changesCheckout reports whether one git invocation may change the
// checkout it runs in. A checkout, a switch, a pull and a branch rename
// count here, and keepsCheckout lets through the ones that leave the
// checkout as it is, since telling them apart needs the checkout.
func changesCheckout(c gitInvocation) bool {
	switch c.sub {
	case "checkout", "switch", "pull":
		return true
	case "branch":
		move, names := branchRename(c.args)
		return move && len(names) > 0
	case "stash":
		return len(c.args) == 0 || (c.args[0] != "list" && c.args[0] != "show")
	case "clean":
		return cleanDiscards(c.args)
	case "apply":
		return !applyOnlyReports(c.args)
	case "sparse-checkout":
		return len(c.args) > 0 && c.args[0] != "list" && c.args[0] != "check-rules"
	case "submodule":
		return !submoduleReports(c.args)
	}
	return checkoutChangers[c.sub]
}

// keepsCheckout reports whether a call changesCheckout counts still leaves
// the main checkout at root as the guard keeps it: a switch back to its
// default branch, the pull that syncs it, and a rename of a branch it does
// not have checked out.
func (g *MainCheckout) keepsCheckout(c gitInvocation, root string) bool {
	switch c.sub {
	case "checkout", "switch":
		return returnsToDefault(c.args, root)
	case "pull":
		return g.pullSyncs(c)
	case "branch":
		return g.renamesOther(c)
	}
	return false
}

// returnsToDefault reports whether a checkout or switch at most puts the
// default branch of the main checkout at root back (isDefaultBranch): no
// option but -q, and no operand but that branch name. That is the way back
// to the state the guard keeps.
func returnsToDefault(args []string, root string) bool {
	var names []string
	for _, a := range args {
		switch {
		case a == "-q" || a == "--quiet":
		case strings.HasPrefix(a, "-"):
			return false
		default:
			names = append(names, a)
		}
	}
	return len(names) == 0 || (len(names) == 1 && isDefaultBranch(root, names[0]))
}

// isDefaultBranch reports whether name is the default branch of the main
// checkout at root. It is main or master; when origin/HEAD names one of
// the two, only that one.
func isDefaultBranch(root, name string) bool {
	if d := originDefault(root); defaultBranches[d] {
		return name == d
	}
	return defaultBranches[name]
}

// originDefault returns the branch origin/HEAD names in the main checkout
// at root, read from the file git keeps that symbolic ref in (packed-refs
// holds no symbolic refs), or "" when there is none: a clone that never set
// it, a submodule, whose refs sit elsewhere, or a reftable repo.
func originDefault(root string) string {
	name, _ := pathLine(
		filepath.Join(root, ".git", "refs", "remotes", "origin", "HEAD"),
		"ref: refs/remotes/origin/",
	)
	return name
}

// branchRename reads git branch's arguments for a rename: move is set when
// -m, -M or --move is given, alone or in a bundle of short options, and
// names are the branch operands, the old name first when there are two.
func branchRename(args []string) (move bool, names []string) {
	for i, a := range args {
		switch {
		case a == "--":
			return move, append(names, args[i+1:]...)
		case a == "--move":
			move = true
		case strings.HasPrefix(a, "--"):
		case len(a) > 1 && a[0] == '-':
			move = move || shortBranchMove(a[1:])
		default:
			names = append(names, a)
		}
	}
	return move, names
}

// shortBranchMove reports whether a bundle of short git branch options
// holds -m or -M before a letter that takes the rest of the word as its
// value (-u, -t).
func shortBranchMove(letters string) bool {
	for _, l := range letters {
		switch l {
		case 'm', 'M':
			return true
		case 'u', 't':
			return false
		}
	}
	return false
}

// renamesOther reports whether a git branch rename leaves the checked-out
// branch alone: it names the old branch, and that is not the one checked
// out. With one name git renames the checked-out branch. A branch git
// cannot read passes, as the guard fails open.
func (g *MainCheckout) renamesOther(c gitInvocation) bool {
	_, names := branchRename(c.args)
	if len(names) < 2 {
		return false
	}
	branch := g.currentBranch(c.dir)
	return branch == "" || names[0] != branch
}

// submoduleReports reports whether git submodule only reports: bare, which
// runs status, status or summary, after the options git takes before the
// subcommand. foreach runs any command, so it counts as a change.
func submoduleReports(args []string) bool {
	for _, a := range args {
		switch a {
		case "-q", "--quiet", "--cached":
			continue
		case "status", "summary":
			return true
		}
		return false
	}
	return true
}

// applyOnlyReports reports whether `git apply` only checks or describes the
// patch: --check never applies it, and --stat, --numstat and --summary turn
// applying off unless --apply turns it back on.
func applyOnlyReports(args []string) bool {
	if hasFlag(args, "--check") {
		return true
	}
	return hasFlag(args, "--stat", "--numstat", "--summary") && !hasFlag(args, "--apply")
}

// pullMergeFlags are the long pull options that make it more than a
// fast-forward: a merge commit, a squash, an edited message or another
// merge strategy. pullShortMerge holds their short forms.
var pullMergeFlags = wordSet(
	"--no-ff --squash --no-commit --edit --strategy --strategy-option --allow-unrelated-histories",
)

// pullValueFlags are the long pull options whose value may be the next
// word. --jobs, --rebase, --log, --gpg-sign and --recurse-submodules take
// theirs only after an =.
var pullValueFlags = wordSet(
	"--cleanup --strategy --strategy-option --upload-pack --depth --deepen " +
		"--shallow-since --shallow-exclude --refmap --server-option " +
		"--negotiation-tip --negotiation-restrict --negotiation-include",
)

// The short pull options by how git reads them: pullShortMerge make a
// merge of another kind (-e, -s, -X), pullShortValue take the rest of the
// word or else the next word as their value, and pullShortAttached take
// only the rest of the word.
const (
	pullShortMerge    = "esX"
	pullShortValue    = "sXo"
	pullShortAttached = "rSj"
)

// pullSyncs reports whether a pull in a main checkout is the sync that keeps
// it current: the checkout is on a default branch, no option makes a merge
// of another kind, and the operands name at most the remote and that same
// branch. A checkout that keeps to the rule has no commits of its own, so
// such a pull, --rebase or not, fast-forwards. A branch git cannot read
// passes, as the guard fails open.
func (g *MainCheckout) pullSyncs(c gitInvocation) bool {
	merges, operands := readPull(c.args)
	if merges {
		return false
	}
	branch := g.currentBranch(c.dir)
	switch {
	case branch == "":
		return true
	case !defaultBranches[branch]:
		return false
	}
	return len(operands) <= 1 || (len(operands) == 2 && operands[1] == branch)
}

// readPull reads git pull's arguments the way its option parser does. It
// reports whether a pullMergeFlags option or a pullShortMerge letter is
// set, and returns the operands, the remote and its refspecs, without the
// values of the options that take one.
func readPull(args []string) (merges bool, operands []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			return merges, append(operands, args[i+1:]...)
		case strings.HasPrefix(a, "--"):
			name, _, attached := strings.Cut(a, "=")
			merges = merges || pullMergeFlags[name]
			if !attached && pullValueFlags[name] {
				i++
			}
		case len(a) > 1 && a[0] == '-':
			merge, takesNext := readPullShort(a[1:])
			merges = merges || merge
			if takesNext {
				i++
			}
		default:
			operands = append(operands, a)
		}
	}
	return merges, operands
}

// readPullShort reads one bundle of short pull options, the letters after
// the dash. It reports whether one makes a merge of another kind, and
// whether the bundle ends in a letter that takes the next word as its
// value. A letter that takes a value ends the bundle, since the rest of the
// word is that value (-Xtheirs, -sours).
func readPullShort(letters string) (merge, takesNext bool) {
	for i, l := range letters {
		merge = merge || strings.ContainsRune(pullShortMerge, l)
		switch {
		case strings.ContainsRune(pullShortValue, l):
			return merge, i == len(letters)-1
		case strings.ContainsRune(pullShortAttached, l):
			return merge, false
		}
	}
	return merge, false
}

// checkoutBranch returns the branch checked out at dir ("HEAD" when
// detached), or "" when git fails or takes longer than
// branchLookupTimeout.
func checkoutBranch(dir string) string {
	ctx, cancel := context.WithTimeout(context.Background(), branchLookupTimeout)
	defer cancel()
	return gitloc.Output(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// physical resolves the symlinks on p's way, so a file reached through a
// link into a checkout (a dotfile linked from the home directory) is judged
// in the checkout it lands in. The part of p that does not exist yet, a new
// file or directory, is kept as written.
func physical(p string) string {
	rest := ""
	for d := p; ; {
		if real, err := filepath.EvalSymlinks(d); err == nil {
			return filepath.Join(real, rest)
		}
		parent := filepath.Dir(d)
		if parent == d {
			return filepath.Clean(p)
		}
		rest = filepath.Join(filepath.Base(d), rest)
		d = parent
	}
}

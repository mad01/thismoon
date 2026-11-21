package guard

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// GitDiscardID identifies the guard against discarding uncommitted work.
const GitDiscardID = "git-discard"

// gitGoAhead opens the tail of every deny: the hook never sees the user's
// message, so a go-ahead cannot pass it.
const gitGoAhead = "belt cannot see the user's message, so a go-ahead does not pass this check"

// gitDiscardAsk is the tail of a discard's deny, with the two ways forward.
const gitDiscardAsk = gitGoAhead +
	": save the work first, or let the user run it with `! <command>`."

// stashDropReason is what the model reads when a command may delete stash
// entries (stashDrops).
const stashDropReason = "this deletes stash entries. Every worktree of a repo shares one " +
	"stash list, so an entry may be another session's set-aside work, and a dropped one " +
	"comes back only through `git fsck` until the next gc. Run `git stash list` to see the " +
	"entries, and set your own work aside with a WIP commit, not a stash. " + gitGoAhead +
	": let the user run it with `! <command>`."

// gitDiscardRule says when a discard the proof reads passes.
const gitDiscardRule = "It passes only when everything it drops is already in HEAD, " +
	"its upstream or origin's default branch."

// gitDiscardReason is what the model reads on a deny. why names the first
// loss or the reason the losses could not be listed; a command the proof
// does not read passes on a clean tree alone.
func gitDiscardReason(why error) string {
	if errors.Is(why, errNoProof) {
		return "this discards uncommitted work and passes only on a clean tree. " + gitDiscardAsk
	}
	return "this discards uncommitted work: " + why.Error() + ". " + gitDiscardRule + " " +
		gitDiscardAsk
}

// GitDiscard blocks git commands that throw away uncommitted work: a stash
// push, `reset --hard`, a checkout or restore over the worktree, a forced
// clean. A session audit found agents running these over the user's own
// edits without being asked, and the edits are not in any reflog. A clean
// working tree has nothing to lose, so the guard allows there. So does a
// checkout, restore, clean or hard reset proven to lose nothing (prover):
// every file version it drops is committed in HEAD, its upstream or origin's
// default branch. It also blocks `stash drop` and `stash clear`, which
// delete stash entries any worktree of the repo may have set aside, whatever
// the tree holds. allow_repos exempts repos by canonical identity, and
// `mode: soft` warns instead of denying.
type GitDiscard struct {
	cfg config.Config
	// resolveRepo returns the canonical host/owner/repo of the repo at dir,
	// or "" when it cannot be determined. Injectable for tests.
	resolveRepo func(dir string) string
	// status returns `git status --porcelain` output for the repo at dir
	// (extra args appended) and whether git succeeded. Injectable for tests.
	status func(dir string, extra ...string) (string, bool)
	// isTree reports whether rev names a tree-ish in the repo at dir, which
	// sorts a checkout's operands (sortCheckoutOperands); err is set when
	// ctx ran out first. Injectable for tests.
	isTree func(ctx context.Context, dir, rev string) (bool, error)
	// lookupBudget bounds every isTree call of one Check together.
	lookupBudget time.Duration
	// prove returns nil when a discard provably loses nothing, else the
	// first loss. Injectable for tests.
	prove func(c gitInvocation) error
	// emit sends the soft-mode warning to the events service. Injectable
	// for tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewGitDiscard builds the guard with the real git and events backends.
func NewGitDiscard(cfg config.Config) *GitDiscard {
	return &GitDiscard{
		cfg:          cfg,
		resolveRepo:  CanonicalRepoAt,
		status:       gitStatusPorcelain,
		isTree:       gitIsTree,
		lookupBudget: treeLookupBudget,
		prove:        newProver().prove,
		emit:         notify.EmitEventSync,
	}
}

func (g *GitDiscard) ID() string    { return GitDiscardID }
func (g *GitDiscard) Event() string { return EventBash }

// Check judges every discarding git invocation in the command in the
// directory it runs in (the same cd/pushd and `git -C` tracking the other
// git guards use). An invocation whose directory only a shell can name is
// denied: there is no tree to check for changes. So is one in code the
// reader could not follow (walkStep.unread), where no directory is certain.
// A checkout is judged in the form git reads it as (sortCheckoutOperands),
// with one lookupBudget for all its ref lookups. The walk is read whole
// first, since a step after a discard may run beside it (proofBlocker). An
// invocation that may delete stash entries (stashDrops) is judged on its own
// (checkStashDrop), outside text no shell runs (walkStep.prose).
func (g *GitDiscard) Check(in Input) *Denial {
	var steps []walkStep
	walkCommands(in.Command, in.Cwd, func(s walkStep) { steps = append(steps, s) })
	beside := runsBeside(in.Command)
	lookups, cancel := context.WithTimeout(context.Background(), g.lookupBudget)
	defer cancel()
	for i, s := range steps {
		dir := s.dir
		if s.unread {
			dir = ""
		}
		c, ok := segmentGitCommand(s.tokens, dir)
		if !ok {
			continue
		}
		fed := fedOperands(s.tokens)
		if !s.prose && stashDrops(c, fed) {
			if d := g.checkStashDrop(c); d != nil {
				return d
			}
			continue
		}
		c, unsorted := g.sortCheckoutOperands(lookups, c)
		if !discards(c, fed) {
			continue
		}
		unsure := cmp.Or(proofBlocker(steps, i, beside), unsorted)
		if d := g.checkDiscard(c, unsure); d != nil {
			return d
		}
	}
	return nil
}

// checkDiscard decides one discarding invocation: exempt repo, clean tree
// or a proof that nothing is lost allows, soft mode warns, anything else
// denies with the first loss. unsure, when set, says why the command around
// the discard voids the proof.
func (g *GitDiscard) checkDiscard(c gitInvocation, unsure error) *Denial {
	why := errors.New("belt cannot tell which repository it runs in")
	if c.dir != "" {
		if g.cfg.RepoAllowed(GitDiscardID, g.resolveRepo(c.dir)) {
			return nil
		}
		if out, ok := g.status(c.dir, statusExtra(c)...); ok && out == "" {
			return nil
		}
		if why = g.proveUnless(c, unsure); why == nil {
			return nil
		}
	}
	if g.warnSoft(c, "would discard uncommitted work") {
		return nil
	}
	return Reasonf(GitDiscardID, "%s", gitDiscardReason(why))
}

// checkStashDrop decides one invocation that may delete stash entries
// (stashDrops). The loss is the entry, not the tree, so a clean tree or a
// proof allows nothing: only an exempt repo allows, and soft mode warns.
func (g *GitDiscard) checkStashDrop(c gitInvocation) *Denial {
	if c.dir != "" && g.cfg.RepoAllowed(GitDiscardID, g.resolveRepo(c.dir)) {
		return nil
	}
	if g.warnSoft(c, "would delete stash entries") {
		return nil
	}
	return Reasonf(GitDiscardID, "%s", stashDropReason)
}

// warnSoft sends the soft-mode warning that c loses what loss says, and
// reports whether soft mode is on, so the caller allows instead of denying.
func (g *GitDiscard) warnSoft(c gitInvocation, loss string) bool {
	if !g.cfg.Guards[GitDiscardID].Soft() {
		return false
	}
	g.emit(
		"belt",
		"warn",
		"git-discard (soft)",
		"belt["+GitDiscardID+"]: git "+c.sub+" "+strings.Join(c.args, " ")+" "+loss+
			". Allowed (soft mode).",
		map[string]string{"guard": GitDiscardID, "dir": c.dir},
	)
	return true
}

// proveUnless runs the nothing-lost proof unless unsure says the command
// around the discard voids it. A subcommand the proof never reads (stash,
// switch) keeps its own reason.
func (g *GitDiscard) proveUnless(c gitInvocation, unsure error) error {
	if unsure == nil {
		return g.prove(c)
	}
	if _, err := readDiscardSpec(c); errors.Is(err, errNoProof) {
		return err
	}
	return unsure
}

// proofBlocker says why the nothing-lost proof cannot vouch for the discard
// at steps[i], or nil. The proof reads the tree before the command runs, so
// no step before the discard may change the tree, a ref or git's
// environment (mayChangeTree), and no step after it may when the command
// can run two steps at once (beside). The proof runs git as belt does, so
// the discard takes no word in front of git and writes through no
// redirection of its own.
func proofBlocker(steps []walkStep, i int, beside bool) error {
	s := steps[i]
	if pre, _ := beforeGit(s.tokens); len(pre) > 0 || s.configEnv {
		return notListed("a word before git or a git config variable may change how git reads it")
	}
	if writesThrough(s) {
		return notListed("a redirection on it writes a file")
	}
	for j, o := range steps {
		if j != i && (j < i || beside) && mayChangeTree(o) {
			return fmt.Errorf(
				"`%s` runs in the same command and may change what it drops; "+
					"run the discard in a command of its own",
				stepText(o.tokens),
			)
		}
	}
	return nil
}

// readOnlyGit are the git subcommands that change no file, index content or
// ref.
var readOnlyGit = wordSet("status log diff show rev-parse ls-files")

// mayChangeTree reports whether a step may change a worktree file, the
// index, a ref or the environment git runs with. Text no shell runs
// (walkStep.prose) changes nothing. A plainly run readOnlyGit command, or a
// readOnlyCommand, with no redirection and no word in front of git, an
// override or --output, changes nothing either. Anything else may: a bare
// assignment (an exported variable keeps its new value), a function
// definition, code the reader could not follow.
func mayChangeTree(s walkStep) bool {
	switch {
	case s.prose:
		return false
	case s.unread || s.defines || s.configEnv || writesThrough(s):
		return true
	}
	if c, ok := segmentGitCommand(s.tokens, s.dir); ok {
		pre, _ := beforeGit(s.tokens)
		return len(pre) > 0 || !readOnlyGit[c.sub] || c.configured ||
			slices.ContainsFunc(c.args, outputOption)
	}
	words := stripWrappers(s.tokens)
	return len(words) == 0 || !readOnlyCommand(words)
}

// writesThrough reports whether a step has a redirection that may write a
// file. /dev/null and a lone digit, which is how the walk records a
// descriptor copy (`2>&1`), do not count; any other target does, an input
// or a heredoc delimiter included, since the walk does not keep the
// direction.
func writesThrough(s walkStep) bool {
	targets := append(slices.Clone(s.targets), redirectOperands(s.tokens)...)
	return slices.ContainsFunc(targets, func(t string) bool {
		fdCopy := len(t) == 1 && '0' <= t[0] && t[0] <= '9'
		return t != "/dev/null" && !fdCopy
	})
}

// runsBeside reports whether a command may run two of its steps at once: it
// holds a pipe or a lone & that is no part of &&, ||, or a redirection
// (`2>&1`, `&>`, `>|`). Quotes are not read, so a quoted | or & counts too.
func runsBeside(command string) bool {
	at := func(i int) byte {
		if i < 0 || i >= len(command) {
			return 0
		}
		return command[i]
	}
	for i := 0; i < len(command); i++ {
		ch, prev, next := command[i], at(i-1), at(i+1)
		switch {
		case ch != '|' && ch != '&':
		case next == ch:
			i++
		case ch == '|' && prev != '>':
			return true
		case ch == '&' && prev != '>' && prev != '<' && next != '>':
			return true
		}
	}
	return false
}

// statusExtra widens the clean-tree check for `git clean -x`/`-X`, which
// also deletes ignored files that plain `git status --porcelain` never
// lists, and for `-e`, whose `!` pattern brings an ignored file in. A clean
// whose words belt cannot read (readGitWords) may hold any of them.
func statusExtra(c gitInvocation) []string {
	if c.sub != "clean" {
		return nil
	}
	w, err := readGitWords(c.args, cleanOptions)
	if err != nil || w.sets("x") || w.sets("X") || w.sets("exclude") {
		return []string{"--ignored"}
	}
	return nil
}

// discards reports whether one git invocation throws away uncommitted work.
// fed marks one a command in front of git may hand more words (xargs,
// fedOperands), which for a reset or clean may be any option.
func discards(c gitInvocation, fed bool) bool {
	switch c.sub {
	case "stash":
		return stashDiscards(c.args)
	case "reset":
		return fed || resetDiscards(c.args)
	case "checkout":
		return checkoutDiscards(c.args, fed)
	case "switch":
		return setsOption(c.args, 'f', switchBundleEnds, "--force", "--discard-changes")
	case "restore":
		return restoreDiscards(c.args)
	case "clean":
		return fed || cleanDiscards(c.args)
	}
	return false
}

// resetDiscards: a reset drops worktree changes in --hard mode, written in
// any form git reads as it (`--ha`, readGitWords). Words belt cannot read
// count too.
func resetDiscards(args []string) bool {
	w, err := readGitWords(args, resetOptions)
	return err != nil || w.sets("hard")
}

// stashKeeps are the stash subcommands that leave the working tree alone
// or only add to it. drop and clear delete stash entries, not worktree
// changes, which stashDrops judges on its own.
var stashKeeps = []string{
	"list",
	"show",
	"apply",
	"pop",
	"branch",
	"drop",
	"clear",
	"create",
	"store",
	"export",
	"import",
}

// stashDiscards: a bare `git stash`, a flag-led one (`git stash -u`), and
// the push/save subcommands all move worktree changes off the tree.
func stashDiscards(args []string) bool {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return true
	}
	return !slices.Contains(stashKeeps, args[0])
}

// stashDrops reports whether a git invocation may delete stash entries:
// `stash drop` or `stash clear`. git takes the stash subcommand from the
// first word alone, matched exactly; an option there makes the stash a push,
// where a later `drop` is a pathspec or an error (git 2.55 fails `git stash
// -q drop`). Every drop or clear git runs deletes, so the words after the
// subcommand change nothing. A first word the shell may still rewrite
// (asWritten) may be either, and so may a missing one a command in front of
// git may hand over (fed, `xargs git stash`). pop and branch drop an entry
// only once its work is back in the tree, so they pass.
func stashDrops(c gitInvocation, fed bool) bool {
	switch {
	case c.sub != "stash":
		return false
	case len(c.args) == 0:
		return fed
	}
	switch first := c.args[0]; {
	case first == "drop" || first == "clear":
		return true
	case strings.HasPrefix(first, "-"):
		return false // no shell rewrite drops the leading -
	default:
		return !asWritten(first)
	}
}

// asWritten reports whether the shell hands a word to git as the walk shows
// it: letters, digits and plainValuePunct alone, with nothing to expand,
// split or glob.
func asWritten(word string) bool {
	for _, r := range word {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) &&
			!strings.ContainsRune(plainValuePunct, r) {
			return false
		}
	}
	return true
}

// checkoutDiscards: a path checkout (`checkout -- <path>`, `checkout <ref>
// -- <path>`, the forms sortCheckoutOperands gives one without `--` or in
// patch mode, and `checkout .`) overwrites the worktree copy, and a forced
// branch switch (-f in any bundle, setsOption) drops local changes on the
// way. Paths read from a file (--pathspec-from-file) or handed over by a
// command in front of git (fed) may be any, so either counts as a path
// checkout too.
func checkoutDiscards(args []string, fed bool) bool {
	if i := slices.Index(args, "--"); i >= 0 && i+1 < len(args) {
		return true
	}
	return fed || slices.Contains(args, ".") ||
		setsOption(args, 'f', checkoutBundleEnds, "--force") ||
		slices.ContainsFunc(args, pathspecFromFile)
}

// restoreDiscards: restore touches the worktree unless only --staged is
// given; `--staged --worktree` touches both.
func restoreDiscards(args []string) bool {
	staged := hasFlag(args, "--staged", "-S")
	worktree := hasFlag(args, "--worktree", "-W")
	return !staged || worktree
}

// cleanDiscards: clean deletes with -f, or with -i once the user picks what
// goes (git refuses a plain clean under the default clean.requireForce),
// unless the last -n or --no-dry-run left it a dry run. Options count in any
// form git reads (`--fo`, `-fd`, readGitWords), and words belt cannot read
// count as a delete.
func cleanDiscards(args []string) bool {
	w, err := readGitWords(args, cleanOptions)
	if err != nil {
		return true
	}
	dry := false
	for _, o := range w.opts {
		if o.name == "dry-run" {
			dry = !o.unset
		}
	}
	return !dry && (w.sets("force") || w.sets("interactive"))
}

// gitStatusPorcelain runs `git status --porcelain` in dir. ok is false when
// dir is empty or git fails (not a repo), which the guard treats as dirty.
// A submodule's `ignore` setting is overridden: under submodule.recurse a
// reset rewrites a dirty submodule that plain status would hide.
func gitStatusPorcelain(dir string, extra ...string) (string, bool) {
	if dir == "" {
		return "", false
	}
	args := append([]string{"status", "--porcelain", "--ignore-submodules=none"}, extra...)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

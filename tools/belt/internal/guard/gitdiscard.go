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

	"github.com/mad01/thismoon/kit/gitloc"
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
// push, `reset --hard` or `--merge`, a checkout or restore over the
// worktree, a clean that deletes (forced, or with clean.requireForce off), a
// forced worktree remove, rm or mv, an apply, a `read-tree -u`, a forced
// checkout-index. A session audit found agents running these over the
// user's own edits without being asked, and the edits are not in any
// reflog. A clean working tree has nothing to lose, so the guard allows
// there. So does a discard other than a stash push or switch proven to lose
// nothing (prover): every file version it drops is committed in HEAD, its
// upstream or origin's default branch. A git call whose subcommand belt
// cannot see (hiddenGit) passes on a clean tree alone. It also blocks
// `stash drop`, `stash clear` and a reflog or ref delete that reaches
// refs/stash, which delete stash entries any worktree of the repo may have
// set aside, whatever the tree holds, and any worktree remove or prune that
// deletes the HEAD and HEAD reflog that alone hold some commits
// (keepsWorktreeCommits, judgePrune). allow_repos exempts repos by canonical
// identity, and `mode: soft` warns instead of denying.
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
	// worktrees lists the worktrees of the repository at dir, which places
	// a worktree remove (removedWorktrees). Injectable for tests.
	worktrees func(dir string) ([]listedWorktree, error)
	// requireForce reports whether git in dir refuses a clean without -f or
	// -i (cleanRequiresForce). Injectable for tests.
	requireForce func(ctx context.Context, dir string) bool
	// stranded counts the commits a remove of the linked worktree wt, run
	// in dir, strands (keepsWorktreeCommits). Injectable for tests.
	stranded func(ctx context.Context, dir string, wt listedWorktree) (lostCommits, error)
	// pruned counts the commits a `worktree prune` with the expire words,
	// run in dir, strands (judgePrune). Injectable for tests.
	pruned func(ctx context.Context, dir string, expire []string) (lostCommits, error)
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
		worktrees:    gitWorktrees,
		requireForce: cleanRequiresForce,
		stranded:     removeStranded,
		pruned:       pruneStranded,
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
// (checkStashDrop), outside text no shell runs (walkStep.prose), and so is
// one whose subcommand belt cannot see (hiddenGit, checkHidden). A clean
// without -f or -i counts when clean.requireForce may be off
// (unforcedCleanDeletes). A denial for a step of a message read as code for
// bash 3.2 alone says how to pass the message as data (bash32Hinted).
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
		if c, drops, hidden := hiddenGit(s, dir); hidden {
			if d := g.checkHidden(c, drops); d != nil {
				return bash32Hinted(d, s.bash32)
			}
			continue
		}
		c, ok := segmentGitCommand(s.tokens, dir)
		if !ok {
			continue
		}
		fed := fedOperands(s.tokens)
		if !s.prose && stashDrops(c, fed) {
			if d := g.checkStashDrop(c); d != nil {
				return bash32Hinted(d, s.bash32)
			}
			continue
		}
		c, unsorted := g.sortCheckoutOperands(lookups, c)
		if !discards(c, fed) && !g.unforcedCleanDeletes(lookups, c, steps, i, beside) {
			continue
		}
		unsure := cmp.Or(proofBlocker(steps, i, beside), unsorted)
		if d := g.checkDiscard(c, fed, unsure); d != nil {
			return bash32Hinted(d, s.bash32)
		}
	}
	return nil
}

// checkDiscard decides one discarding invocation: exempt repo, clean tree
// or a proof that nothing is lost allows, soft mode warns, anything else
// denies with the first loss. fed says a command in front of git may hand
// it words (fedOperands). unsure, when set, says why the command around the
// discard voids the proof.
func (g *GitDiscard) checkDiscard(c gitInvocation, fed bool, unsure error) *Denial {
	why := errors.New("belt cannot tell which repository it runs in")
	if c.dir != "" {
		if g.cfg.RepoAllowed(GitDiscardID, g.resolveRepo(c.dir)) {
			return nil
		}
		if why = g.judge(c, fed, unsure); why == nil {
			return nil
		}
	}
	if g.warnSoft(c, "would discard uncommitted work") {
		return nil
	}
	return Reasonf(GitDiscardID, "%s", gitDiscardReason(why))
}

// judge returns nil when the discard c loses nothing in the tree it
// discards in, else the first loss. A worktree remove is judged in each
// worktree it may delete (removedWorktrees), not where it runs: its HEAD and
// HEAD reflog must strand no commit (keepsWorktreeCommits), forced or not,
// and a forced remove must lose no file (judgeTree). The commit counts of
// one remove share one proofBudget. A prune is judged on its own
// (judgePrune).
func (g *GitDiscard) judge(c gitInvocation, fed bool, unsure error) error {
	if c.sub != "worktree" {
		return g.judgeTree(c, unsure)
	}
	if len(c.args) > 0 && c.args[0] == "prune" {
		return g.judgePrune(c, fed)
	}
	rm, err := g.removedWorktrees(c, fed)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), proofBudget)
	defer cancel()
	for _, wt := range rm.targets {
		in := c
		in.dir = wt.path
		err := g.keepsWorktreeCommits(ctx, c.dir, wt)
		if err == nil && rm.forced {
			err = g.judgeTree(in, unsure)
		}
		if err != nil {
			return fmt.Errorf("in the worktree at %s, %w", wt.path, err)
		}
	}
	return nil
}

// judgeTree passes a discard when the tree at c.dir is clean, unless the
// command may also write past it (writesOutside), or when the proof passes.
func (g *GitDiscard) judgeTree(c gitInvocation, unsure error) error {
	if out, ok := g.status(c.dir, statusExtra(c)...); ok && out == "" && !writesOutside(c) {
		return nil
	}
	return g.proveUnless(c, unsure)
}

// outsideWriters are the options that make a discard write past the tree
// git status sees, per subcommand: a --prefix export, a patch path outside
// the worktree, a fake ancestor or index file at a path the user names.
var outsideWriters = map[string]struct {
	table []gitOpt
	names []string
}{
	"checkout-index": {checkoutIndexOptions, []string{"prefix"}},
	"apply":          {applyOptions, []string{"unsafe-paths", "build-fake-ancestor"}},
	"read-tree":      {readTreeOptions, []string{"index-output"}},
}

// writesOutside reports whether a discard sets an outsideWriters option, so
// a clean tree alone does not pass it. Words belt cannot read count as a
// discard already and keep the clean-tree rule.
func writesOutside(c gitInvocation) bool {
	o, ok := outsideWriters[c.sub]
	if !ok {
		return false
	}
	w, err := readGitWords(c.args, o.table)
	return err == nil && slices.ContainsFunc(o.names, w.on)
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

// checkHidden decides a git call whose subcommand belt cannot see
// (hiddenGit). It may discard, so only a clean tree passes, with no proof
// to read; and when it may delete stash entries too (drops), only an exempt
// repo passes (checkStashDrop).
func (g *GitDiscard) checkHidden(c gitInvocation, drops bool) *Denial {
	if d := g.checkDiscard(c, false, errNoProof); d != nil || !drops {
		return d
	}
	return g.checkStashDrop(c)
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
// whose words belt cannot read (readGitWords) may hold any of them. A
// forced mv may overwrite an ignored file at its destination.
func statusExtra(c gitInvocation) []string {
	if c.sub == "mv" {
		return []string{"--ignored"}
	}
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
		return fed || switchDiscards(c.args)
	case "restore":
		return restoreDiscards(c.args)
	case "clean":
		return fed || cleanDiscards(c.args)
	case "worktree":
		return worktreeDeletes(c.args, fed)
	case "apply":
		return fed || applyWrites(c.args)
	case "read-tree":
		return fed || readTreeUpdates(c.args)
	case "checkout-index":
		return fed || checkoutIndexForces(c.args)
	case "rm":
		return fed || rmDeletes(c.args)
	case "mv":
		return fed || mvOverwrites(c.args)
	}
	return false
}

// resetDiscards: a reset drops worktree changes in --hard mode, and staged
// ones in --merge mode, written in any form git reads as it (`--ha`,
// readGitWords). Words belt cannot read count too.
func resetDiscards(args []string) bool {
	w, err := readGitWords(args, resetOptions)
	return err != nil || w.sets("hard") || w.sets("merge")
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

// stashDrops reports whether a git invocation may delete stash entries: a
// stash call that drops them (stashCommandDrops), or a reflog or ref delete
// that may reach refs/stash, which holds them (reflogDrops, updateRefDrops).
// fed says a command in front of git may hand it words.
func stashDrops(c gitInvocation, fed bool) bool {
	switch c.sub {
	case "stash":
		return stashCommandDrops(c.args, fed)
	case "reflog":
		return reflogDrops(c.args, fed)
	case "update-ref":
		return updateRefDrops(c.args, fed)
	}
	return false
}

// stashCommandDrops reports whether the words of a stash call drop entries:
// `stash drop` or `stash clear`. git takes the stash subcommand from the
// first word alone, matched exactly; an option there makes the stash a push,
// where a later `drop` is a pathspec or an error (git 2.55 fails `git stash
// -q drop`). Every drop or clear git runs deletes, so the words after the
// subcommand change nothing. A first word the shell may still rewrite
// (asWritten) may be either, and so may a missing one a command in front of
// git may hand over (fed, `xargs git stash`). pop and branch drop an entry
// only once its work is back in the tree, so they pass.
func stashCommandDrops(args []string, fed bool) bool {
	if len(args) == 0 {
		return fed
	}
	switch first := args[0]; {
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
// branch switch drops local changes on the way. Options count in any form
// git reads (`--forc`, `-qf`, readGitWords), and words belt cannot read
// count as a discard. Paths read from a file (--pathspec-from-file) or
// handed over by a command in front of git (fed) may be any, so either
// counts as a path checkout too.
func checkoutDiscards(args []string, fed bool) bool {
	w, err := readGitWords(args, checkoutOptions)
	if err != nil {
		return true
	}
	return fed || len(w.paths) > 0 || slices.Contains(w.operands, ".") ||
		w.on("force") || w.on("pathspec-from-file")
}

// switchDiscards: a switch drops local changes with -f or
// --discard-changes, read in any form git reads (readGitWords). Words belt
// cannot read count as a discard.
func switchDiscards(args []string) bool {
	w, err := readGitWords(args, switchOptions)
	return err != nil || w.on("force") || w.on("discard-changes")
}

// restoreDiscards: restore touches the worktree unless only --staged is
// given; `--staged --worktree` touches both.
func restoreDiscards(args []string) bool {
	staged := hasFlag(args, "--staged", "-S")
	worktree := hasFlag(args, "--worktree", "-W")
	return !staged || worktree
}

// cleanDiscards: clean deletes with -f, or with -i once the user picks what
// goes (git refuses a plain clean under the default clean.requireForce,
// which unforcedCleanDeletes reads), unless the last -n or --no-dry-run
// left it a dry run. Options count in any
// form git reads (`--fo`, `-fd`, readGitWords), and words belt cannot read
// count as a delete.
func cleanDiscards(args []string) bool {
	w, err := readGitWords(args, cleanOptions)
	if err != nil {
		return true
	}
	return !w.on("dry-run") && (w.sets("force") || w.sets("interactive"))
}

// unforcedCleanDeletes reports whether a clean that sets neither -f nor -i
// deletes all the same: git runs one when clean.requireForce is false. The
// command may set that itself (setsGitConfig), and any config scope on disk
// may hold it, read in the repository the clean runs in (requireForce). A
// dry run deletes nothing, and words belt cannot read already count as a
// delete (cleanDiscards).
func (g *GitDiscard) unforcedCleanDeletes(
	ctx context.Context,
	c gitInvocation,
	steps []walkStep,
	i int,
	beside bool,
) bool {
	if c.sub != "clean" {
		return false
	}
	if w, err := readGitWords(c.args, cleanOptions); err != nil || w.on("dry-run") {
		return false
	}
	return setsGitConfig(steps, i, c, beside) || !g.requireForce(ctx, c.dir)
}

// setsGitConfig reports whether the command may hand the git call c at
// steps[i] config the repository does not hold: its own -c or --config-env,
// a config variable set in front of it or exported before it, or a step
// before it, or beside it, that writes git config or sets such a variable
// (retargetsPush, setsConfigEnv).
func setsGitConfig(steps []walkStep, i int, c gitInvocation, beside bool) bool {
	s := steps[i]
	if pre, _ := beforeGit(s.tokens); c.configured || s.configEnv || overridesConfig(pre) {
		return true
	}
	for j, o := range steps {
		if j != i && (j < i || beside) && (retargetsPush(o) || setsConfigEnv(o)) {
			return true
		}
	}
	return false
}

// cleanRequiresForce reports whether git in dir refuses a clean without -f
// or -i: clean.requireForce as every config scope git reads sets it, true
// when none does. fsmonitor is off for the read, so no repo config runs a
// program. A value git cannot read as a boolean, a dir belt cannot name and
// a read past ctx all read as false, so the clean counts as a delete.
func cleanRequiresForce(ctx context.Context, dir string) bool {
	out := gitloc.Output(
		ctx, dir, "-c", "core.fsmonitor=false", "config", "--type=bool", "--default=true",
		"--get", "clean.requireForce",
	)
	return out == "true"
}

// worktreeDeletes: `worktree remove` deletes a linked worktree with its
// HEAD and HEAD reflog, so every remove counts: commits only they hold go
// with it, forced or not. With --force (twice for a locked worktree) the
// changes and untracked files go too; without it git refuses a tree that
// has them. A prune deletes the HEAD and HEAD reflog of each worktree whose
// folder is gone, so one counts unless it is a dry run (-n), and so does one
// a command in front of git may hand more words (fed). judge reads the
// words. git takes the subcommand from the first word alone, so one the
// shell may still rewrite may be either, and so may a missing one a command
// in front of git hands over (fed).
func worktreeDeletes(args []string, fed bool) bool {
	if len(args) == 0 {
		return fed
	}
	switch args[0] {
	case "remove":
		return true
	case "prune":
		w, err := readGitWords(args[1:], worktreePruneOptions)
		return fed || err != nil || !w.on("dry-run")
	}
	return !strings.HasPrefix(args[0], "-") && !asWritten(args[0])
}

// applyWrites: apply rewrites the files its patches touch, and may drop
// edits either way: -R reverts them, and a forward hunk replaces the lines
// it matches. It writes nothing when it only reports (applyReports), and
// keeps the worktree with --cached, which like a mixed reset only changes
// the index. --build-fake-ancestor writes its file in any mode.
func applyWrites(args []string) bool {
	w, err := readGitWords(args, applyOptions)
	switch {
	case err != nil || w.on("build-fake-ancestor"):
		return true
	case w.on("cached"):
		return false
	}
	return !applyReports(w)
}

// applyReports reports whether apply only checks or describes its patches:
// --check never applies them, and --stat, --numstat and --summary turn
// applying off unless --apply turns it back on. The last of an option and
// its --no- form decides. main-checkout reads apply through it too.
func applyReports(w gitWords) bool {
	if w.on("check") {
		return true
	}
	return (w.on("stat") || w.on("numstat") || w.on("summary")) && !w.on("apply")
}

// readTreeUpdates: read-tree writes the worktree with -u, over uncommitted
// edits with --reset and over staged ones with -m, and --index-output
// writes the index over whatever file it names. -n leaves both a dry run.
// Without either only the index changes, as in a mixed reset.
func readTreeUpdates(args []string) bool {
	w, err := readGitWords(args, readTreeOptions)
	return err != nil || ((w.sets("u") || w.sets("index-output")) && !w.on("dry-run"))
}

// checkoutIndexForces: checkout-index overwrites a worktree file from the
// index only with --force; without it an existing file is left alone, and
// --temp writes to new temporary files instead.
func checkoutIndexForces(args []string) bool {
	w, err := readGitWords(args, checkoutIndexOptions)
	return err != nil || (w.on("force") && !w.on("temp"))
}

// rmDeletes: rm deletes a file that differs from HEAD only with --force;
// without it git refuses one, --cached keeps the worktree file, and -n
// deletes nothing.
func rmDeletes(args []string) bool {
	w, err := readGitWords(args, rmOptions)
	return err != nil || (w.on("force") && !w.on("cached") && !w.on("dry-run"))
}

// mvOverwrites: mv overwrites a file at the destination only with --force;
// without it git refuses one that exists, and -n moves nothing.
func mvOverwrites(args []string) bool {
	w, err := readGitWords(args, mvOptions)
	return err != nil || (w.on("force") && !w.on("dry-run"))
}

// gitStatusPorcelain runs `git status --porcelain` in dir. ok is false when
// dir is empty or git fails (not a repo), which the guard treats as dirty.
// A submodule's `ignore` setting is overridden: under submodule.recurse a
// reset rewrites a dirty submodule that plain status would hide. So is
// status.showUntrackedFiles, which can hide every untracked file: normal
// lists an untracked directory once, as the default does, which is enough
// to tell a clean tree from a dirty one.
func gitStatusPorcelain(dir string, extra ...string) (string, bool) {
	if dir == "" {
		return "", false
	}
	args := append(
		[]string{"status", "--porcelain", "--ignore-submodules=none", "--untracked-files=normal"},
		extra...,
	)
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(string(out)), true
}

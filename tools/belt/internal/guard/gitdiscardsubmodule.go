package guard

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// submoduleRule says when a forced submodule deinit or update passes.
const submoduleRule = "It passes only when every submodule it touches is clean."

// submoduleTouch is what a forced `git submodule deinit` or `update` may
// rewrite or delete: the submodule work trees its pathspecs match, or every
// one.
type submoduleTouch struct {
	all   bool
	paths []string
}

// everySubmodule is the touch of a call belt cannot read word for word.
var everySubmodule = submoduleTouch{all: true}

// submoduleWords are the options of one `git submodule` subcommand as
// git-submodule.sh (git 2.55) matches them: flags take no value, values
// take the next word, and a word with one of prefixes carries its value.
type submoduleWords struct {
	flags, values map[string]bool
	prefixes      []string
}

// submoduleDeinitWords are the options of `git submodule deinit`.
var submoduleDeinitWords = submoduleWords{flags: wordSet("-f --force -q --quiet --all")}

// submoduleUpdateWords are the options of `git submodule update`.
var submoduleUpdateWords = submoduleWords{
	flags: wordSet(`-f --force -q --quiet -v --verbose --progress -i --init --require-init
		--remote -N --no-fetch -r --rebase --dissociate -m --merge --recursive --checkout
		--recommend-shallow --no-recommend-shallow --single-branch --no-single-branch`),
	values:   wordSet("--ref-format --reference --depth -j --jobs --filter"),
	prefixes: []string{"--ref-format=", "--reference=", "--depth=", "-j", "--jobs=", "--filter="},
}

// submoduleDrops are the git submodule subcommands that may drop work, with
// their options.
var submoduleDrops = map[string]submoduleWords{
	"deinit": submoduleDeinitWords,
	"update": submoduleUpdateWords,
}

// readSubmoduleCall reads a git call that may drop work in a submodule work
// tree: `git submodule deinit -f`, which deletes it, and `git submodule
// update -f`, which checks out over it. The words are read as
// git-submodule.sh reads them (readSubmoduleWords): before the subcommand
// only -q, --quiet and --cached, then the subcommand word exactly. Any
// other word there makes git stop with its usage. A word the shell may
// still rewrite may be any subcommand. A missing one is status, unless a
// command in front of git may hand one over (fed). A `submodule--helper
// deinit` or `update` touches every submodule, since belt does not read the
// helper's options. ok is false for any other call.
func readSubmoduleCall(c gitInvocation, fed bool) (touch submoduleTouch, ok bool) {
	switch c.sub {
	case "submodule--helper":
		if len(c.args) == 0 {
			return everySubmodule, fed
		}
		_, drops := submoduleDrops[unquoteWord(c.args[0])]
		return everySubmodule, drops || mayExpand(c.args[0])
	case "submodule":
	default:
		return submoduleTouch{}, false
	}
	for i, word := range c.args {
		if mayExpand(word) {
			return everySubmodule, true
		}
		w := unquoteWord(word)
		if o, drops := submoduleDrops[w]; drops {
			return readSubmoduleWords(c.args[i+1:], fed, o)
		}
		if w != "-q" && w != "--quiet" && w != "--cached" {
			return submoduleTouch{}, false
		}
	}
	return everySubmodule, fed
}

// readSubmoduleWords reads the words after `git submodule deinit` or
// `update` (scanSubmoduleArgs). ok says -f or --force came up. A word the
// shell may still rewrite, and words a command in front of git may hand
// over (fed), may be a -f or any path, so the call touches every
// submodule. So do --all and no pathspec at all.
func readSubmoduleWords(args []string, fed bool, o submoduleWords) (touch submoduleTouch, ok bool) {
	a := scanSubmoduleArgs(args, o)
	switch {
	case a.usage:
		return submoduleTouch{}, false
	case a.unread || fed:
		return everySubmodule, true
	case !a.forced:
		return submoduleTouch{}, false
	case a.all || len(a.paths) == 0:
		return everySubmodule, true
	}
	for _, p := range a.paths {
		if mayExpand(p) {
			return everySubmodule, true
		}
		touch.paths = append(touch.paths, unquoteWord(p))
	}
	return touch, true
}

// submoduleArgs is what the words after deinit or update say: whether -f
// and --all came up, and the pathspecs. unread marks a word the shell may
// still rewrite before the pathspecs, and usage a word git stops at.
type submoduleArgs struct {
	forced, all, unread, usage bool
	paths                      []string
}

// scanSubmoduleArgs reads the words after deinit or update as the script's
// loop does: an exact option from o, a value option taking the next word,
// `--` ending the options, any other word with a leading - stopping git
// with its usage, and the first other word starting the pathspecs. So
// `deinit a -f` names a path -f and does not force.
func scanSubmoduleArgs(args []string, o submoduleWords) submoduleArgs {
	var a submoduleArgs
	for i := 0; i < len(args); i++ {
		if mayExpand(args[i]) {
			return submoduleArgs{unread: true}
		}
		switch w := unquoteWord(args[i]); {
		case w == "-f" || w == "--force":
			a.forced = true
		case w == "--all" && o.flags[w]:
			a.all = true
		case o.values[w]:
			i++
			switch {
			case i == len(args) || unquoteWord(args[i]) == "":
				return submoduleArgs{usage: true}
			case mayExpand(args[i]):
				return submoduleArgs{unread: true}
			}
		case o.flags[w] || hasAnyPrefix(w, o.prefixes):
		case w == "--":
			a.paths = args[i+1:]
			return a
		case strings.HasPrefix(w, "-"):
			return submoduleArgs{usage: true}
		default:
			a.paths = args[i:]
			return a
		}
	}
	return a
}

// hasAnyPrefix reports whether word starts with one of prefixes.
func hasAnyPrefix(word string, prefixes []string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(word, p) {
			return true
		}
	}
	return false
}

// submoduleLoss is why a forced submodule deinit or update may lose work: a
// submodule it touches holds some, or belt could not read one. Its deny
// ends with submoduleRule.
type submoduleLoss struct{ reason string }

func (e submoduleLoss) Error() string { return e.reason }

// submoduleUnread is the loss when belt could not read a submodule the call
// touches.
func submoduleUnread(cause any) error {
	return submoduleLoss{fmt.Sprintf("belt could not read the submodules it touches (%v)", cause)}
}

// judgeSubmodules passes a forced deinit or update (readSubmoduleCall) when
// every submodule work tree it touches is clean (submoduleDirt), with one
// proofBudget for all its git calls. A global option may change how git
// matches the pathspecs, so with one the call touches every submodule.
func (g *GitDiscard) judgeSubmodules(c gitInvocation, touch submoduleTouch) error {
	if c.optioned {
		touch = everySubmodule
	}
	ctx, cancel := context.WithTimeout(context.Background(), proofBudget)
	defer cancel()
	return g.submoduleCheck(ctx, c.dir, touch)
}

// submoduleDirt returns nil when every submodule work tree touch names in the
// repository at dir is clean, else the first that holds work or why belt
// could not read one (submoduleTreeDirt). A GIT_DIR-style variable would
// point belt's own git calls at another repository, so it refuses.
func submoduleDirt(ctx context.Context, dir string, touch submoduleTouch) error {
	if gitEnvRedirected() {
		return submoduleUnread("a GIT_DIR-style variable is set")
	}
	out, err := runProofGit(ctx, dir, "-c", "core.fsmonitor=false", "rev-parse", "--show-toplevel")
	if err != nil {
		return submoduleUnread(err)
	}
	top := strings.TrimSpace(string(out))
	trees, err := submoduleTrees(ctx, submoduleExec(dir), top, touch)
	if err != nil {
		return submoduleUnread(err)
	}
	return submoduleTreesDirt(ctx, trees)
}

// submoduleTreesDirt returns nil when every submodule work tree in trees is
// clean, else the first loss (submoduleTreeDirt).
func submoduleTreesDirt(ctx context.Context, trees []string) error {
	for _, tree := range trees {
		if err := submoduleTreeDirt(ctx, tree); err != nil {
			return err
		}
	}
	return nil
}

// submoduleExec is how belt runs git for a submodule check in dir, with env
// and proofGitEnv added. core.fsmonitor is off, so the repository's config
// starts no monitor program inside the hook.
func submoduleExec(dir string, env ...string) gitExec {
	return gitExec{dir: dir, pre: []string{"-c", "core.fsmonitor=false"}, env: slices.Concat(env, proofGitEnv)}
}

// submoduleTrees returns the work trees of the submodules touch names in the
// repository whose work tree root is top: every gitlink in the index, or
// each one its pathspecs match from e's directory. git ls-files matches
// them, magic and all, as the submodule helper does.
func submoduleTrees(ctx context.Context, e gitExec, top string, touch submoduleTouch) ([]string, error) {
	args := []string{"ls-files", "--stage", "-z", "--full-name"}
	if touch.all {
		e.dir = top
	} else {
		args = append(append(args, "--"), touch.paths...)
	}
	out, _, err := execGit(ctx, e, args...)
	if err != nil {
		return nil, err
	}
	var trees []string
	for _, f := range nulFields(out) {
		meta, name, _ := strings.Cut(f, "\t")
		if strings.HasPrefix(meta, gitlinkMode+" ") {
			trees = appendNew(trees, []string{filepath.Join(top, name)})
		}
	}
	return trees, nil
}

// submoduleTreeDirt returns nil when the submodule work tree at tree holds
// nothing a forced deinit or update drops: it is missing, an empty folder,
// or a clean checkout (submoduleCheckoutDirt). A folder with files but no
// .git holds files no repository tracks, which deinit deletes with the
// folder.
func submoduleTreeDirt(ctx context.Context, tree string) error {
	info, err := os.Lstat(tree)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return submoduleUnread(err)
	case !info.IsDir():
		return submoduleUnread(tree + " is not a folder")
	}
	if _, err := os.Lstat(filepath.Join(tree, ".git")); err != nil {
		return untrackedSubmoduleFiles(tree, err)
	}
	return submoduleCheckoutDirt(ctx, tree)
}

// submoduleCheckoutDirt returns nil when the submodule checkout at tree is
// clean: git reads tree as its work tree (submoduleWorkTree), git status
// there is empty, and each submodule nested in it is clean too
// (submoduleTreeDirt), to any depth. Status does not list the files in a
// nested folder that is not checked out, which deinit deletes all the same.
// Each git call runs with the folder's parent as a ceiling, so a .git git
// cannot read fails it instead of letting git find the superproject. Status
// counts a nested checkout's changes and every untracked file, as
// gitStatusPorcelain does.
func submoduleCheckoutDirt(ctx context.Context, tree string) error {
	e := submoduleExec(tree, "GIT_CEILING_DIRECTORIES="+filepath.Dir(tree))
	if err := submoduleWorkTree(ctx, e, tree); err != nil {
		return err
	}
	out, _, err := execGit(ctx, e, "status", "--porcelain", "--ignore-submodules=none", "--untracked-files=normal")
	if err != nil {
		return submoduleUnread(err)
	}
	if first, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n"); first != "" {
		return submoduleLoss{fmt.Sprintf("the submodule at %s holds uncommitted work (`%s`)", tree, first)}
	}
	nested, err := submoduleTrees(ctx, e, tree, everySubmodule)
	if err != nil {
		return submoduleUnread(err)
	}
	return submoduleTreesDirt(ctx, nested)
}

// submoduleWorkTree returns nil when git, run as e, reads the submodule
// checkout at tree as its work tree. A core.worktree in the submodule's git
// directory may name another folder: status would read that one, while
// deinit deletes the folder at tree.
func submoduleWorkTree(ctx context.Context, e gitExec, tree string) error {
	out, _, err := execGit(ctx, e, "rev-parse", "--show-toplevel")
	if err != nil {
		return submoduleUnread(err)
	}
	want, err := filepath.EvalSymlinks(tree)
	if err != nil {
		return submoduleUnread(err)
	}
	if got := strings.TrimSuffix(string(out), "\n"); got != want {
		return submoduleLoss{fmt.Sprintf(
			"git reads the work tree of the submodule at %s from %s, so belt cannot read what the folder holds",
			tree, got,
		)}
	}
	return nil
}

// untrackedSubmoduleFiles judges a submodule folder with no .git, which
// lstatErr says: an empty one holds nothing, any file in it is in no
// repository.
func untrackedSubmoduleFiles(tree string, lstatErr error) error {
	if !errors.Is(lstatErr, fs.ErrNotExist) {
		return submoduleUnread(lstatErr)
	}
	entries, err := os.ReadDir(tree)
	switch {
	case err != nil:
		return submoduleUnread(err)
	case len(entries) > 0:
		return submoduleLoss{fmt.Sprintf(
			"the submodule folder %s is not checked out and holds files no repository tracks (`%s`)",
			tree, entries[0].Name(),
		)}
	}
	return nil
}

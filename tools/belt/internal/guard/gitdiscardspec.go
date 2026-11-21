package guard

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// discardKind names the discards the nothing-lost proof can read.
type discardKind int

const (
	// pathRestore writes paths back from the index or a tree: `checkout
	// [<tree>] -- <paths>`, `checkout .`, `restore <paths>`.
	pathRestore discardKind = iota + 1
	// untrackedClean deletes untracked files: `clean -f`.
	untrackedClean
	// hardReset moves the branch and rewrites index and worktree: `reset
	// --hard [<commit>]`.
	hardReset
	// mergeReset is a hard reset that keeps the worktree copies that differ
	// from the index: `reset --merge [<commit>]`.
	mergeReset
	// treeRead rewrites index and worktree from a tree and leaves the
	// branch alone: `read-tree -u --reset <tree>`, `read-tree -m -u <tree>`.
	treeRead
	// trackedRemoval deletes tracked files from index and worktree: `rm -f`.
	trackedRemoval
	// forcedMove overwrites what stands at each destination: `mv -f`.
	forcedMove
	// patchApply rewrites the files its patches touch: `apply`.
	patchApply
	// worktreeRemoval deletes a linked worktree with its index: `worktree
	// remove --force`, read in the worktree it removes.
	worktreeRemoval
)

// cleanScope says which untracked files a clean deletes.
type cleanScope int

const (
	// cleanUntracked is the default: untracked files the ignore rules keep.
	cleanUntracked cleanScope = iota
	// cleanWithIgnored is `-x`: untracked and ignored files.
	cleanWithIgnored
	// cleanOnlyIgnored is `-X`: ignored files alone.
	cleanOnlyIgnored
)

// discardSpec is what a discarding command overwrites, read from its
// arguments before any git runs.
type discardSpec struct {
	kind discardKind
	// source is the tree the command writes from. "" means the index for a
	// path restore; a hard reset always names one (HEAD by default).
	source string
	// pathspecs are the paths the command names, each a literal path
	// relative to the directory it runs in.
	pathspecs []string
	// staged marks a path restore or an apply that overwrites the index too.
	staged bool
	// overlay marks a path restore that leaves index paths the source lacks
	// in place (checkout), where restore deletes them.
	overlay bool
	// scope is which untracked files a clean deletes.
	scope cleanScope
	// dest is where a forced move puts its sources (pathspecs), as written.
	dest string
	// patches are the patch files an apply reads, and patchOpts its options
	// that change which paths the patches name, as git reads them.
	patches   []string
	patchOpts []string
}

// errNotRead is the cause for a discard shape the proof does not read.
var errNotRead = errors.New("a form belt does not read")

// errNoProof marks a discarding subcommand the proof never reads (stash,
// switch): only a clean tree passes it.
var errNoProof = errors.New("only a clean tree passes it")

// readDiscardSpec reads a discarding invocation into the spec the proof
// checks. It refuses anything it cannot read exactly: an option it does not
// know, a ref or path the shell could still rewrite, a glob or pathspec
// magic git would expand.
func readDiscardSpec(c gitInvocation) (discardSpec, error) {
	switch c.sub {
	case "checkout":
		return readCheckout(c.args)
	case "restore":
		return readRestore(c.args)
	case "clean":
		return readClean(c.args)
	case "reset":
		return readReset(c.args)
	case "read-tree":
		return readReadTree(c.args)
	case "checkout-index":
		return readCheckoutIndex(c.args)
	case "rm":
		return readGitRm(c.args)
	case "mv":
		return readMv(c.args)
	case "apply":
		return readApply(c.args)
	case "worktree":
		// judge reads a remove's words once (removedWorktrees) and runs the
		// proof in each worktree it removes, where every file goes, tracked
		// or not, and the index with it. The proof reads nothing from them.
		return discardSpec{kind: worktreeRemoval}, nil
	}
	return discardSpec{}, errNoProof
}

// readCheckout reads `checkout [-q] [-p] [<tree>] -- <paths>` and
// `checkout [-q] .`. A tree writes index and worktree; without one the
// worktree comes back from the index. Patch mode (-p) is read as taking
// every hunk, the most it drops, and from a tree it also deletes the paths
// the tree lacks, as git does for a file only the index has. Options are
// read as git reads them (readGitWords); any but -q and -p, `-f` included,
// is not read.
func readCheckout(args []string) (discardSpec, error) {
	w, err := readGitWords(args, checkoutOptions)
	if err != nil {
		return discardSpec{}, err
	}
	if err := onlyOptions(w, "quiet", "patch"); err != nil {
		return discardSpec{}, err
	}
	words := w.operands
	spec := discardSpec{kind: pathRestore, overlay: true, pathspecs: w.paths}
	switch {
	case w.dashed && len(words) == 1:
		spec.source, spec.staged, spec.overlay = words[0], true, !w.on("patch")
	case w.dashed && len(words) == 0:
	case !w.dashed && len(words) == 1 && words[0] == ".":
		spec.pathspecs = words
	default:
		return discardSpec{}, errNotRead
	}
	return spec, checkOperands(spec)
}

// readRestore reads `restore [-q] [-W] [-S] [-s <tree>] [--] <paths>`.
// Without a source the worktree comes back from the index, or from HEAD
// when the index is restored too. With one, paths the source lacks are
// deleted (restore's default no-overlay mode).
func readRestore(args []string) (discardSpec, error) {
	spec := discardSpec{kind: pathRestore}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			spec.pathspecs = append(spec.pathspecs, args[i+1:]...)
			i = len(args)
		case a == "-q" || a == "--quiet" || a == "-W" || a == "--worktree":
		case a == "-S" || a == "--staged" || a == "-SW" || a == "-WS":
			spec.staged = true
		case (a == "-s" || a == "--source") && i+1 < len(args):
			spec.source = args[i+1]
			i++
		case strings.HasPrefix(a, "--source="):
			spec.source = strings.TrimPrefix(a, "--source=")
		case strings.HasPrefix(a, "-"):
			return discardSpec{}, fmt.Errorf("option %s", a)
		default:
			spec.pathspecs = append(spec.pathspecs, a)
		}
	}
	if len(spec.pathspecs) == 0 {
		return discardSpec{}, errNotRead
	}
	if spec.staged && spec.source == "" {
		spec.source = "HEAD"
	}
	return spec, checkOperands(spec)
}

// readClean reads `clean -f [-d] [-x|-X] [-q] [--] [<paths>]`, its options
// as git reads them (readGitWords). An -e (--exclude) pattern is not read:
// a `!` pattern brings an ignored file back in, and under -X a plain one
// adds untracked files, so it can widen what the clean deletes past what
// the proof lists. Nor is -i, --no-force or any other option.
func readClean(args []string) (discardSpec, error) {
	w, err := readGitWords(args, cleanOptions)
	if err != nil {
		return discardSpec{}, err
	}
	spec := discardSpec{kind: untrackedClean, pathspecs: slices.Concat(w.operands, w.paths)}
	for _, o := range w.opts {
		switch {
		case o.name == "x":
			spec.scope = cleanWithIgnored
		case o.name == "X":
			spec.scope = cleanOnlyIgnored
		case o.name == "force" && !o.unset, o.name == "d", o.name == "quiet":
		default:
			return discardSpec{}, fmt.Errorf("option %s", o.word)
		}
	}
	return spec, checkOperands(spec)
}

// readReset reads `reset (--hard | --merge) [-q] [<commit>]`, its options as
// git reads them (readGitWords); git keeps the last mode given. A reset with
// paths is not one git runs in either mode, and any other option is not
// read.
func readReset(args []string) (discardSpec, error) {
	w, err := readGitWords(args, resetOptions)
	if err != nil {
		return discardSpec{}, err
	}
	var kind discardKind
	for _, o := range w.opts {
		switch o.name {
		case "hard":
			kind = hardReset
		case "merge":
			kind = mergeReset
		}
	}
	if w.dashed || kind == 0 {
		return discardSpec{}, errNotRead
	}
	if err := onlyOptions(w, "hard", "merge", "quiet"); err != nil {
		return discardSpec{}, err
	}
	spec := discardSpec{kind: kind, source: "HEAD"}
	switch len(w.operands) {
	case 0:
	case 1:
		spec.source = w.operands[0]
	default:
		return discardSpec{}, errNotRead
	}
	return spec, checkOperands(spec)
}

// readReadTree reads `read-tree -u (--reset | -m) [-q] [-v] <tree>`, its
// options as git reads them (readGitWords). Both forms are read as --reset,
// which overwrites every change git status lists; -m drops only the staged
// ones, since git refuses to overwrite an unstaged edit. Two or three
// trees, --prefix and any other option are not read.
func readReadTree(args []string) (discardSpec, error) {
	w, err := readGitWords(args, readTreeOptions)
	switch {
	case err != nil:
		return discardSpec{}, err
	case !w.sets("m") && !w.on("reset"):
		return discardSpec{}, errNotRead
	}
	if err := onlyOptions(w, "u", "m", "reset", "quiet", "verbose"); err != nil {
		return discardSpec{}, err
	}
	trees := slices.Concat(w.operands, w.paths)
	if len(trees) != 1 {
		return discardSpec{}, errNotRead
	}
	spec := discardSpec{kind: treeRead, source: trees[0]}
	return spec, checkOperands(spec)
}

// readCheckoutIndex reads `checkout-index -f [-q] [-u] [-n] (-a | [--]
// <file>...)` as a restore of those paths from the index; -a is every index
// path under the directory it runs in, as git limits it. --stdin, --prefix,
// --stage and any other option are not read.
func readCheckoutIndex(args []string) (discardSpec, error) {
	w, err := readGitWords(args, checkoutIndexOptions)
	if err != nil {
		return discardSpec{}, err
	}
	if err := onlyOptions(w, "all", "force", "quiet", "index", "no-create", "create"); err != nil {
		return discardSpec{}, err
	}
	spec := discardSpec{
		kind: pathRestore, overlay: true, pathspecs: slices.Concat(w.operands, w.paths),
	}
	if w.on("all") {
		spec.pathspecs = append(spec.pathspecs, ".")
	}
	if len(spec.pathspecs) == 0 {
		return discardSpec{}, errNotRead
	}
	return spec, checkOperands(spec)
}

// readGitRm reads `rm -f [-r] [-q] [--ignore-unmatch] [--] <pathspec>...`, its
// options as git reads them (readGitWords): every index path the pathspecs
// match goes from index and worktree. --pathspec-from-file, --sparse and any
// other option are not read.
func readGitRm(args []string) (discardSpec, error) {
	w, err := readGitWords(args, rmOptions)
	if err != nil {
		return discardSpec{}, err
	}
	if err := onlyOptions(w, "force", "r", "quiet", "ignore-unmatch"); err != nil {
		return discardSpec{}, err
	}
	spec := discardSpec{kind: trackedRemoval, pathspecs: slices.Concat(w.operands, w.paths)}
	if len(spec.pathspecs) == 0 {
		return discardSpec{}, errNotRead
	}
	return spec, checkOperands(spec)
}

// readMv reads `mv -f [-k] [-v] [--] <source>... <destination>`, its options
// as git reads them (readGitWords). --sparse and any other option are not
// read.
func readMv(args []string) (discardSpec, error) {
	w, err := readGitWords(args, mvOptions)
	if err != nil {
		return discardSpec{}, err
	}
	if err := onlyOptions(w, "force", "k", "verbose"); err != nil {
		return discardSpec{}, err
	}
	words := slices.Concat(w.operands, w.paths)
	if len(words) < 2 {
		return discardSpec{}, errNotRead
	}
	last := len(words) - 1
	spec := discardSpec{kind: forcedMove, pathspecs: words[:last], dest: words[last]}
	if !literalPathspec(spec.dest) {
		return discardSpec{}, fmt.Errorf("the pathspec %s", spec.dest)
	}
	return spec, checkOperands(spec)
}

// patchNameOptions are the apply options that change which paths a patch
// names or how its hunks parse, which the proof passes on when it lists
// them; patchValueOptions are those that take a value.
var (
	patchNameOptions  = wordSet("p directory include exclude reverse recount inaccurate-eof allow-empty")
	patchValueOptions = wordSet("p directory include exclude")
)

// patchApplyOptions are the other apply options the proof reads: how hunks
// match, what git prints, and --index or --3way, which write the index too.
var patchApplyOptions = wordSet(
	"index 3way ours theirs union C whitespace ignore-space-change ignore-whitespace " +
		"unidiff-zero allow-overlap no-add add intent-to-add verbose quiet z " +
		"stat numstat summary check apply cached",
)

// readApply reads `apply [<options>] <patch>...`, its options as git reads
// them (readGitWords): the files its patches touch are rewritten, and with
// --index or --3way their index entries too. A patch read from stdin cannot
// be read a second time, --reject writes .rej files beside the paths, and
// --unsafe-paths and --build-fake-ancestor write outside the tree, so none
// of them is read; nor is a value the shell may still rewrite on an option
// the proof passes on.
func readApply(args []string) (discardSpec, error) {
	w, err := readGitWords(args, applyOptions)
	if err != nil {
		return discardSpec{}, err
	}
	spec := discardSpec{
		kind:    patchApply,
		staged:  w.on("index") || w.on("3way"),
		patches: slices.Concat(w.operands, w.paths),
	}
	for _, o := range w.opts {
		switch {
		case patchValueOptions[o.name] && !o.unset && (o.value == "" || !asWritten(o.value)):
			return discardSpec{}, fmt.Errorf("option %s", o.word)
		case patchNameOptions[o.name]:
			spec.patchOpts = append(spec.patchOpts, o.asWord())
		case !patchApplyOptions[o.name]:
			return discardSpec{}, fmt.Errorf("option %s", o.word)
		}
	}
	if len(spec.patches) == 0 || slices.Contains(spec.patches, "-") {
		return discardSpec{}, errors.New("a patch read from stdin")
	}
	for _, p := range spec.patches {
		if !literalPathspec(p) {
			return discardSpec{}, fmt.Errorf("the patch %s", p)
		}
	}
	return spec, nil
}

// onlyOptions refuses a read option whose name is not in names: the proof
// reads no other.
func onlyOptions(w gitWords, names ...string) error {
	for _, o := range w.opts {
		if !slices.Contains(names, o.name) {
			return fmt.Errorf("option %s", o.word)
		}
	}
	return nil
}

// checkOperands refuses a source or path the proof cannot pass to git as
// the command will: a word the shell may still rewrite, a ref that reads as
// an option, a glob or pathspec magic git expands.
func checkOperands(spec discardSpec) error {
	if spec.source != "" && !plainRef(spec.source) {
		return fmt.Errorf("the ref %s", spec.source)
	}
	for _, p := range spec.pathspecs {
		if !literalPathspec(p) {
			return fmt.Errorf("the pathspec %s", p)
		}
	}
	return nil
}

// plainRef reports whether a ref token reaches git as written: no leading
// `-` or `~`, and only characters a ref, a reflog entry (`@{u}`) or a
// revision suffix (`~1`, `^`) uses.
func plainRef(ref string) bool {
	if ref == "" || ref[0] == '-' || ref[0] == '~' {
		return false
	}
	for _, r := range ref {
		ok := r == '.' || r == '_' || r == '/' || r == '-' || r == '@' || r == '{' ||
			r == '}' || r == '^' || r == '~' || r == '+' ||
			('a' <= r && r <= 'z') || ('A' <= r && r <= 'Z') || ('0' <= r && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

// literalPathspec reports whether a pathspec names one literal path the way
// git reads it: no glob or escape character git would match on, no `:`
// magic, and nothing the shell could still expand (unresolvableChars).
func literalPathspec(p string) bool {
	return p != "" && p[0] != ':' && !strings.ContainsAny(p, unresolvableChars+`\`)
}

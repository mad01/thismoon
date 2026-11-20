package guard

import (
	"errors"
	"fmt"
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
	// staged marks a path restore that overwrites the index too.
	staged bool
	// overlay marks a path restore that leaves index paths the source lacks
	// in place (checkout), where restore deletes them.
	overlay bool
	// scope is which untracked files a clean deletes.
	scope cleanScope
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
	}
	return discardSpec{}, errNoProof
}

// readCheckout reads `checkout [-q] [-p] [<tree>] -- <paths>` and
// `checkout [-q] .`. A tree writes index and worktree; without one the
// worktree comes back from the index. Patch mode (-p) is read as taking
// every hunk, the most it drops, and from a tree it also deletes the paths
// the tree lacks, as git does for a file only the index has. Any other
// flag, `-f` included, is not read.
func readCheckout(args []string) (discardSpec, error) {
	before, paths, dashed := splitAtDashes(args)
	var words []string
	patch := false
	for _, a := range before {
		read, p := readCheckoutFlag(a)
		switch {
		case read:
			patch = patch || p
		case strings.HasPrefix(a, "-"):
			return discardSpec{}, fmt.Errorf("option %s", a)
		default:
			words = append(words, a)
		}
	}
	spec := discardSpec{kind: pathRestore, overlay: true, pathspecs: paths}
	switch {
	case dashed && len(words) == 1:
		spec.source, spec.staged, spec.overlay = words[0], true, !patch
	case dashed && len(words) == 0:
	case !dashed && len(words) == 1 && words[0] == ".":
		spec.pathspecs = words
	default:
		return discardSpec{}, errNotRead
	}
	return spec, checkOperands(spec)
}

// readCheckoutFlag reports whether readCheckout reads a checkout option,
// and whether it selects patch mode: -q, -p, a bundle of the two, --quiet
// and --patch.
func readCheckoutFlag(a string) (read, patch bool) {
	switch {
	case a == "--quiet":
		return true, false
	case a == "--patch":
		return true, true
	case len(a) < 2 || a[0] != '-' || strings.Trim(a[1:], "qp") != "":
		return false, false
	}
	return true, strings.Contains(a, "p")
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

// readClean reads `clean -f [-d] [-x|-X] [-q] [--] [<paths>]`. An -e
// (--exclude) pattern is not read: a `!` pattern brings an ignored file
// back in, and under -X a plain one adds untracked files, so it can widen
// what the clean deletes past what the proof lists.
func readClean(args []string) (discardSpec, error) {
	spec := discardSpec{kind: untrackedClean}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--":
			spec.pathspecs = append(spec.pathspecs, args[i+1:]...)
			i = len(args)
		case a == "--force" || a == "--quiet":
		case strings.HasPrefix(a, "--"):
			return discardSpec{}, fmt.Errorf("option %s", a)
		case strings.HasPrefix(a, "-"):
			if err := readCleanShort(a, &spec); err != nil {
				return discardSpec{}, err
			}
		default:
			spec.pathspecs = append(spec.pathspecs, a)
		}
	}
	return spec, checkOperands(spec)
}

// readCleanShort reads one short-flag token of clean (`-fdx`) into spec.
// Any letter it does not know, -e included, is not read.
func readCleanShort(tok string, spec *discardSpec) error {
	for i := 1; i < len(tok); i++ {
		switch tok[i] {
		case 'f', 'd', 'q':
		case 'x':
			spec.scope = cleanWithIgnored
		case 'X':
			spec.scope = cleanOnlyIgnored
		default:
			return fmt.Errorf("option %s", tok)
		}
	}
	return nil
}

// readReset reads `reset --hard [-q] [<commit>]`. A reset with paths is
// not one git runs with --hard, and any other flag is not read.
func readReset(args []string) (discardSpec, error) {
	spec := discardSpec{kind: hardReset, source: "HEAD"}
	var words []string
	for _, a := range args {
		switch {
		case a == "--hard" || a == "-q" || a == "--quiet":
		case strings.HasPrefix(a, "-"):
			return discardSpec{}, fmt.Errorf("option %s", a)
		default:
			words = append(words, a)
		}
	}
	switch len(words) {
	case 0:
	case 1:
		spec.source = words[0]
	default:
		return discardSpec{}, errNotRead
	}
	return spec, checkOperands(spec)
}

// splitAtDashes splits args at the first `--`: the words before it, the
// paths after it, and whether there was one.
func splitAtDashes(args []string) (before, after []string, dashed bool) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:], true
		}
	}
	return args, nil, false
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

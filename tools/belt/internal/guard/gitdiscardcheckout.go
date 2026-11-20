package guard

import (
	"context"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/kit/gitloc"
)

// treeLookupBudget bounds the ref lookups one Check makes, all of them
// together (gitIsTree): fifteen checkouts in one command must not run the
// hook past its timeout.
const treeLookupBudget = 2 * time.Second

// checkoutValueFlags are the checkout options that take the next word as
// their value (`git checkout -h`). Their = forms are one word.
var checkoutValueFlags = wordSet(
	"-b -B -U --orphan --conflict --unified --inter-hunk-context --pathspec-from-file",
)

// checkoutShortValues are the short checkout options that take a value. In
// a bundle the first of them takes the rest of the word (`-bname`), or the
// next word when it ends the bundle (`-qb name`).
const checkoutShortValues = "bBU"

// checkoutBundleEnds are the short checkout options that end a bundle:
// checkoutShortValues, and -t, whose optional value is the rest of the word.
const checkoutBundleEnds = checkoutShortValues + "t"

// switchBundleEnds are the short switch options that end a bundle: -c and
// -C take a value, -t an optional one (`git switch -h`).
const switchBundleEnds = "cCt"

// sortCheckoutOperands rewrites a checkout into the form git reads it as,
// so discards and the proof see its paths. Without `--`, git reads the
// first operand as a tree-ish when it names one (`-` is @{-1}): alone it is
// a branch switch and stays as written, and the operands after it are paths
// restored from it (`checkout <tree> -- <paths>`). When it names none, every
// operand is a path restored from the index (`checkout -- <paths>`). A first
// operand that is both a ref and a path reads as the ref, as git reads it
// alone; with more operands git refuses it as ambiguous. A patch checkout
// that names no path walks the whole tree (patchWholeTree). err, when set,
// says why the proof cannot vouch for the form (sortOperands).
func (g *GitDiscard) sortCheckoutOperands(
	ctx context.Context,
	c gitInvocation,
) (_ gitInvocation, err error) {
	if c.sub != "checkout" {
		return c, nil
	}
	if !slices.Contains(c.args, "--") {
		c.args, err = g.sortOperands(ctx, c.dir, c.args)
	}
	c.args = patchWholeTree(c.args)
	return c, err
}

// sortOperands sorts the words of a checkout without `--` in dir (see
// sortCheckoutOperands). A first operand belt cannot look up (a word git may
// read past plainRef, a directory belt cannot name, a lookup past the
// budget) reads as a path. Alone that is the wider reading, since a switch
// drops nothing; in patch mode or with more operands git may restore from
// it instead, so the error says the proof cannot vouch for the paths.
func (g *GitDiscard) sortOperands(
	ctx context.Context,
	dir string,
	args []string,
) ([]string, error) {
	opts, operands := splitCheckoutArgs(args)
	if len(operands) == 0 {
		return args, nil
	}
	first := operands[0]
	if first == "-" {
		first = "@{-1}"
	}
	tree, known := g.lookupTree(ctx, dir, first)
	switch {
	case tree && len(operands) == 1:
		return args, nil
	case tree:
		return slices.Concat(opts, []string{first, "--"}, operands[1:]), nil
	}
	paths := slices.Concat(opts, []string{"--"}, operands)
	if !known && (checkoutPatches(opts) || len(operands) > 1) {
		return paths, notListed(first + " may name a branch belt could not look up")
	}
	return paths, nil
}

// lookupTree reports whether rev names a tree-ish in the repository at dir,
// and whether belt could tell: not for an empty dir, a rev plainRef refuses,
// or a lookup that fails on the budget.
func (g *GitDiscard) lookupTree(ctx context.Context, dir, rev string) (tree, known bool) {
	if dir == "" || !plainRef(rev) {
		return false, false
	}
	tree, err := g.isTree(ctx, dir, rev)
	return tree && err == nil, err == nil
}

// patchWholeTree gives a patch checkout that names no path the path `.`:
// git then walks the whole tree, `-p` as `-- .` and `-p <tree>` as `<tree>
// -- .`. Taking every hunk is the most it drops, so the proof judges that.
func patchWholeTree(args []string) []string {
	before, after, dashed := splitAtDashes(args)
	if len(after) > 0 || !checkoutPatches(before) {
		return args
	}
	if !dashed {
		args = append(slices.Clone(args), "--")
	}
	return append(args, ".")
}

// checkoutPatches reports whether checkout options select patch mode.
func checkoutPatches(opts []string) bool {
	return setsOption(opts, 'p', checkoutBundleEnds, "--patch")
}

// setsOption reports whether the options of a checkout or switch, the words
// before any `--`, set the short option letter or a long option in long. A
// letter counts in a bundle (`-qf`) up to a letter in ends, which takes the
// rest of the word as its value the way git reads it: `-fb x` forces, `-bf`
// creates a branch named f. A long option counts with an = value and as any
// prefix git may take for it (`--forc`); a prefix git finds ambiguous fails
// before git changes anything, so counting it errs toward a deny.
func setsOption(args []string, letter byte, ends string, long ...string) bool {
	before, _, _ := splitAtDashes(args)
	return slices.ContainsFunc(before, func(word string) bool {
		if name, ok := strings.CutPrefix(word, "--"); ok {
			name, _, _ = strings.Cut(name, "=")
			return name != "" && slices.ContainsFunc(long, func(opt string) bool {
				return strings.HasPrefix(opt, "--"+name)
			})
		}
		return bundleSets(word, letter, ends)
	})
}

// bundleSets reports whether a short option word (`-f`, `-qf`) sets letter
// before any letter in ends.
func bundleSets(word string, letter byte, ends string) bool {
	if len(word) < 2 || word[0] != '-' {
		return false
	}
	for i := 1; i < len(word); i++ {
		switch {
		case word[i] == letter:
			return true
		case strings.IndexByte(ends, word[i]) >= 0:
			return false
		}
	}
	return false
}

// splitCheckoutArgs splits the words of a checkout without `--` into its
// options, each with its value, and its operands, in order. git reads an
// option after an operand too (`checkout f -q`), and a lone `-` is an
// operand.
func splitCheckoutArgs(args []string) (opts, operands []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-" || !strings.HasPrefix(a, "-"):
			operands = append(operands, a)
		case checkoutTakesNext(a) && i+1 < len(args):
			opts = append(opts, a, args[i+1])
			i++
		default:
			opts = append(opts, a)
		}
	}
	return opts, operands
}

// checkoutTakesNext reports whether a checkout option takes the next word as
// its value: a long one in checkoutValueFlags, or a short bundle whose first
// value letter ends it.
func checkoutTakesNext(opt string) bool {
	if strings.HasPrefix(opt, "--") {
		return checkoutValueFlags[opt]
	}
	return strings.IndexAny(opt[1:], checkoutShortValues) == len(opt)-2
}

// pathspecFromFile reports whether a word is checkout's --pathspec-from-file,
// with or without a value, or a prefix of it git may accept for it.
func pathspecFromFile(word string) bool {
	const name = "--pathspec-from-file"
	opt, _, _ := strings.Cut(word, "=")
	return len(opt) >= len("--pathspec-fr") && strings.HasPrefix(name, opt)
}

// fedOperands reports whether a command in front of git may hand it words
// belt never sees: xargs, find -exec, parallel, or any word but an
// assignment or a wrapper that runs git as written (stripWrappers).
func fedOperands(tokens []string) bool {
	pre, _ := beforeGit(tokens)
	return len(stripWrappers(pre)) > 0
}

// gitIsTree reports whether rev names a tree-ish in the repository at dir:
// a branch, tag, commit or tree. It is the one git call that sorts a
// checkout's operands. A failed lookup reads as no tree; err is set only
// when ctx ran out first, and then the answer is unknown.
func gitIsTree(ctx context.Context, dir, rev string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	out := gitloc.Output(
		ctx, dir, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{tree}",
	)
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return out != "", nil
}

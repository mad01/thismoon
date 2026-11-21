package guard

import (
	"context"
	"slices"
	"time"

	"github.com/mad01/thismoon/kit/gitloc"
)

// treeLookupBudget bounds the ref lookups one Check makes, all of them
// together (gitIsTree): fifteen checkouts in one command must not run the
// hook past its timeout.
const treeLookupBudget = 2 * time.Second

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
// sortCheckoutOperands), its options read as git reads them (readGitWords).
// A first operand belt cannot look up (a word git may read past plainRef, a
// directory belt cannot name, a lookup past the budget) reads as a path.
// Alone that is the wider reading, since a switch drops nothing; in patch
// mode or with more operands git may restore from it instead, so the error
// says the proof cannot vouch for the paths. Words readGitWords refuses stay
// as written, and the error says the same.
func (g *GitDiscard) sortOperands(
	ctx context.Context,
	dir string,
	args []string,
) ([]string, error) {
	w, err := readGitWords(args, checkoutOptions)
	switch {
	case err != nil:
		return args, notListed(err)
	case len(w.operands) == 0:
		return args, nil
	}
	operands := w.operands
	first := operands[0]
	if first == "-" {
		first = "@{-1}"
	}
	tree, known := g.lookupTree(ctx, dir, first)
	switch {
	case tree && len(operands) == 1:
		return args, nil
	case tree:
		return slices.Concat(w.optWords, []string{first, "--"}, operands[1:]), nil
	}
	paths := slices.Concat(w.optWords, []string{"--"}, operands)
	if !known && (w.on("patch") || len(operands) > 1) {
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
// Words readGitWords refuses stay as written.
func patchWholeTree(args []string) []string {
	w, err := readGitWords(args, checkoutOptions)
	if err != nil || len(w.paths) > 0 || !w.on("patch") {
		return args
	}
	if !w.dashed {
		args = append(slices.Clone(args), "--")
	}
	return append(args, ".")
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

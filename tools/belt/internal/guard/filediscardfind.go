package guard

import (
	"path"
	"regexp"
	"slices"
	"strings"
)

// This file holds find for file-discard: the reader of its options and
// expression, the -delete and -exec judge, and find as a feeder.

// findCall is a find command read for file-discard: its roots as written,
// how it treats symlinks ('P' none, 'H' the roots, 'L' every one), each
// action that deletes or prints, and the tests of the whole expression
// (end), nil when a part belt does not read may widen what matches.
// printsOther marks an action that prints more than paths, and why says
// why belt cannot read the command at all.
type findCall struct {
	roots       []shWord
	follow      byte
	actions     []findAction
	end         *pathFilter
	printsOther bool
	why         string
}

// findAction is one action of a find: -print or -print0 (print), the
// command -exec, -execdir, -ok or -okdir runs (exec; inMatchDir for the two
// that run it in the match's folder), or else -delete. filter holds the
// tests in front of it, nil when a part belt does not read comes first.
type findAction struct {
	print      bool
	exec       []shWord
	inMatchDir bool
	filter     *pathFilter
}

// The find primaries file-discard reads besides -name, -iname and -type:
// the ones that change nothing it judges, the global ones with a value,
// the ones that print more than paths, and the ones that run a command.
// -prune is true and only keeps find out of a folder, so the tests in
// front of it still narrow the action after it, and what it skips only
// narrows what belt reads as at risk.
var (
	findNeutral = wordSet("-a -and -depth -d -xdev -mount -noleaf -ignore_readdir_race " +
		"-noignore_readdir_race -nowarn -warn -daystart -true -prune")
	findWithValue   = wordSet("-maxdepth -mindepth")
	findPrintsOther = wordSet("-ls -fls -printf -fprintf -fprint -fprint0")
	findExecs       = wordSet("-exec -execdir -ok -okdir")
)

// findDeletes reports whether a find's words hold an action that may
// delete, so only such a find is read.
func findDeletes(args []shWord) bool {
	return slices.ContainsFunc(args, func(w shWord) bool {
		v, ok := w.literal()
		return ok && (v == "-delete" || findExecs[v])
	})
}

// readFind reads a find command, GNU's or BSD's: the options before the
// roots, the roots (default .), then the expression (readExpression).
func readFind(args []shWord) findCall {
	fc := findCall{follow: 'P'}
	i := fc.readOptions(args)
	for ; i < len(args); i++ {
		if v, ok := args[i].literal(); ok && (len(v) > 1 && v[0] == '-' || v == "(" || v == ")" || v == "!" || v == ",") {
			break
		}
		fc.roots = append(fc.roots, args[i])
	}
	if len(fc.roots) == 0 {
		fc.roots = []shWord{literalWord(".")}
	}
	fc.readExpression(args[i:])
	return fc
}

// findTests are the tests a find expression has read so far. opaque marks
// a part belt does not read, which may widen what matches.
type findTests struct {
	filter pathFilter
	opaque bool
}

// snapshot is the filter of the tests so far, nil when they match every
// path or a part belt does not read came first.
func (t *findTests) snapshot() *pathFilter {
	if t.opaque || !t.filter.hasTests() {
		return nil
	}
	f := t.filter
	f.names = slices.Clone(t.filter.names)
	return &f
}

// read reads the test at args[*i] and its value: -name, -iname, -type, a
// global option with a value, or one that changes nothing judged. It
// reports false for any other word.
func (t *findTests) read(v string, args []shWord, i *int) bool {
	switch {
	case v == "-name" || v == "-iname":
		*i++
		n, ok := findPattern(args, *i, v == "-iname")
		if t.opaque = t.opaque || !ok; ok {
			t.filter.names = append(t.filter.names, n)
		}
	case v == "-type":
		*i++
		types, ok := findTypes(args, *i, t.filter.types)
		t.opaque, t.filter.types = t.opaque || !ok, types
	case findWithValue[v]:
		*i++
	case !findNeutral[v]:
		return false
	}
	return true
}

// readExpression reads find's expression. Tests joined by and narrow what
// an action takes, so each action keeps the tests in front of it; any part
// belt does not read (!, parentheses, -path, -newer) may widen it, and
// every action after it is taken to match every path. find runs what
// follows an -o outside parentheses only where what comes before it is
// false, so an action there keeps the tests between the -o and it, as in
// `-name node_modules -prune -o -name '*.pyc' -delete`; the expression as
// a whole (end) then matches either side, which no filter holds.
func (fc *findCall) readExpression(args []shWord) {
	var tests findTests
	grouped, joined := false, false
	for i := 0; i < len(args); i++ {
		v, ok := args[i].literal()
		switch {
		case !ok:
			tests.opaque = true
		case (v == "-o" || v == "-or") && !grouped:
			tests, joined = findTests{}, true
		case v == "(" || v == ")":
			grouped, tests.opaque = true, true
		case tests.read(v, args, &i):
		case v == "-follow":
			fc.follow = 'L'
		case v == "-print" || v == "-print0":
			fc.actions = append(fc.actions, findAction{print: true, filter: tests.snapshot()})
		case findPrintsOther[v]:
			fc.printsOther, tests.opaque = true, true
		case v == "-delete":
			fc.actions = append(fc.actions, findAction{filter: tests.snapshot()})
		case findExecs[v]:
			end := execEnd(args, i+1)
			if end < 0 {
				fc.why = "belt cannot find the ; or + that ends find's " + v
				return
			}
			fc.actions = append(fc.actions, findAction{
				exec: args[i+1 : end], inMatchDir: v == "-execdir" || v == "-okdir", filter: tests.snapshot(),
			})
			i = end
		case v == "-files0-from":
			fc.why = "find reads its roots from a file belt cannot see"
			return
		default:
			tests.opaque = true
		}
	}
	if fc.end = tests.snapshot(); joined {
		fc.end = nil
	}
}

// readOptions reads the options before find's roots and returns where the
// roots start: GNU's -H, -L, -P, -D and -O, and BSD's -E, -H, -L, -P, -X,
// -d, -s and -x, which may share a word, and BSD's -f, which names a root.
func (fc *findCall) readOptions(args []shWord) int {
	for i := 0; i < len(args); i++ {
		v, ok := args[i].literal()
		switch {
		case !ok || len(v) < 2 || v[0] != '-':
			return i
		case v == "--":
			return i + 1
		case v == "-D":
			i++
		case v == "-f" && i+1 < len(args):
			i++
			fc.roots = append(fc.roots, args[i])
		case strings.HasPrefix(v, "-O") && strings.Trim(v[2:], "0123456789") == "":
		case strings.Trim(v[1:], "HLPEXdsx") == "":
			for _, c := range v[1:] {
				if c == 'H' || c == 'L' || c == 'P' {
					fc.follow = byte(c)
				}
			}
		default:
			return i
		}
	}
	return len(args)
}

// findPattern reads the -name or -iname pattern at args[i] as path.Match
// reads it: a shell's `[!` negation is Go's `[^`. A POSIX class or a
// pattern path.Match rejects is not read.
func findPattern(args []shWord, i int, fold bool) (findName, bool) {
	if i >= len(args) {
		return findName{}, false
	}
	p, ok := args[i].literal()
	if !ok || strings.Contains(p, "[:") || strings.Contains(p, "[=") || strings.Contains(p, "[.") {
		return findName{}, false
	}
	p = strings.ReplaceAll(p, "[!", "[^")
	if _, err := path.Match(p, ""); err != nil {
		return findName{}, false
	}
	return findName{pattern: p, fold: fold}, true
}

// findTypes reads the -type letters at args[i] (GNU takes several joined
// by commas) and keeps those in had too, since tests joined by and must
// all hold. No letter left matches nothing.
func findTypes(args []shWord, i int, had string) (string, bool) {
	if i >= len(args) {
		return had, false
	}
	v, ok := args[i].literal()
	if !ok || v == "" {
		return had, false
	}
	letters := strings.ReplaceAll(v, ",", "")
	if strings.Trim(letters, "fdlpscbD") != "" {
		return had, false
	}
	if had == "" {
		return letters, true
	}
	kept := "!"
	for _, c := range letters {
		if strings.ContainsRune(had, c) {
			kept += string(c)
		}
	}
	return kept, true
}

// execEnd returns where the command of an -exec starting at from ends: a
// `;`, or a `+` right after `{}`; -1 when none does.
func execEnd(args []shWord, from int) int {
	for k := from; k < len(args); k++ {
		v, ok := args[k].literal()
		switch {
		case !ok:
		case v == ";":
			return k
		case v == "+" && k > from:
			if prev, ok := args[k-1].literal(); ok && prev == "{}" {
				return k
			}
		}
	}
	return -1
}

// feedRoots are the roots a find action hands paths from.
func (fc findCall) feedRoots(filter *pathFilter) []feedRoot {
	roots := make([]feedRoot, len(fc.roots))
	for i, w := range fc.roots {
		roots[i] = feedRoot{word: w, tree: true, follows: fc.follow == 'H', filter: filter}
	}
	return roots
}

// findMarks are the texts find fills in an -exec command: {}, which GNU
// and BSD find fill in anywhere in a word for the ; form.
var findMarks = regexp.MustCompile(`\{\}`)

// find judges a find that deletes: each -delete takes what its tests match
// under every root, and the command each -exec, -execdir, -ok or -okdir
// runs (discardJudge.consumer) takes the matches it is handed as `{}`.
// -L follows symlinks out of the roots, so belt does not read it.
func (j *discardJudge) find(args []shWord, s walkStep) *fileLoss {
	if !findDeletes(args) {
		return nil
	}
	fc := readFind(args)
	call := &lossCall{toss: "have find hand what it matches to the trash: `-exec " + TossBin +
		" --safe-mode -r {} +`"}
	if fc.why != "" {
		return &fileLoss{call: call, why: fc.why}
	}
	for _, a := range fc.actions {
		if a.print {
			continue
		}
		if fc.follow == 'L' {
			return &fileLoss{call: call, why: "find -L follows symlinks out of its roots, so belt " +
				"cannot tell what it deletes"}
		}
		fd := &pathFeed{mark: "{}", roots: fc.feedRoots(a.filter), inMatchDir: a.inMatchDir}
		var loss *fileLoss
		if a.exec == nil {
			loss = j.fed(call, lossOperand{}, fd, s)
		} else {
			loss = j.consumer(consumerRun{
				who: "find", command: a.exec, mark: "{}", marks: findMarks, feeds: []*pathFeed{fd}, call: call,
			}, s)
		}
		if loss != nil {
			return loss
		}
	}
	return nil
}

// findFeed reads the paths a find prints: its roots, narrowed by the tests
// in front of its one -print, or of the whole expression when it has no
// action. A find that runs a command prints what that command prints, and
// -ls, -printf and the -f forms print more than paths or print elsewhere,
// so belt reads neither.
func findFeed(args []shWord) ([]feedRoot, string) {
	fc := readFind(args)
	switch {
	case fc.why != "":
		return nil, fc.why
	case fc.follow == 'L':
		return nil, "find -L follows symlinks out of its roots"
	case fc.printsOther:
		return nil, "find prints more than paths there"
	}
	var prints []findAction
	deletes := false
	for _, a := range fc.actions {
		switch {
		case a.exec != nil:
			return nil, "find -exec prints what its command prints"
		case a.print:
			prints = append(prints, a)
		default:
			deletes = true
		}
	}
	filter := fc.end
	switch {
	case len(prints) == 1:
		filter = prints[0].filter
	case len(prints) > 1:
		filter = nil
	case deletes:
		return nil, ""
	}
	return fc.feedRoots(filter), ""
}

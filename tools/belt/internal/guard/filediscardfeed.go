package guard

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// This file holds the feeders whose output names the paths a delete drops
// (`git ls-files | xargs rm`, `rm $(grep -rl x .)`): how a feed is judged, and
// the readers of git ls-files, git grep, grep and rg (find's and fd's are
// in their own files).

// feedMark is the operand that stands for the paths xargs appends to its
// command. It never occurs in shell text.
const feedMark = "\x00feed"

// substSplit are the bytes at which the shell cuts an unquoted $( ) into
// words, the default IFS, and the bytes that then make a word a glob bash
// expands.
const substSplit = " \t\n*?["

// feedRoot is one path a feeder hands paths from, as written relative to
// the shell's directory: tree when the paths may lie anywhere below it,
// follows when the root is taken through a symlink, and the filter that
// narrows them (nil for all).
type feedRoot struct {
	word    shWord
	tree    bool
	follows bool
	filter  *pathFilter
}

// pathFeed stands for the paths a feeder hands a command: the operand text
// that marks them (`{}`, xargs' -I text or feedMark), the roots they come
// from, or why belt cannot tell. inMatchDir marks find's -execdir, whose
// other relative operands name a path in each match's folder. split holds
// the bytes at which the command cuts what the feeder prints into paths
// (pathFilter.split).
type pathFeed struct {
	mark       string
	roots      []feedRoot
	why        string
	inMatchDir bool
	split      string
}

// literalWord is a path as a word the shell expands nothing in.
func literalWord(p string) shWord {
	return shWord{raw: p, parts: []wordPart{{kind: partLit, text: p, quoted: true}}}
}

// fed judges the paths a feeder hands a loss command as one operand: each
// root as a drop, every match taken whole when the command drops trees, and
// each printed path checked whole when the command cuts what it reads. A
// command that writes through a symlink also writes the file each link
// below a root leads to (fedLinks).
func (j *discardJudge) fed(call *lossCall, consumer lossOperand, fd *pathFeed, s walkStep) *fileLoss {
	if fd.why != "" {
		return j.unknown(&fileLoss{call: call, why: fd.why})
	}
	for _, r := range fd.roots {
		op := lossOperand{word: r.word, tree: r.tree, follows: r.follows || consumer.follows}
		if r.filter != nil || fd.split != "" {
			f := pathFilter{all: true}
			if r.filter != nil {
				f = *r.filter
			}
			f.tree, f.split = consumer.tree, fd.split
			op.filter = &f
		}
		if loss := j.operand(call, op, s); loss != nil {
			return loss
		}
		if consumer.follows && r.tree {
			if loss := j.fedLinks(call, r, s); loss != nil {
				return loss
			}
		}
	}
	return nil
}

// fedLinks judges the files a command that writes through a symlink (sed
// --follow-symlinks, cp) reaches by way of the links a feeder hands it
// below the root r: git status in the root's tree sees each link as a file
// of its own, while the file it leads to may hold work in another tree.
// Each link the walk finds that the feeder's tests may match is followed
// to the end and judged as one file (drop). A root belt cannot place, or a
// walk past its cap, denies. The walk reads node_modules the way the
// search for hidden repositories does (walkMode).
func (j *discardJudge) fedLinks(call *lossCall, r feedRoot, s walkStep) *fileLoss {
	const through = "the command writes through symlinks, and belt "
	p, _, ok := j.placeDest(r.word, s)
	if !ok {
		return j.unknown(&fileLoss{call: call, why: through + "cannot tell which files the links " +
			"under `" + r.word.raw + "` lead to"})
	}
	root := physical(p)
	if !isDir(root) {
		return nil
	}
	var loss *fileLoss
	visit := func(dir string, entries []fs.DirEntry) walkNext {
		if loss = j.linkTargets(call, r.filter, dir, entries); loss != nil {
			return walkStop
		}
		return walkOn
	}
	err := j.walkDirs(root, walkMode{skipGit: true, packages: true}, visit)
	switch {
	case errors.Is(err, errWalkTooBig):
		return &fileLoss{call: call, why: fmt.Sprintf(through+"could not check every link under `%s`: "+
			"it stops at %d folders and the time budget", root, j.g.scanDirs)}
	case err != nil:
		return &fileLoss{call: call, why: fmt.Sprintf(through+"could not read every folder under `%s` (%v)",
			root, err)}
	}
	return loss
}

// linkTargets judges the file each symlink in a folder's entries leads to,
// for the links the feeder's tests f may match.
func (j *discardJudge) linkTargets(
	call *lossCall, f *pathFilter, dir string, entries []fs.DirEntry,
) *fileLoss {
	for _, e := range entries {
		if e.Type()&fs.ModeSymlink == 0 || f.hasTests() && !f.matchEntry(e.Name(), 'l') {
			continue
		}
		if loss := j.drop(call, false, nil, physical(filepath.Join(dir, e.Name()))); loss != nil {
			return loss
		}
	}
	return nil
}

// substBody returns the command text of a word that is one whole $( ) or
// backtick substitution.
func substBody(w shWord) (string, bool) {
	if len(w.parts) != 1 || w.parts[0].kind != partSubst {
		return "", false
	}
	return w.parts[0].text, true
}

// substFeed reads the paths a substitution prints: one simple command
// that is a feeder belt reads (feederRoots). Unquoted, the shell cuts what
// it prints at blanks and newlines and expands each piece as a glob.
func substFeed(w shWord, body string) *pathFeed {
	cmds, ok := readShell(body)
	if !ok || len(cmds) != 1 || len(cmds[0].subshells) > 0 || len(cmds[0].docs) > 0 {
		return &pathFeed{why: "belt cannot tell which paths `" + w.raw + "` names"}
	}
	roots, why := feederRoots(plainWords(cmds[0].words))
	if why != "" {
		why = "belt cannot tell which paths `" + w.raw + "` names: " + why
	}
	fd := &pathFeed{roots: roots, why: why}
	if !w.parts[0].quoted {
		fd.split = substSplit
	}
	return fd
}

// stdinFeed reads the paths the command in front of a consumer that reads
// its standard input (xargs, parallel) prints. A redirection of the
// consumer's input takes the place of the pipe, and the error output of
// the command in front may name paths too.
func stdinFeed(s walkStep, consumer string) *pathFeed {
	switch {
	case s.stdinRedirected:
		return &pathFeed{why: consumer + " reads its paths from a redirection in place of the pipe, " +
			"input belt cannot see"}
	case s.before != jointPipe:
		return &pathFeed{why: consumer + " reads its paths from input belt cannot see"}
	case len(s.feeder) == 0:
		return &pathFeed{why: "belt cannot tell which paths the command in front of " + consumer + " prints"}
	case s.feederStderr:
		return &pathFeed{why: "the error output of the command in front of " + consumer + " reaches " +
			consumer + " too, and belt cannot tell which paths it names"}
	}
	words := plainWords(s.feeder)
	roots, why := feederRoots(words)
	if why != "" {
		raw := make([]string, len(words))
		for i, w := range words {
			raw[i] = w.raw
		}
		why = "belt cannot tell which paths `" + strings.Join(raw, " ") + "` hands " + consumer + ": " + why
	}
	return &pathFeed{roots: roots, why: why}
}

func plainWords(placed []placedWord) []shWord {
	words := make([]shWord, len(placed))
	for i, w := range placed {
		words[i] = w.shWord
	}
	return words
}

// feederRoots reads a command whose output a delete takes as paths into
// the roots those paths lie under, or why belt cannot tell. A GIT_
// variable set in front of it may point git at another repository or
// change how it prints paths.
func feederRoots(words []shWord) ([]feedRoot, string) {
	rest := skipAssignments(words)
	for _, w := range words[:len(words)-len(rest)] {
		if name, _, _ := w.assignment(); strings.HasPrefix(name, "GIT_") {
			return nil, "belt does not read a feeder that sets " + name
		}
	}
	if len(rest) == 0 {
		return nil, "it runs no command"
	}
	switch runName(rest[0]) {
	case "git":
		return gitFeed(rest[1:])
	case "find", "gfind":
		return findFeed(rest[1:])
	case "grep", "ggrep", "egrep", "fgrep":
		return grepFeed(rest[1:])
	case "rg":
		return rgFeed(rest[1:])
	case "fd", "fdfind":
		return fdFeed(rest[1:])
	}
	return nil, "belt reads only the paths git ls-files, git grep -l, grep -l, rg -l, rg --files, find and fd print"
}

// gitFeed reads git ls-files and git grep -l, behind git options that
// leave the paths relative to the shell's directory. A -c that sets
// core.quotePath changes how git prints a path with odd bytes, so belt
// does not read it.
func gitFeed(args []shWord) ([]feedRoot, string) {
	for i := 0; i < len(args); i++ {
		v, ok := args[i].literal()
		switch {
		case !ok:
			return nil, "belt cannot read the git word `" + args[i].raw + "`"
		case v == "-c":
			i++
			if i < len(args) && strings.Contains(strings.ToLower(args[i].raw), "quotepath") {
				return nil, "belt does not read git -c " + args[i].raw + " there"
			}
		case v == "--no-pager" || v == "-P" || v == "--literal-pathspecs" || v == "--no-optional-locks":
		case strings.HasPrefix(v, "-"):
			return nil, "belt does not read git's " + v + " there"
		case v == "ls-files":
			return lsFilesFeed(args[i+1:])
		case v == "grep":
			return gitGrepFeed(args[i+1:])
		default:
			return nil, "git " + v + " prints no list of paths belt reads"
		}
	}
	return nil, "git runs no command"
}

// The git ls-files options file-discard reads: the long ones that list
// files git does not track, take a value, or change nothing it reads.
var (
	lsFilesUntracked = wordSet("others ignored killed directory no-empty-directory")
	lsFilesValues    = wordSet("exclude exclude-from exclude-per-directory with-tree")
	lsFilesFlags     = wordSet("cached modified deleted exclude-standard recurse-submodules error-unmatch " +
		"sparse deduplicate")
)

// lsFilesOpts is what lsFilesFeed reads from git ls-files' options:
// untracked for an option that lists files git does not track, ignored
// for -i, standard for --exclude-standard, and nul for -z.
type lsFilesOpts struct {
	untracked, ignored, standard, nul bool
}

// lsFilesFeed reads git ls-files: the files git tracks under each literal
// pathspec, or under the shell's directory. With -o, -i or -k it lists
// files git does not track, so every path but git's own data is at risk,
// an ignored one too unless --exclude-standard leaves them out, and a
// nested repository's folder as one path. An option that prints more than
// paths, or paths relative to another folder (--full-name), is not read.
func lsFilesFeed(args []shWord) ([]feedRoot, string) {
	var o lsFilesOpts
	endOpts := false
	var specs []shWord
	for i := 0; i < len(args); i++ {
		v, ok := args[i].literal()
		switch {
		case endOpts || !ok || !strings.HasPrefix(v, "-") || v == "-":
			specs = append(specs, args[i])
		case v == "--":
			endOpts = true
		default:
			if why := o.read(v, &i); why != "" {
				return nil, why
			}
		}
	}
	filter := &pathFilter{tracked: true, quoted: !o.nul}
	if o.untracked {
		filter = &pathFilter{
			skipsGit: true, skipsIgnored: o.standard && !o.ignored, nestedWhole: true, quoted: !o.nul,
		}
	}
	globbed := func(w shWord) bool {
		v, ok := w.literal()
		return !ok || strings.ContainsAny(v, "*?[") || strings.HasPrefix(v, ":")
	}
	if len(specs) == 0 || slices.ContainsFunc(specs, globbed) {
		specs = []shWord{literalWord(".")}
	}
	roots := make([]feedRoot, len(specs))
	for i, w := range specs {
		roots[i] = feedRoot{word: w, tree: true, filter: filter}
	}
	return roots, ""
}

// read reads one git ls-files option word at args[*i], and steps past its
// value; why says why belt does not read it.
func (o *lsFilesOpts) read(v string, i *int) (why string) {
	if strings.HasPrefix(v, "--") {
		name, _, inline := strings.Cut(v[2:], "=")
		switch {
		case lsFilesUntracked[name]:
			o.untracked = true
			o.ignored = o.ignored || name == "ignored"
		case lsFilesValues[name] && !inline:
			*i++
		case !lsFilesValues[name] && !lsFilesFlags[name]:
			return "belt does not read git ls-files " + v
		}
		o.standard = o.standard || name == "exclude-standard"
		return ""
	}
	for k := 1; k < len(v); k++ {
		switch c := v[k]; {
		case strings.IndexByte("oik", c) >= 0:
			o.untracked = true
			o.ignored = o.ignored || c == 'i'
		case c == 'z':
			o.nul = true
		case c == 'x' || c == 'X':
			if k == len(v)-1 {
				*i++
			}
			return ""
		case strings.IndexByte("cmd", c) < 0:
			return "belt does not read git ls-files -" + string(c)
		}
	}
	return ""
}

// The git grep options file-discard reads.
var (
	gitGrepFiles   = wordSet("files-with-matches name-only files-without-match")
	gitGrepRefused = wordSet("full-name open-files-in-pager")
	gitGrepValues  = wordSet("regexp file after-context before-context context max-count max-depth threads")
)

// gitGrepOpts is what gitGrepFeed reads from git grep's options: files
// for -l or -L, the options that search files git does not track, the
// ones that turn the standard ignores on or off, and nul for -z.
type gitGrepOpts struct {
	files, untracked, noIndex, standard, noStandard, nul bool
}

// gitGrepFeed reads git grep -l: files git tracks under the shell's
// directory, every file but git's own data with --untracked or --no-index.
// --untracked leaves ignored files out unless --no-exclude-standard says
// otherwise, and --no-index searches them unless --exclude-standard says
// otherwise. Its pathspecs only narrow that, so belt does not read them.
func gitGrepFeed(args []shWord) ([]feedRoot, string) {
	var o gitGrepOpts
	for i := 0; i < len(args); i++ {
		v, ok := args[i].literal()
		switch {
		case !ok || !strings.HasPrefix(v, "-") || v == "-":
		case v == "--":
			i = len(args)
		case strings.HasPrefix(v, "--"):
			if why := o.readLong(v, &i); why != "" {
				return nil, why
			}
		default:
			if why := o.readShorts(v, &i); why != "" {
				return nil, why
			}
		}
	}
	if !o.files {
		return nil, "git grep without -l prints lines, not paths"
	}
	filter := &pathFilter{tracked: true, quoted: !o.nul}
	if o.untracked || o.noIndex {
		skips := o.untracked && !o.noStandard || o.noIndex && o.standard
		filter = &pathFilter{skipsGit: true, skipsIgnored: skips, quoted: !o.nul}
	}
	return []feedRoot{{word: literalWord("."), tree: true, filter: filter}}, ""
}

// readLong reads one long git grep option at args[*i], and steps past its
// value.
func (o *gitGrepOpts) readLong(v string, i *int) string {
	name, _, inline := strings.Cut(v[2:], "=")
	switch {
	case gitGrepFiles[name]:
		o.files = true
	case name == "untracked":
		o.untracked = true
	case name == "no-index":
		o.noIndex = true
	case name == "exclude-standard":
		o.standard = true
	case name == "no-exclude-standard":
		o.noStandard = true
	case name == "null":
		o.nul = true
	case gitGrepRefused[name]:
		return "belt does not read git grep " + v
	case gitGrepValues[name] && !inline:
		*i++
	}
	return ""
}

// readShorts reads one cluster of short git grep options at args[*i]: a
// letter that takes a value takes the rest of the cluster or the next word.
func (o *gitGrepOpts) readShorts(v string, i *int) string {
	for k := 1; k < len(v); k++ {
		switch c := v[k]; {
		case c == 'l' || c == 'L':
			o.files = true
		case c == 'z':
			o.nul = true
		case c == 'O':
			return "belt does not read git grep -O"
		case strings.IndexByte("efABCm", c) >= 0:
			if k == len(v)-1 {
				*i++
			}
			return ""
		}
	}
	return ""
}

// The grep options file-discard reads, GNU's and BSD's: the long ones that
// take a value and those that do not, and the short ones that do.
var (
	grepValues = wordSet("regexp file max-count after-context before-context context directories devices " +
		"include exclude exclude-from exclude-dir label binary-files group-separator")
	grepFlags = wordSet("extended-regexp fixed-strings basic-regexp perl-regexp ignore-case no-ignore-case " +
		"word-regexp line-regexp null-data no-messages invert-match byte-offset line-buffered with-filename " +
		"no-filename only-matching quiet silent text null count initial-tab line-number no-group-separator " +
		"color colour files-with-matches files-without-match recursive dereference-recursive " +
		"unix-byte-offsets help version")
)

const grepValueShorts = "efmABCdD"

// grepOpts is what grepFeed reads from grep's options.
type grepOpts struct {
	recursive, follows, files, pattern, skipsGit bool
}

// grepFeed reads grep -l or -L: with -r each path operand (default .) is a
// tree, without it each names files. -R follows symlinks out of its
// folders, so belt does not read it; grep goes into .git unless
// --exclude-dir names it.
func grepFeed(args []shWord) ([]feedRoot, string) {
	var g grepOpts
	var positional []shWord
	endOpts := false
	for i := 0; i < len(args); i++ {
		v, ok := args[i].literal()
		switch {
		case endOpts || !ok || !strings.HasPrefix(v, "-") || v == "-":
			positional = append(positional, args[i])
		case v == "--":
			endOpts = true
		case strings.HasPrefix(v, "--"):
			if why := g.readLong(v, args, &i); why != "" {
				return nil, why
			}
		default:
			g.readShorts(v, args, &i)
		}
	}
	if !g.pattern && len(positional) > 0 {
		positional = positional[1:]
	}
	switch {
	case !g.files:
		return nil, "grep without -l prints lines, not paths"
	case g.follows:
		return nil, "grep -R follows symlinks out of the folders it searches"
	case !g.recursive && len(positional) == 0:
		return nil, "grep without -r or files reads its input"
	case len(positional) == 0:
		positional = []shWord{literalWord(".")}
	}
	filter := &pathFilter{skipsGit: g.skipsGit}
	roots := make([]feedRoot, len(positional))
	for i, w := range positional {
		roots[i] = feedRoot{word: w, tree: g.recursive, filter: filter}
	}
	return roots, ""
}

// readLong reads one long grep option at args[*i], and its value.
func (g *grepOpts) readLong(v string, args []shWord, i *int) string {
	name, val, inline := strings.Cut(v[2:], "=")
	if grepValues[name] && !inline && *i+1 < len(args) {
		*i++
		val, _ = args[*i].literal()
	}
	switch {
	case name == "files-with-matches" || name == "files-without-match":
		g.files = true
	case name == "recursive" || name == "directories" && val == "recurse":
		g.recursive = true
	case name == "dereference-recursive":
		g.follows = true
	case name == "exclude-dir":
		ok, _ := path.Match(val, ".git")
		g.skipsGit = g.skipsGit || ok
	case name == "regexp" || name == "file":
		g.pattern = true
	case !grepValues[name] && !grepFlags[name]:
		return "belt does not read grep " + v
	}
	return ""
}

// readShorts reads one cluster of short grep options at args[*i]: a letter
// that takes a value takes the rest of the cluster or the next word.
func (g *grepOpts) readShorts(v string, args []shWord, i *int) {
	for k := 1; k < len(v); k++ {
		c := v[k]
		if strings.IndexByte(grepValueShorts, c) >= 0 {
			val := v[k+1:]
			if val == "" && *i+1 < len(args) {
				*i++
				val, _ = args[*i].literal()
			}
			g.pattern = g.pattern || c == 'e' || c == 'f'
			g.recursive = g.recursive || c == 'd' && val == "recurse"
			return
		}
		switch c {
		case 'r':
			g.recursive = true
		case 'R', 'S':
			g.follows = true
		case 'l', 'L':
			g.files = true
		}
	}
}

// The rg options file-discard reads: the long ones that take a value and
// those that do not, and the short ones that take a value (rg's -r is
// --replace).
var (
	rgValues = wordSet("regexp file glob iglob type type-not type-add type-clear max-count max-depth " +
		"maxdepth max-filesize after-context before-context context threads max-columns encoding engine " +
		"replace sort sortr pre pre-glob ignore-file colors color context-separator " +
		"field-context-separator field-match-separator dfa-size-limit regex-size-limit hostname-bin " +
		"hyperlink-format generate")
	rgFlags = wordSet("files files-with-matches files-without-match follow no-follow hidden no-hidden " +
		"no-ignore no-ignore-vcs no-ignore-parent no-ignore-dot no-ignore-global no-ignore-exclude " +
		"no-ignore-files no-ignore-messages null null-data ignore-case smart-case case-sensitive " +
		"fixed-strings word-regexp line-regexp invert-match multiline multiline-dotall pcre2 " +
		"auto-hybrid-regex text binary no-messages quiet count count-matches no-filename with-filename " +
		"line-number no-line-number heading no-heading search-zip one-file-system no-config " +
		"unrestricted crlf trim vimgrep json stats debug trace no-unicode no-require-git " +
		"block-buffered line-buffered mmap no-mmap sort-files ignore-file-case-insensitive")
)

const rgValueShorts = "efgtTmABCjMdEr"

// rgOpts is what rgFeed reads from rg's options.
type rgOpts struct {
	listing, files, follows, hidden, pattern, skipsGit, noIgnore bool
	unrestricted                                                 int
}

// rgFeed reads rg -l, -L-free, and rg --files: each path operand (default
// .) is a tree. rg skips hidden folders, .git among them, unless --hidden,
// -. or -uu lets it in and no glob shuts .git out again, and it skips what
// git ignores unless a --no-ignore option or -u lets it in.
func rgFeed(args []shWord) ([]feedRoot, string) {
	var o rgOpts
	var positional []shWord
	endOpts := false
	for i := 0; i < len(args); i++ {
		v, ok := args[i].literal()
		switch {
		case endOpts || !ok || !strings.HasPrefix(v, "-") || v == "-":
			positional = append(positional, args[i])
		case v == "--":
			endOpts = true
		case strings.HasPrefix(v, "--"):
			if why := o.readLong(v, args, &i); why != "" {
				return nil, why
			}
		default:
			o.readShorts(v, args, &i)
		}
	}
	if !o.listing && !o.pattern && len(positional) > 0 {
		positional = positional[1:]
	}
	switch {
	case !o.listing && !o.files:
		return nil, "rg without -l or --files prints lines, not paths"
	case o.follows:
		return nil, "rg -L follows symlinks out of the folders it searches"
	case len(positional) == 0:
		positional = []shWord{literalWord(".")}
	}
	filter := &pathFilter{
		skipsGit:     o.skipsGit || !o.hidden && o.unrestricted < 2,
		skipsIgnored: !o.noIgnore && o.unrestricted == 0,
	}
	roots := make([]feedRoot, len(positional))
	for i, w := range positional {
		roots[i] = feedRoot{word: w, tree: true, filter: filter}
	}
	return roots, ""
}

// readLong reads one long rg option at args[*i], and its value.
func (o *rgOpts) readLong(v string, args []shWord, i *int) string {
	name, val, inline := strings.Cut(v[2:], "=")
	if rgValues[name] && !inline && *i+1 < len(args) {
		*i++
		val, _ = args[*i].literal()
	}
	switch {
	case name == "files":
		o.listing = true
	case name == "files-with-matches" || name == "files-without-match":
		o.files = true
	case name == "follow" || name == "no-follow":
		o.follows = name == "follow"
	case name == "hidden" || name == "no-hidden":
		o.hidden = name == "hidden"
	case name == "unrestricted":
		o.unrestricted++
	case strings.HasPrefix(name, "no-ignore") && name != "no-ignore-messages":
		o.noIgnore = true
	case name == "regexp" || name == "file":
		o.pattern = true
	case name == "glob" || name == "iglob":
		o.skipsGit = o.skipsGit || shutsOutGit(val)
	case !rgValues[name] && !rgFlags[name]:
		return "belt does not read rg " + v
	}
	return ""
}

// readShorts reads one cluster of short rg options at args[*i].
func (o *rgOpts) readShorts(v string, args []shWord, i *int) {
	for k := 1; k < len(v); k++ {
		c := v[k]
		if strings.IndexByte(rgValueShorts, c) >= 0 {
			val := v[k+1:]
			if val == "" && *i+1 < len(args) {
				*i++
				val, _ = args[*i].literal()
			}
			o.pattern = o.pattern || c == 'e' || c == 'f'
			o.skipsGit = o.skipsGit || c == 'g' && shutsOutGit(val)
			return
		}
		switch c {
		case 'l':
			o.files = true
		case 'L':
			o.follows = true
		case '.':
			o.hidden = true
		case 'u':
			o.unrestricted++
		}
	}
}

// shutsOutGit reports whether an rg glob leaves out the .git folder.
func shutsOutGit(glob string) bool {
	return slices.Contains([]string{"!.git", "!.git/", "!.git/*", "!.git/**"}, glob)
}

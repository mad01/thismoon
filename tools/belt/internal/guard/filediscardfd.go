package guard

import (
	"regexp"
	"strings"
)

// This file holds fd (fdfind on Debian), a consumer like find -exec: the
// command its -x or -X runs takes each entry fd matches, and a feeder like
// find, whose printed paths xargs or $( ) hand a command. The option list
// is fd 10.5.1's `fd --help`; any other option is one belt does not read.

// fdCall is an fd command read for file-discard: the roots it searches,
// the filter its tests make, the commands its -x and -X run, why belt
// cannot read it, and the option that makes it print more than its paths
// (fdOpts.printsOther).
type fdCall struct {
	roots  []shWord
	filter pathFilter
	execs  [][]shWord
	why    string
	prints string
}

// fdOpts is what readFd keeps of fd's options while it reads them. A
// later option overrides an earlier one (--no-hidden after -H).
type fdOpts struct {
	hidden, noIgnore, glob, fullPath, follows bool
	fold, sensitive, absolute                 bool
	exts                                      []string
	types                                     string
	anyType, anyExt                           bool
	searchPaths, positional                   []shWord
	other                                     string
}

// The fd options file-discard reads: the long ones that take no value and
// those that take one, the short ones of each by their long name, and the
// two that take a value only after = (--hyperlink=always).
var (
	fdFlags = wordSet("hidden no-hidden no-ignore ignore no-ignore-vcs ignore-vcs no-require-git " +
		"require-git no-ignore-parent ignore-parent unrestricted case-sensitive ignore-case glob regex " +
		"fixed-strings exact absolute-path relative-path list-details follow no-follow full-path print0 " +
		"prune quiet has-results show-errors one-file-system help version")
	fdValues = wordSet("and max-depth min-depth exact-depth exclude type extension size changed-within " +
		"change-newer-than newer changed-after changed-before change-older-than older owner format " +
		"batch-size ignore-file color ignore-contain threads max-results base-directory path-separator " +
		"search-path")
	fdOptional  = wordSet("hyperlink strip-cwd-prefix")
	fdShortFlag = map[byte]string{
		'H': "hidden", 'I': "no-ignore", 'u': "unrestricted", 's': "case-sensitive", 'i': "ignore-case",
		'g': "glob", 'F': "fixed-strings", 'a': "absolute-path", 'l': "list-details", 'L': "follow",
		'p': "full-path", '0': "print0", '1': "max-results", 'q': "quiet", 'h': "help", 'V': "version",
	}
	fdShortValue = map[byte]string{
		'd': "max-depth", 'E': "exclude", 't': "type", 'e': "extension", 'S': "size", 'o': "owner",
		'c': "color", 'j': "threads", 'C': "base-directory",
	}
)

// fdTypes are fd's --type values as find's -type letters: an executable
// is a file, and empty is an empty file or folder.
var fdTypes = map[string]string{
	"f": "f", "file": "f", "d": "d", "dir": "d", "directory": "d", "l": "l", "symlink": "l",
	"s": "s", "socket": "s", "p": "p", "pipe": "p", "b": "b", "block-device": "b",
	"c": "c", "char-device": "c", "x": "f", "executable": "f", "e": "fd", "empty": "fd",
}

// fdMarks are the texts fd fills in a command: {}, {/}, {//}, {.} and
// {/.}, and the {{ and }} that stand for one brace.
var fdMarks = regexp.MustCompile(`\{\{|\}\}|\{(?:/|//|\.|/\.)?\}`)

// fdRuns reports whether an fd's words hold an -x or -X, alone, long or at
// the end of a short cluster, so only such an fd is read.
func fdRuns(args []shWord) bool {
	for _, w := range args {
		v, ok := w.literal()
		switch {
		case !ok:
		case v == "--exec" || v == "--exec-batch" || strings.HasPrefix(v, "--exec="),
			strings.HasPrefix(v, "--exec-batch="):
			return true
		case len(v) > 1 && v[0] == '-' && v[1] != '-' && strings.ContainsAny(v, "xX"):
			return true
		}
	}
	return false
}

// readFd reads an fd command: its options, the pattern and the roots after
// it, and each command an -x or -X runs, which takes every word up to a
// `;`.
func readFd(args []shWord) fdCall {
	var o fdOpts
	var f fdCall
	endOpts := false
	for i := 0; i < len(args) && f.why == ""; i++ {
		v, ok := args[i].literal()
		switch {
		case endOpts || !ok || len(v) < 2 || v[0] != '-':
			o.positional = append(o.positional, args[i])
		case v == "--":
			endOpts = true
		case strings.HasPrefix(v, "--"):
			f.why = o.readLong(v[2:], args, &i, &f)
		default:
			f.why = o.readShorts(v, args, &i, &f)
		}
	}
	if f.why == "" && o.follows {
		f.why = "fd -L follows symlinks out of the folders it searches"
	}
	f.roots, f.filter = o.rootsAndFilter()
	f.prints = o.printsOther()
	return f
}

// printsOther names the first option that makes fd print more than the
// paths it matches, "" for none: --format, -l, help and version text, a
// --color or --hyperlink of always, which wraps each path in escapes, and
// -a, whose paths do not start with the roots as written.
func (o *fdOpts) printsOther() string {
	if o.other == "" && o.absolute {
		return "-a"
	}
	return o.other
}

// prints keeps the first option that makes fd print more than its paths.
func (o *fdOpts) prints(option string) {
	if o.other == "" {
		o.other = option
	}
}

// readLong reads one long fd option at args[*i], and its value.
func (o *fdOpts) readLong(spec string, args []shWord, i *int, f *fdCall) string {
	name, val, inline := strings.Cut(spec, "=")
	switch {
	case name == "exec" || name == "exec-batch":
		var first []shWord
		if inline {
			first = []shWord{literalWord(val)}
		}
		f.execs = append(f.execs, execWords(first, args, i))
	case fdFlags[name] && !inline:
		o.flag(name)
	case name == "hyperlink" && inline && val != "never" && val != "auto":
		o.prints("--" + spec)
	case fdValues[name]:
		w := literalWord(val)
		if !inline {
			if *i+1 >= len(args) {
				return "fd's --" + name + " takes a value the command does not give"
			}
			*i++
			w = args[*i]
		}
		return o.value(name, w)
	case !fdOptional[name]:
		return "belt does not read fd's --" + spec
	}
	return ""
}

// readShorts reads one cluster of short fd options at args[*i]. A value
// option takes the rest of the cluster or the next word, and -x or -X
// takes the rest of the cluster as its command's first word.
func (o *fdOpts) readShorts(v string, args []shWord, i *int, f *fdCall) string {
	for k := 1; k < len(v); k++ {
		c := v[k]
		if name, ok := fdShortFlag[c]; ok {
			o.flag(name)
			continue
		}
		rest := v[k+1:]
		switch name, ok := fdShortValue[c]; {
		case ok && rest != "":
			return o.value(name, literalWord(rest))
		case ok && *i+1 < len(args):
			*i++
			return o.value(name, args[*i])
		case ok:
			return "fd's -" + string(c) + " takes a value the command does not give"
		case c == 'x' || c == 'X':
			var first []shWord
			if rest != "" {
				first = []shWord{literalWord(rest)}
			}
			f.execs = append(f.execs, execWords(first, args, i))
			return ""
		}
		return "belt does not read fd's -" + string(c)
	}
	return ""
}

// execWords returns the command of an -x or -X at args[*i]: first, then
// every word after it up to a `;`, where *i is left.
func execWords(first, args []shWord, i *int) []shWord {
	words := first
	for *i++; *i < len(args); *i++ {
		if v, ok := args[*i].literal(); ok && v == ";" {
			break
		}
		words = append(words, args[*i])
	}
	return words
}

// flag takes one fd option that takes no value, by its long name.
func (o *fdOpts) flag(name string) {
	switch name {
	case "hidden", "no-hidden":
		o.hidden = name == "hidden"
	case "no-ignore", "ignore", "no-ignore-vcs", "ignore-vcs", "no-ignore-parent", "ignore-parent":
		o.noIgnore = strings.HasPrefix(name, "no-")
	case "unrestricted":
		o.hidden, o.noIgnore = true, true
	case "case-sensitive", "ignore-case":
		o.sensitive, o.fold = name == "case-sensitive", name == "ignore-case"
	case "glob", "regex", "fixed-strings", "exact":
		o.glob = name == "glob"
	case "full-path":
		o.fullPath = true
	case "follow", "no-follow":
		o.follows = name == "follow"
	case "absolute-path", "relative-path":
		o.absolute = name == "absolute-path"
	case "list-details", "help", "version":
		o.prints("--" + name)
	}
}

// value takes one fd option's value, by the option's long name; why says
// why belt does not read it. --base-directory runs fd and its commands in
// another folder, and --path-separator changes the paths it hands them.
func (o *fdOpts) value(name string, w shWord) string {
	v, lit := w.literal()
	switch name {
	case "extension":
		o.exts = append(o.exts, strings.TrimPrefix(v, "."))
		o.anyExt = o.anyExt || !lit
	case "type":
		letters, ok := fdTypes[v]
		o.types += letters
		o.anyType = o.anyType || !lit || !ok
	case "search-path":
		o.searchPaths = append(o.searchPaths, w)
	case "format":
		o.prints("--format")
	case "color":
		if !lit || v != "never" && v != "auto" {
			o.prints("--color " + w.raw)
		}
	case "base-directory", "path-separator":
		return "belt does not read fd's --" + name + " " + w.raw
	}
	return ""
}

// rootsAndFilter returns the folders fd searches, the paths after its
// pattern and the --search-path ones (default .), and the filter its
// tests make. fd skips hidden entries, .git among them, unless -H or -u,
// and what git ignores unless a --no-ignore option or -u. One -e names
// the files; several match any of them, which no filter holds. A --glob
// pattern names them too, matched without case unless it holds an upper
// case letter or -s, but a regex does not.
func (o *fdOpts) rootsAndFilter() ([]shWord, pathFilter) {
	f := pathFilter{skipsGit: !o.hidden, skipsIgnored: !o.noIgnore}
	if !o.anyType {
		f.types = o.types
	}
	if len(o.exts) == 1 && !o.anyExt && !strings.ContainsAny(o.exts[0], `*?[\`) {
		f.names = append(f.names, findName{pattern: "*." + o.exts[0], fold: true})
	}
	roots := o.searchPaths
	if len(o.positional) > 0 {
		roots = append(roots, o.positional[1:]...)
		if n, ok := o.globName(o.positional[0]); ok {
			f.names = append(f.names, n)
		}
	}
	if len(roots) == 0 {
		roots = []shWord{literalWord(".")}
	}
	return roots, f
}

// globName reads fd's pattern as a name test when it is a --glob pattern
// matched against the name alone that path.Match reads alike (findPattern):
// a brace list, which path.Match does not read, is left out.
func (o *fdOpts) globName(w shWord) (findName, bool) {
	p, ok := w.literal()
	if !ok || !o.glob || o.fullPath || strings.ContainsAny(p, "{}") {
		return findName{}, false
	}
	fold := !o.sensitive && (o.fold || strings.ToLower(p) == p)
	return findPattern([]shWord{w}, 0, fold)
}

// fd judges an fd that runs a command (fdRuns): each -x or -X command
// takes the entries fd matches under its roots (discardJudge.consumer), at
// {} or after the command's own words.
func (j *discardJudge) fd(args []shWord, s walkStep) *fileLoss {
	if !fdRuns(args) {
		return nil
	}
	f := readFd(args)
	call := &lossCall{toss: "have fd hand what it matches to the trash: `-X " + TossBin + " --safe-mode -r`"}
	if f.why != "" {
		return &fileLoss{call: call, why: f.why}
	}
	for _, command := range f.execs {
		r := consumerRun{
			who: "fd", command: command, mark: "{}", marks: fdMarks, appends: true,
			feeds: []*pathFeed{{roots: f.feedRoots()}}, call: call,
		}
		if loss := j.consumer(r, s); loss != nil {
			return loss
		}
	}
	return nil
}

// feedRoots returns fd's roots as the trees it hands paths from, each with
// its own copy of the filter.
func (f fdCall) feedRoots() []feedRoot {
	roots := make([]feedRoot, len(f.roots))
	for i, w := range f.roots {
		filter := f.filter
		roots[i] = feedRoot{word: w, tree: true, filter: &filter}
	}
	return roots
}

// fdFeed reads the paths an fd prints as a feeder (`fd -e pyc | xargs rm`):
// its roots and filter, read as for -x (readFd), so an option belt does
// not read denies there too. An fd that runs a command prints what that
// command prints, and printsOther names the options that print more than
// paths, so belt reads neither.
func fdFeed(args []shWord) ([]feedRoot, string) {
	f := readFd(args)
	switch {
	case f.why != "":
		return nil, f.why
	case len(f.execs) > 0:
		return nil, "fd -x prints what its command prints"
	case f.prints != "":
		return nil, "fd " + f.prints + " prints more than paths"
	}
	return f.feedRoots(), ""
}

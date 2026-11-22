package guard

import (
	"cmp"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// This file holds file-discard's reading of a make command (readMake): its
// options, its makefile, and the recipe lines each target it runs and
// each prerequisite of those runs, expanded the way make expands them.

// makeArgs is what a make command asks for: the folder it runs in (-C),
// its makefiles (-f), the folders it looks in for an include (-I), its
// targets and the variables it sets. dry marks -n, -q and -t, which run
// only the lines marked + or that run $(MAKE). unread says why belt cannot
// tell what it runs, "" when it can. own says why the run holds makefile
// text no file holds: a makefile on standard input or an --eval text,
// which the trust rule never trusts. envFirst marks -e, under which the
// environment overrides the makefile.
type makeArgs struct {
	dir      string
	files    []string
	includes []string
	targets  []string
	vars     map[string]string
	dry      bool
	envFirst bool
	unread   string
	own      string
}

// makeShortValues are the short options that take a value, and
// makeShortCounts the ones that may take a number.
const (
	makeShortValues = "CfIoW"
	makeShortCounts = "jlO"
)

// makeLongValues are the long options that take a value, in the same word
// or the next; makeLongDry the ones that run no recipe.
var (
	makeLongValues = wordSet("directory file makefile include-dir old-file assume-old what-if new-file " +
		"assume-new eval")
	makeLongDry    = wordSet("dry-run just-print recon question touch")
	makeLongCounts = wordSet("jobs load-average max-load")
	makeInfo       = wordSet("-v --version -h --help")
)

// readMake reads what a make command run from dir runs: the recipe of each
// target it names, or of the default goal, with each prerequisite's first,
// and the $(shell) commands make runs as it reads the makefile. A command
// with no makefile to read runs nothing. One with makefile text no file
// holds (makeArgs.own) is one belt cannot read, and no file vouches for it.
// The files a MAKEFILES variable the command sets names are read first.
func readMake(args []shWord, env map[string]string, dir string, v *buildView) (buildRun, bool) {
	a, ok := readMakeArgs(args, dir)
	if !ok {
		return buildRun{}, false
	}
	a.environment(env)
	files := a.makefiles(v)
	m := newMakeFile(a.dir, a.vars, env, v)
	m.includeDirs = a.includeDirs()
	first := m.readExtra(strings.Fields(env["MAKEFILES"]))
	for _, f := range files {
		m.read(f, true)
	}
	switch {
	case a.own != "":
		return buildRun{files: m.files, text: m.text.String(), unread: a.own}, true
	case len(m.files) == first:
		return buildRun{}, false
	}
	main := m.files[first]
	lang, shellUnread := m.recipeLang()
	run := buildRun{file: main, files: m.files, text: m.text.String(), unread: cmp.Or(a.unread, shellUnread)}
	w := &makeWalk{m: m, dry: a.dry, seen: map[string]bool{}, shown: shownPath(main, dir), lang: lang}
	for _, sh := range m.shells {
		w.bodies = append(w.bodies, buildBody{
			what: "reading " + w.shown + ", make runs `" + shortLine(sh) + "`", text: sh, dir: m.dir,
			file: main,
		})
	}
	targets := a.targets
	if len(targets) == 0 {
		targets = []string{m.goal()}
	}
	for _, t := range targets {
		if t != "" {
			w.target(t, nil)
		}
	}
	run.bodies = w.bodies
	return run, true
}

// environment makes the variables the command sets in front of make count
// as the command line's under -e (makeArgs.envFirst) or a MAKEFLAGS that
// holds it (makeflagsOverride), since the makefile no longer overrides
// them. A MAKEFILES value belt cannot read leaves the run unread.
func (a *makeArgs) environment(env map[string]string) {
	if strings.Contains(env["MAKEFILES"], "${__env_") {
		a.unread = "belt cannot tell which makefiles `MAKEFILES` names"
	}
	if !a.envFirst && !makeflagsOverride(env["MAKEFLAGS"]) {
		return
	}
	for name, v := range env {
		if _, set := a.vars[name]; !set && name != "MAKEFLAGS" && name != "MAKEFILES" {
			a.vars[name] = v
		}
	}
}

// makeflagsOverride reports whether a MAKEFLAGS value asks for -e: the
// word --environment-overrides, or an e among the letters of a word of
// short options (`-e`, `-ke`, or make's own form, a first word of letters
// with no dash), up to a letter that takes a value. A value belt cannot
// read may hold it.
func makeflagsOverride(flags string) bool {
	if strings.Contains(flags, "${__env_") {
		return true
	}
	for i, w := range strings.Fields(flags) {
		letters, short := strings.CutPrefix(w, "-")
		switch {
		case w == "--environment-overrides":
			return true
		case strings.HasPrefix(w, "--") || strings.Contains(w, "=") || !short && i > 0:
			continue
		}
		for _, c := range letters {
			if c == 'e' {
				return true
			}
			if strings.ContainsRune(makeShortValues, c) {
				break
			}
		}
	}
	return false
}

// readMakeArgs reads make's words. ok is false for --version and --help,
// which run nothing. A word belt cannot read leaves the run unread.
func readMakeArgs(args []shWord, dir string) (makeArgs, bool) {
	a := makeArgs{dir: dir, vars: map[string]string{}}
	opts := true
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		switch {
		case !lit:
			a.unread = "belt cannot tell what `" + args[i].raw + "` asks make to run"
		case opts && v == "--":
			opts = false
		case opts && makeInfo[v]:
			return a, false
		case opts && strings.HasPrefix(v, "--"):
			i = a.long(v[2:], args, i)
		case opts && strings.HasPrefix(v, "-") && len(v) > 1:
			i = a.short(v[1:], args, i)
		case isMakeAssignment(v):
			name, value := splitMakeAssignment(v)
			a.vars[name] = value
		default:
			a.targets = append(a.targets, v)
		}
	}
	return a, true
}

// long reads one long option at args[i], and returns the index of the last
// word it took. A count option takes the next word when it is a number.
func (a *makeArgs) long(opt string, args []shWord, i int) int {
	name, value, hasValue := strings.Cut(opt, "=")
	if makeLongDry[name] {
		a.dry = true
	}
	a.envFirst = a.envFirst || name == "environment-overrides"
	if makeLongCounts[name] && !hasValue && i+1 < len(args) && isCount(args[i+1]) {
		return i + 1
	}
	if !makeLongValues[name] {
		return i
	}
	if !hasValue {
		if i+1 >= len(args) {
			return i
		}
		i++
		value = a.word(args[i])
	}
	a.option(name, value)
	return i
}

// short reads one word of short options at args[i], and returns the index
// of the last word it took.
func (a *makeArgs) short(letters string, args []shWord, i int) int {
	for k := 0; k < len(letters); k++ {
		c := letters[k]
		switch {
		case strings.IndexByte(makeShortValues, c) >= 0:
			value := letters[k+1:]
			if value == "" && i+1 < len(args) {
				i++
				value = a.word(args[i])
			}
			a.option(string(c), value)
			return i
		case strings.IndexByte(makeShortCounts, c) >= 0:
			if k+1 == len(letters) && i+1 < len(args) && isCount(args[i+1]) {
				i++
			}
			return i
		case c == 'n' || c == 'q' || c == 't':
			a.dry = true
		case c == 'e':
			a.envFirst = true
		}
	}
	return i
}

// isCount reports whether a word is a number, as -j and -l may take.
func isCount(w shWord) bool {
	v, ok := w.literal()
	return ok && v != "" && strings.Trim(v, "0123456789.") == ""
}

// word is an option's value as a literal; one belt cannot read leaves the
// run unread.
func (a *makeArgs) word(w shWord) string {
	v, ok := w.literal()
	if !ok {
		a.unread = "belt cannot tell what `" + w.raw + "` asks make to run"
	}
	return v
}

// option applies an option's value: -C moves make's folder, -f adds a
// makefile, -I a folder to look in for an include, --eval adds makefile
// text belt does not read.
func (a *makeArgs) option(name, value string) {
	switch name {
	case "C", "directory":
		if value != "" {
			a.dir = joinDir(a.dir, value)
		}
	case "f", "file", "makefile":
		a.files = append(a.files, value)
	case "I", "include-dir":
		if value != "" {
			a.includes = append(a.includes, value)
		}
	case "eval":
		a.own = "belt does not read the makefile text `--eval` adds"
	}
}

// makeDefaultIncludes are the folders GNU make looks in for an include
// after the -I ones.
var makeDefaultIncludes = []string{"/usr/gnu/include", "/usr/local/include", "/usr/include"}

// includeDirs returns the folders make looks in for an include it does not
// find in its own folder: each -I folder, from make's folder, since make
// moves there (-C) before it reads a makefile, then makeDefaultIncludes.
func (a *makeArgs) includeDirs() []string {
	out := make([]string, 0, len(a.includes)+len(makeDefaultIncludes))
	for _, d := range a.includes {
		out = append(out, joinDir(a.dir, d))
	}
	return append(out, makeDefaultIncludes...)
}

// makefiles returns the makefiles make reads: the -f files from its
// folder, else the first of GNUmakefile, makefile and Makefile there that
// a step before it writes (discardJudge.writes) or that exists on disk. A
// makefile on standard input is text no file holds (makeArgs.own).
func (a *makeArgs) makefiles(v *buildView) []string {
	if len(a.files) > 0 {
		out := make([]string, 0, len(a.files))
		for _, f := range a.files {
			if f == "-" || f == "/dev/stdin" || f == "/dev/fd/0" {
				a.own = "belt cannot read a makefile make reads from standard input"
				continue
			}
			out = append(out, joinDir(a.dir, f))
		}
		return out
	}
	entries, _ := os.ReadDir(a.dir)
	for _, name := range []string{"GNUmakefile", "makefile", "Makefile"} {
		if p := filepath.Join(a.dir, name); v.j.writes(p) {
			return []string{p}
		}
		for _, e := range entries {
			if strings.EqualFold(e.Name(), name) && e.Type()&^fs.ModeSymlink == 0 {
				return []string{filepath.Join(a.dir, e.Name())}
			}
		}
	}
	return nil
}

// isMakeAssignment reports whether a make word sets a variable
// (`OUT=build`, `OUT:=build`).
func isMakeAssignment(v string) bool {
	op, at := findMakeOp(v)
	return at > 0 && op != ":" && op != "::"
}

// splitMakeAssignment splits a command-line assignment into its name and
// its value, which make takes as it is.
func splitMakeAssignment(v string) (string, string) {
	op, at := findMakeOp(v)
	return strings.TrimSpace(v[:at]), v[at+len(op):]
}

// makeWalk gathers the recipe bodies of the targets make runs, each
// target once, its prerequisites first. dry marks a run that runs only
// the lines marked + or that run $(MAKE).
type makeWalk struct {
	m      *makeFile
	dry    bool
	seen   map[string]bool
	bodies []buildBody
	shown  string
	lang   string
}

// target adds the bodies one target runs, with its prerequisites' first,
// under the target-specific variables of it and the targets that need it
// (overlay). A target belt finds no rule for runs nothing, unless belt
// could not read all of the makefile, which may hold one.
func (w *makeWalk) target(t string, overlay map[string]*makeVar) {
	if w.seen[t] || w.over() {
		return
	}
	w.seen[t] = true
	rules, stem := w.m.rulesFor(t)
	if len(rules) == 0 && w.m.partial != "" {
		w.bodies = append(w.bodies, buildBody{
			what: "the make target `" + t + "` in " + w.shown, file: w.m.files[0],
			unread: w.m.partial + ", so it cannot tell what that target runs",
		})
		return
	}
	vars := make(map[string]*makeVar, len(overlay))
	for k, v := range overlay {
		vars[k] = v
	}
	for _, r := range w.m.patternVars {
		if _, ok := matchPattern(r.targets[0], t); ok {
			maps.Copy(vars, r.vars)
		}
	}
	var prereqs []string
	for _, r := range rules {
		maps.Copy(vars, r.vars)
		for _, pre := range r.prereqs {
			prereqs = append(prereqs, w.expandPrereq(strings.ReplaceAll(pre, "%", stem), vars)...)
		}
	}
	for _, pre := range prereqs {
		w.target(pre, vars)
	}
	auto := makeAuto(t, stem, prereqs)
	for _, r := range rules {
		w.recipe(t, r, auto, vars)
	}
}

// expandPrereq returns the targets a prerequisite names: itself, or after
// .SECONDEXPANSION each word of each value make expands it to a second
// time, under the target's variables.
func (w *makeWalk) expandPrereq(pre string, vars map[string]*makeVar) []string {
	if !w.m.secondExpansion || !strings.Contains(pre, "$") {
		return []string{pre}
	}
	var out []string
	for _, v := range (&makeExpand{m: w.m, overlay: vars}).expand(pre) {
		out = append(out, strings.Fields(v)...)
	}
	return out
}

// over reports whether the walk holds maxBuildBodies bodies, and adds an
// unread one the first time it does.
func (w *makeWalk) over() bool {
	switch n := len(w.bodies); {
	case n < maxBuildBodies:
		return false
	case n == maxBuildBodies:
		w.bodies = append(w.bodies, buildBody{file: w.m.files[0], unread: overBodies(w.m.files[0]).why})
	}
	return true
}

// makeAuto are a target's automatic variables.
func makeAuto(t, stem string, prereqs []string) map[string]string {
	first := ""
	if len(prereqs) > 0 {
		first = prereqs[0]
	}
	return map[string]string{
		"@": t, "*": stem, "<": first, "+": strings.Join(prereqs, " "),
		"^": strings.Join(dedupe(prereqs), " "), "?": makePlaceholder("?"), "|": "",
	}
}

// dedupe returns words without repeats, in order.
func dedupe(words []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, w := range words {
		if !seen[w] {
			seen[w] = true
			out = append(out, w)
		}
	}
	return out
}

// recipe adds the bodies of one rule's recipe: each line, as make hands
// it to a shell of its own, and under .ONESHELL the whole recipe as one
// script too, since make before 3.82 runs each line apart.
func (w *makeWalk) recipe(t string, r *makeRule, auto map[string]string, vars map[string]*makeVar) {
	var script []string
	for _, l := range r.recipe {
		text, runs := w.lineText(l.text)
		if !runs {
			continue
		}
		script = append(script, text)
		w.line(t, r, makeLine{text: text, line: l.line}, auto, vars)
	}
	if w.m.oneShell && len(script) > 1 {
		w.line(t, r, makeLine{text: strings.Join(script, "\n"), line: r.recipe[0].line}, auto, vars)
	}
}

// lineText drops a recipe line's @, - and + marks, and reports whether the
// line runs: under -n, -q and -t only a line marked + or that runs
// $(MAKE) does.
func (w *makeWalk) lineText(l string) (string, bool) {
	text := strings.TrimLeft(l, " \t@-+")
	plus := strings.Contains(l[:len(l)-len(text)], "+")
	if w.dry && !plus && !strings.Contains(text, "$(MAKE)") && !strings.Contains(text, "${MAKE}") {
		return text, false
	}
	return text, true
}

// line adds the bodies of one recipe line: the $(shell) commands its
// expansion runs, then each value it expands to. A line belt cannot
// expand in full, or that expands a function belt cannot read whose text
// names a delete, is unread.
func (w *makeWalk) line(t string, r *makeRule, l makeLine, auto map[string]string, vars map[string]*makeVar) {
	e := &makeExpand{m: w.m, overlay: vars, auto: auto}
	values := e.expand(l.text)
	shown := shownPath(r.file, w.m.dir)
	what := bodyWhat("make target", t, shown+":"+strconv.Itoa(l.line), l.text)
	for _, sh := range e.shells {
		w.bodies = append(w.bodies, buildBody{what: what, text: sh, dir: w.m.dir, file: r.file})
	}
	switch {
	case e.over:
		w.bodies = append(w.bodies, buildBody{what: what, file: r.file,
			unread: "belt cannot expand every value of that line"})
		return
	case e.deletes != "":
		w.bodies = append(w.bodies, buildBody{what: what, file: r.file,
			unread: "belt cannot read what `" + e.deletes + "` expands to, and it names a delete"})
		return
	}
	for _, v := range values {
		w.bodies = append(w.bodies, buildBody{what: what, text: v, dir: w.m.dir, file: r.file, lang: w.lang})
	}
}

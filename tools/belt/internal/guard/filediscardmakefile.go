package guard

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// This file reads a makefile for file-discard (makeFile): its rules, its
// variables and the files it includes, the way GNU make reads them, as far
// as a recipe's deletes need. What it cannot read leaves the makefile
// partial, so a target it does not find there is unknown.

// makeFile is what belt read of a makefile and the files it includes: the
// explicit rules by target and the pattern rules, the targets in the order
// met (for the default goal), the variables, the ones the command line or
// an override directive set (cmdline, which only an override changes), and
// the shell commands make runs as it reads the file. partial says what
// belt could not read, "" when it read all of it. patternVars are the
// pattern-specific variables (`%-out: OUT = src`), goalFrom the first of
// order a default goal may come from (past the files MAKEFILES names),
// prefix the first byte of a recipe line, a tab unless .RECIPEPREFIX sets
// another, and includeDirs the folders make looks in for an include.
type makeFile struct {
	view            *buildView
	dir             string
	rules           map[string][]*makeRule
	patterns        []*makeRule
	patternVars     []*makeRule
	order           []string
	goalFrom        int
	vars            map[string]*makeVar
	cmdline         map[string]bool
	oneShell        bool
	secondExpansion bool
	prefix          string
	shells          []string
	partial         string
	files           []string
	includeDirs     []string
	text            strings.Builder
}

// makeRule is one rule: its targets, its prerequisites, its recipe lines,
// the file it is written in and its target-specific variables.
type makeRule struct {
	targets []string
	prereqs []string
	recipe  []makeLine
	file    string
	vars    map[string]*makeVar
}

// makeLine is one recipe line as written, with its line number.
type makeLine struct {
	text string
	line int
}

// makeVar is a variable's values: one, or one per branch of the
// conditionals that set it, since belt does not tell which branch runs.
type makeVar struct{ values []makeValue }

// makeValue is one value of a variable. simple marks one make expanded
// where it was set (`:=`), which is taken as it is.
type makeValue struct {
	text   string
	simple bool
}

// maxMakeFiles bounds the makefiles one read takes in, includes among them.
const maxMakeFiles = 16

// makeParser reads makefile lines into a makeFile: the rule whose recipe
// the lines that start with a tab extend, an open define, and how deep the
// conditionals around the line run.
type makeParser struct {
	m      *makeFile
	file   string
	rule   *makeRule
	define *makeDefine
	cond   int
}

// makeDefine is a define being read: its variable, its operator and its
// lines so far, and how deep the defines inside it run.
type makeDefine struct {
	name, op string
	override bool
	lines    []string
	depth    int
}

// newMakeFile starts the makefile read in dir, with the variables the
// command line sets (which the file does not override) and the ones the
// environment sets (which only fill in what the file leaves unset). It
// reads its files through v.
func newMakeFile(dir string, cmdline, env map[string]string, v *buildView) *makeFile {
	m := &makeFile{
		view: v, dir: dir, prefix: "\t", rules: map[string][]*makeRule{}, vars: map[string]*makeVar{}, cmdline: map[string]bool{},
	}
	for name, v := range env {
		m.vars[name] = &makeVar{values: []makeValue{{text: v, simple: true}}}
	}
	for name, v := range cmdline {
		m.vars[name] = &makeVar{values: []makeValue{{text: v, simple: true}}}
		m.cmdline[name] = true
	}
	return m
}

// read reads one makefile into m. A file belt cannot read leaves m
// partial when must is set, as for an include make needs.
func (m *makeFile) read(file string, must bool) {
	if len(m.files) >= maxMakeFiles {
		m.partial = "belt reads at most " + strconv.Itoa(maxMakeFiles) + " makefiles"
		return
	}
	text, ok := m.view.read(file)
	if !ok {
		if must {
			m.partial = "belt cannot read `" + file + "`, which the makefile includes"
		}
		return
	}
	m.files = append(m.files, file)
	m.text.WriteString(text + "\n")
	p := &makeParser{m: m, file: file}
	for _, l := range makeLines(text) {
		p.line(l)
	}
}

// readExtra reads the makefiles a MAKEFILES variable names, from make's
// folder or its include folders (findInclude): make reads them before the others, skips one it cannot find and
// takes no default goal from them (goalFrom). It returns how many it read.
func (m *makeFile) readExtra(files []string) int {
	for _, f := range files {
		m.read(m.findInclude(f), false)
	}
	m.goalFrom = len(m.order)
	return len(m.files)
}

// findInclude returns the file make reads for an include name: the name
// from make's folder, else from the first of its include folders
// (includeDirs) where the tool finds it (buildView.exists). An absolute
// name, and one found nowhere, is read from make's folder.
func (m *makeFile) findInclude(name string) string {
	p := joinDir(m.dir, name)
	if filepath.IsAbs(name) || m.view.exists(p) {
		return p
	}
	for _, d := range m.includeDirs {
		if q := filepath.Join(d, name); m.view.exists(q) {
			return q
		}
	}
	return p
}

// posixShells are the SHELL names whose recipes belt walks as shell text.
var posixShells = wordSet("sh bash zsh dash ksh ash mksh")

// recipeLang returns the language make runs recipes in, by the values its
// SHELL may hold: a script language belt reads (langDeleteCalls) when one
// names it, else "" when one is a POSIX shell or a value belt cannot
// expand, which is most likely one. For a SHELL that holds neither it says
// why belt cannot read the recipes.
func (m *makeFile) recipeLang() (lang, unread string) {
	posix, other := false, ""
	for _, v := range (&makeExpand{m: m}).expand("$(SHELL)") {
		shell := strings.Fields(v)
		if len(shell) > 1 && filepath.Base(shell[0]) == "env" {
			shell = shell[1:]
		}
		if len(shell) == 0 || strings.Contains(shell[0], "${__") {
			posix = true
			continue
		}
		name := filepath.Base(shell[0])
		_, read := langDeleteCalls[interpreterLang(name)]
		switch {
		case posixShells[name]:
			posix = true
		case read:
			lang = interpreterLang(name)
		default:
			other = strings.TrimSpace(v)
		}
	}
	if lang != "" || posix || other == "" {
		return lang, ""
	}
	return "", "belt does not read recipes make runs with the SHELL `" + other + "`"
}

// makeLines splits makefile text into logical lines with their numbers:
// a backslash at the end joins the next line. A recipe line keeps the
// backslash and newline for the shell, dropping the next line's tab, and
// any other line joins with one space.
func makeLines(text string) []makeLine {
	raw := strings.Split(text, "\n")
	var out []makeLine
	for i := 0; i < len(raw); i++ {
		l, n := raw[i], i+1
		recipe := strings.HasPrefix(l, "\t")
		for continues(l) && i+1 < len(raw) {
			i++
			next := raw[i]
			if recipe {
				l += "\n" + strings.TrimPrefix(next, "\t")
				continue
			}
			l = strings.TrimRight(l[:len(l)-1], " \t") + " " + strings.TrimLeft(next, " \t")
		}
		out = append(out, makeLine{text: l, line: n})
	}
	return out
}

// continues reports whether a line ends in an odd run of backslashes.
func continues(l string) bool {
	n := len(l) - len(strings.TrimRight(l, `\`))
	return n%2 == 1
}

// line reads one logical line: a define's body, a recipe line of the rule
// above it, a directive, an assignment or a rule.
func (p *makeParser) line(l makeLine) {
	if p.define != nil {
		p.defineLine(l.text)
		return
	}
	if strings.HasPrefix(l.text, p.m.prefix) && p.rule != nil {
		p.rule.recipe = append(p.rule.recipe, makeLine{text: l.text[len(p.m.prefix):], line: l.line})
		return
	}
	text := strings.TrimSpace(stripMakeComment(l.text))
	if text == "" || p.directive(text) {
		return
	}
	override := false
	for _, w := range []string{"export ", "override ", "private "} {
		if rest, ok := strings.CutPrefix(text, w); ok {
			text, override = strings.TrimSpace(rest), override || w == "override "
		}
	}
	op, at := findMakeOp(text)
	switch {
	case op == ":" || op == "::":
		p.ruleLine(text[:at], text[at+len(op):], l)
	case op != "":
		p.rule = nil
		name := p.expandName(text[:at])
		p.m.assign(name, op, strings.TrimSpace(text[at+len(op):]), override, p.cond)
		if name == ".RECIPEPREFIX" {
			p.m.prefix = "\t"
			if v := (&makeExpand{m: p.m}).expand("$(.RECIPEPREFIX)")[0]; v != "" {
				p.m.prefix = v[:1]
			}
		}
	case strings.HasPrefix(text, "$("), strings.HasPrefix(text, "${"):
		if !strings.HasPrefix(text, "$(info ") && !strings.HasPrefix(text, "$(warning ") {
			p.m.partial = "belt cannot read the makefile line `" + text + "`"
		}
	}
}

// directive reads a conditional, an include, a define and the other
// directives, and reports whether text was one.
func (p *makeParser) directive(text string) bool {
	word, rest, _ := strings.Cut(text, " ")
	rest = strings.TrimSpace(rest)
	switch word {
	case "ifeq", "ifneq", "ifdef", "ifndef":
		p.cond++
	case "else":
	case "endif":
		p.cond = max(p.cond-1, 0)
	case "include", "-include", "sinclude":
		p.rule = nil
		p.include(rest, word == "include")
	case "define":
		p.startDefine(rest, false)
	case "override", "export":
		if after, ok := strings.CutPrefix(rest, "define "); ok {
			p.startDefine(after, word == "override")
			return true
		}
		return strings.TrimSpace(rest) == "" || !strings.ContainsAny(rest, "=:")
	case "unexport", "vpath", "undefine":
	default:
		return false
	}
	return true
}

// include reads the makefiles an include names, from make's folder or its
// include folders (findInclude). A name belt cannot expand to a plain path
// leaves the makefile partial.
func (p *makeParser) include(rest string, must bool) {
	e := &makeExpand{m: p.m}
	for _, cand := range e.expand(rest) {
		for _, name := range strings.Fields(cand) {
			if strings.ContainsAny(name, "$*?[") || e.over {
				p.m.partial = "belt cannot tell which makefile `include " + rest + "` reads"
				continue
			}
			p.m.read(p.m.findInclude(name), must)
		}
	}
	p.m.shells = append(p.m.shells, e.shells...)
}

// startDefine opens a define: `define NAME [op]`.
func (p *makeParser) startDefine(rest string, override bool) {
	name, op := strings.TrimSpace(rest), "="
	if i := strings.IndexAny(name, " \t"); i >= 0 {
		name, op = name[:i], strings.TrimSpace(name[i:])
	}
	p.define = &makeDefine{name: name, op: op, override: override}
}

// defineLine adds a line to the open define, or closes it at its endef.
func (p *makeParser) defineLine(l string) {
	d := p.define
	word, _, _ := strings.Cut(strings.TrimSpace(l), " ")
	switch {
	case word == "endef" && d.depth == 0:
		p.define = nil
		p.m.assign(p.expandName(d.name), d.op, strings.Join(d.lines, "\n"), d.override, p.cond)
		return
	case word == "endef":
		d.depth--
	case word == "define":
		d.depth++
	}
	d.lines = append(d.lines, l)
}

// expandName expands a variable name written with references in it.
func (p *makeParser) expandName(name string) string {
	e := &makeExpand{m: p.m}
	return strings.TrimSpace(e.expand(strings.TrimSpace(name))[0])
}

// ruleLine reads a rule: targets, then prerequisites up to a ; that starts
// an inline recipe line, or a target-specific assignment. In a static
// pattern rule (`objs: %.o: %.c`) each target takes the prerequisite
// pattern with its own stem.
func (p *makeParser) ruleLine(left, right string, l makeLine) {
	e := &makeExpand{m: p.m}
	targets := strings.Fields(e.expand(left)[0])
	inline, hasInline := "", false
	if i := indexOutsideRefs(right, ';'); i >= 0 {
		inline, right, hasInline = strings.TrimLeft(right[i+1:], " \t"), right[:i], true
	}
	p.rule = nil
	if op, at := findMakeOp(right); op != "" && op != ":" && op != "::" {
		p.targetVar(targets, right[:at], op, strings.TrimSpace(right[at+len(op):]))
		return
	}
	pattern := ""
	if op, at := findMakeOp(right); op == ":" {
		pattern, right = strings.TrimSpace(right[:at]), right[at+1:]
	}
	prereqs := strings.Fields(e.expand(right)[0])
	p.m.shells = append(p.m.shells, e.shells...)
	r := &makeRule{prereqs: dropWord(prereqs, "|"), file: p.file}
	if hasInline {
		r.recipe = append(r.recipe, makeLine{text: inline, line: l.line})
	}
	p.rule = r
	for _, t := range targets {
		p.m.addRule(t, r, pattern)
	}
}

// targetVar sets a target-specific variable for each target: the value
// the line gives it on top of the global one, which stays as it was.
func (p *makeParser) targetVar(targets []string, name, op, value string) {
	name = strings.TrimSpace(name)
	for _, w := range []string{"export ", "override ", "private "} {
		name = strings.TrimSpace(strings.TrimPrefix(name, w))
	}
	name = p.expandName(name)
	saved, had := p.m.vars[name]
	p.m.assign(name, op, value, false, 0)
	v := p.m.vars[name]
	if had {
		p.m.vars[name] = saved
	} else {
		delete(p.m.vars, name)
	}
	for _, t := range targets {
		r := &makeRule{targets: []string{t}, file: p.file, vars: map[string]*makeVar{name: v}}
		if strings.Contains(t, "%") {
			p.m.patternVars = append(p.m.patternVars, r)
			continue
		}
		p.m.rules[t] = append(p.m.rules[t], r)
	}
}

// addRule files a rule under one target: a pattern rule, the special
// targets make reads for itself, or an explicit rule, which a static
// pattern gives its own prerequisites.
func (m *makeFile) addRule(t string, r *makeRule, pattern string) {
	switch {
	case t == ".ONESHELL":
		m.oneShell = true
		return
	case t == ".SECONDEXPANSION":
		m.secondExpansion = true
		return
	case pattern != "":
		stem, ok := matchPattern(pattern, t)
		if !ok {
			return
		}
		own := *r
		own.prereqs = make([]string, len(r.prereqs))
		for i, pre := range r.prereqs {
			own.prereqs[i] = strings.Replace(pre, "%", stem, 1)
		}
		r = &own
	case strings.Contains(t, "%"):
		r.targets = append(r.targets, t)
		if len(r.targets) == 1 {
			m.patterns = append(m.patterns, r)
		}
		return
	}
	m.rules[t] = append(m.rules[t], r)
	m.order = append(m.order, t)
}

// goal is the target make runs when the command names none: .DEFAULT_GOAL
// when it is set, else the first target that does not start with a dot
// (one with a slash may), outside the files MAKEFILES names (goalFrom).
func (m *makeFile) goal() string {
	e := &makeExpand{m: m}
	if v, ok := m.vars[".DEFAULT_GOAL"]; ok && len(v.values) > 0 {
		if g := strings.TrimSpace(e.expand("$(.DEFAULT_GOAL)")[0]); g != "" {
			return g
		}
	}
	for _, t := range m.order[m.goalFrom:] {
		if !strings.HasPrefix(t, ".") || strings.Contains(t, "/") {
			return t
		}
	}
	return ""
}

// assign sets a variable the way make does for each operator: = and define
// keep the text for later, := and ::= expand it now, ?= sets only an unset
// variable, += appends, != runs a shell command belt cannot read. Inside a
// conditional (cond > 0) a value adds to those the variable may hold, and
// a variable unset before may still be unset. The command line's values
// win unless the file says override.
func (m *makeFile) assign(name, op, value string, override bool, cond int) {
	if name == "" || m.cmdline[name] && !override {
		return
	}
	if override && cond == 0 {
		// Only another override changes it from here on, as one from the
		// command line.
		m.cmdline[name] = true
	}
	old := m.vars[name]
	var vals []makeValue
	switch op {
	case "=", ":::=":
		vals = []makeValue{{text: value}}
	case ":=", "::=":
		vals = m.expandNow(value)
	case "?=":
		if old != nil && cond == 0 {
			return
		}
		vals = []makeValue{{text: value}}
	case "+=":
		vals = m.appended(old, value)
	case "!=":
		e := &makeExpand{m: m}
		m.shells = append(m.shells, e.expand(value)...)
		vals = []makeValue{{text: makePlaceholder(name), simple: true}}
	default:
		return
	}
	switch {
	case cond == 0:
		m.vars[name] = &makeVar{values: vals}
	case old == nil:
		m.vars[name] = &makeVar{values: append([]makeValue{{text: makePlaceholder(name), simple: true}}, vals...)}
	default:
		m.vars[name] = &makeVar{values: append(old.values, vals...)}
	}
}

// expandNow expands a := value where it is set, one value per candidate.
func (m *makeFile) expandNow(value string) []makeValue {
	e := &makeExpand{m: m}
	cands := e.expand(value)
	m.shells = append(m.shells, e.shells...)
	vals := make([]makeValue, len(cands))
	for i, c := range cands {
		vals[i] = makeValue{text: c, simple: true}
	}
	return vals
}

// appended is a += onto a variable's values: expanded now onto a simple
// value, kept as text onto any other. An unset variable takes it as =.
func (m *makeFile) appended(old *makeVar, value string) []makeValue {
	if old == nil || len(old.values) == 0 {
		return []makeValue{{text: value}}
	}
	vals := make([]makeValue, 0, len(old.values))
	for _, v := range old.values {
		if !v.simple {
			vals = append(vals, makeValue{text: v.text + " " + value})
			continue
		}
		for _, add := range m.expandNow(value) {
			vals = append(vals, makeValue{text: v.text + " " + add.text, simple: true})
		}
	}
	return vals
}

// rulesFor returns the rules that make a target and its pattern stem: its
// explicit rules, joined by the first pattern rule whose target pattern
// matches it when none of them has a recipe, else the .DEFAULT rule.
func (m *makeFile) rulesFor(t string) ([]*makeRule, string) {
	rules := m.rules[t]
	for _, r := range rules {
		if len(r.recipe) > 0 {
			return rules, ""
		}
	}
	for _, r := range m.patterns {
		for _, pt := range r.targets {
			if stem, ok := matchPattern(pt, t); ok {
				return append(slices.Clone(rules), r), stem
			}
		}
	}
	if len(rules) > 0 {
		return rules, ""
	}
	return m.rules[".DEFAULT"], ""
}

// matchPattern matches a target against a pattern with one %, and returns
// the stem the % stands for.
func matchPattern(pattern, t string) (string, bool) {
	pre, post, ok := strings.Cut(pattern, "%")
	if !ok {
		return "", pattern == t
	}
	if len(t) < len(pre)+len(post) || !strings.HasPrefix(t, pre) || !strings.HasSuffix(t, post) {
		return "", false
	}
	return t[len(pre) : len(t)-len(post)], true
}

// findMakeOp finds the first assignment operator or rule colon in a
// makefile line, outside variable references, and where it starts. A
// colon in a Windows drive name is not one.
func findMakeOp(text string) (string, int) {
	depth := 0
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case c == '$' && i+1 < len(text) && (text[i+1] == '(' || text[i+1] == '{'):
			depth++
			i++
		case depth > 0 && (c == ')' || c == '}'):
			depth--
		case depth > 0:
		case c == '=':
			return "=", i
		case c == ':':
			for _, op := range []string{":::=", "::=", ":=", "::"} {
				if strings.HasPrefix(text[i:], op) {
					return op, i
				}
			}
			return ":", i
		case (c == '?' || c == '+' || c == '!') && i+1 < len(text) && text[i+1] == '=':
			return text[i : i+2], i
		}
	}
	return "", -1
}

// indexOutsideRefs is the index of the first c in text outside variable
// references, -1 for none.
func indexOutsideRefs(text string, c byte) int {
	depth := 0
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '$' && i+1 < len(text) && (text[i+1] == '(' || text[i+1] == '{'):
			depth++
			i++
		case depth > 0 && (text[i] == ')' || text[i] == '}'):
			depth--
		case depth == 0 && text[i] == c:
			return i
		}
	}
	return -1
}

// stripMakeComment cuts a line at its first # outside variable references
// that no backslash escapes.
func stripMakeComment(l string) string {
	depth := 0
	for i := 0; i < len(l); i++ {
		switch {
		case l[i] == '\\':
			i++
		case l[i] == '$' && i+1 < len(l) && (l[i+1] == '(' || l[i+1] == '{'):
			depth++
			i++
		case depth > 0 && (l[i] == ')' || l[i] == '}'):
			depth--
		case depth == 0 && l[i] == '#':
			return l[:i]
		}
	}
	return l
}

// dropWord returns words without each copy of w.
func dropWord(words []string, w string) []string {
	out := words[:0]
	for _, x := range words {
		if x != w {
			out = append(out, x)
		}
	}
	return out
}

// shownPath writes a build file's path for a denial: relative to dir when
// it lies there, else in full.
func shownPath(file, dir string) string {
	if rel, err := filepath.Rel(dir, file); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return file
}

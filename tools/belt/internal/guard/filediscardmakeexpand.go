package guard

import (
	"path"
	"regexp"
	"strconv"
	"strings"
)

// This file expands make's variable references and functions for
// file-discard (makeExpand), as far as a recipe's paths need. A reference
// belt cannot read becomes a shell variable no shell sets
// (makePlaceholder), so the walk judges a path that holds it as unknown.

// makeExpand expands text the way make does in one place: the makefile's
// variables, a rule's target-specific ones over them (overlay), the
// automatic variables of the target ($@, $<, $^, $+, $*) and the arguments
// of a $(call) (local). It gathers the $(shell) commands it meets, notes
// a function it cannot read whose text names a delete (deletes), and over
// marks text with more values than maxMakeValues or nested past
// maxMakeDepth.
type makeExpand struct {
	m       *makeFile
	overlay map[string]*makeVar
	auto    map[string]string
	local   map[string]string
	shells  []string
	deletes string
	over    bool
	depth   int
}

// maxMakeValues bounds the values one expansion may take, a product of
// the variables a conditional sets to more than one; maxMakeDepth bounds
// how deep references nest.
const (
	maxMakeValues = 8
	maxMakeDepth  = 32
)

// makeDefaults are the variables make sets itself that a recipe may use.
var makeDefaults = map[string]string{
	"MAKE": "make", "RM": "rm -f", "SHELL": "/bin/sh", "CC": "cc", "CXX": "c++", "AR": "ar",
	"AS": "as", "LD": "ld", "MAKEFLAGS": "", "MFLAGS": "",
}

// makeFunctions are the names of make's functions.
var makeFunctions = wordSet("subst patsubst strip findstring filter filter-out sort word wordlist " +
	"words firstword lastword dir notdir suffix basename addsuffix addprefix join wildcard realpath " +
	"abspath error warning info shell origin flavor foreach if or and call eval file value guile let intcmp")

// makePlaceholder is the shell text for a make reference belt cannot read:
// a variable no shell sets.
func makePlaceholder(name string) string {
	b := []byte(name)
	for i, c := range b {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			b[i] = '_'
		}
	}
	return "${__make_" + string(b) + "}"
}

// expand returns the values text expands to: one, or one per combination
// of the values its variables may hold.
func (e *makeExpand) expand(text string) []string {
	if e.depth >= maxMakeDepth {
		e.over = true
		return []string{makePlaceholder("deep")}
	}
	e.depth++
	defer func() { e.depth-- }()
	out := []string{""}
	for i := 0; i < len(text); {
		k := strings.IndexByte(text[i:], '$')
		if k < 0 {
			return joinEach(out, text[i:])
		}
		out = joinEach(out, text[i:i+k])
		i += k
		switch {
		case i+1 >= len(text):
			return joinEach(out, "$")
		case text[i+1] == '$':
			out, i = joinEach(out, "$"), i+2
		case text[i+1] == '(' || text[i+1] == '{':
			end := refEnd(text, i+1)
			if end < 0 {
				return joinEach(out, makePlaceholder("ref"))
			}
			out, i = e.product(out, e.ref(text[i+2:end])), end+1
		default:
			out, i = e.product(out, e.value(text[i+1:i+2])), i+2
		}
	}
	return out
}

// joinEach adds s to the end of each value.
func joinEach(vals []string, s string) []string {
	for i := range vals {
		vals[i] += s
	}
	return vals
}

// product joins each value with each of next, up to maxMakeValues.
func (e *makeExpand) product(vals, next []string) []string {
	out := make([]string, 0, len(vals)*len(next))
	for _, v := range vals {
		for _, n := range next {
			if len(out) == maxMakeValues {
				e.over = true
				return out
			}
			out = append(out, v+n)
		}
	}
	return out
}

// refEnd is the index of the bracket that closes the one at text[open],
// counting brackets of its kind, -1 for none.
func refEnd(text string, open int) int {
	closer := byte(')')
	if text[open] == '{' {
		closer = '}'
	}
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case text[open]:
			depth++
		case closer:
			if depth--; depth == 0 {
				return i
			}
		}
	}
	return -1
}

// ref expands the inside of one $( ) or ${ }: a function call, a
// substitution reference (`$(SRCS:.c=.o)`) or a variable, whose name may
// itself hold references.
func (e *makeExpand) ref(inner string) []string {
	if fn, args, ok := strings.Cut(inner, " "); ok && makeFunctions[fn] {
		return e.function(fn, strings.TrimLeft(args, " \t"), inner)
	}
	name, subst := inner, ""
	if i := indexOutsideRefs(inner, ':'); i >= 0 && strings.Contains(inner[i:], "=") {
		name, subst = inner[:i], inner[i+1:]
	}
	var out []string
	for _, n := range e.expand(name) {
		for _, v := range e.value(strings.TrimSpace(n)) {
			if subst != "" {
				v = e.substRef(v, subst)
			}
			out = append(out, v)
		}
	}
	return out
}

// substRef applies a substitution reference's `from=to` to each word of v:
// a suffix, or a pattern with %.
func (e *makeExpand) substRef(v, subst string) string {
	from, to, _ := strings.Cut(e.expand(subst)[0], "=")
	if !strings.Contains(from, "%") {
		from, to = "%"+from, "%"+to
	}
	words := strings.Fields(v)
	for i, w := range words {
		if stem, ok := matchPattern(from, w); ok {
			words[i] = strings.Replace(to, "%", stem, 1)
		}
	}
	return strings.Join(words, " ")
}

// value returns the values a variable may hold: an automatic variable or
// a $(call) argument, with its D and F forms; a target-specific or global
// variable, expanded unless make expanded it where it was set; one make
// sets itself; and for any other, a placeholder, since the environment
// may set it.
func (e *makeExpand) value(name string) []string {
	if v, ok := e.auto[name]; ok {
		return []string{v}
	}
	if v, ok := e.local[name]; ok {
		return []string{v}
	}
	if len(name) == 2 && (name[1] == 'D' || name[1] == 'F') {
		if v, ok := e.auto[name[:1]]; ok {
			return []string{dirOrFile(v, name[1] == 'D')}
		}
	}
	v := e.overlay[name]
	if v == nil {
		v = e.m.vars[name]
	}
	switch {
	case v != nil:
	case name == "CURDIR":
		return []string{e.m.dir}
	case name == "MAKECMDGOALS":
		return []string{makePlaceholder(name)}
	default:
		if d, ok := makeDefaults[name]; ok {
			return []string{d}
		}
		return []string{makePlaceholder(name)}
	}
	var out []string
	for _, val := range v.values {
		if val.simple {
			out = append(out, val.text)
			continue
		}
		out = append(out, e.expand(val.text)...)
	}
	return out
}

// dirOrFile is the folder part ($(@D)) or the file part ($(@F)) of each
// word.
func dirOrFile(v string, dir bool) string {
	words := strings.Fields(v)
	for i, w := range words {
		if dir {
			words[i] = path.Dir(w)
		} else {
			words[i] = path.Base(w)
		}
	}
	return strings.Join(words, " ")
}

// function expands one function call. $(shell) runs its command, which
// belt walks, and prints what belt cannot read; $(call) expands a variable
// with its arguments; $(strip) and $(abspath) are read. Any other function
// is a placeholder, and when its text, or the variables it names, name a
// delete, the line holds a delete belt cannot read (deletes).
func (e *makeExpand) function(fn, args, inner string) []string {
	switch fn {
	case "shell":
		e.shells = append(e.shells, e.expand(args)...)
		return []string{makePlaceholder("shell")}
	case "call":
		return e.call(args)
	case "strip":
		out := e.expand(args)
		for i, v := range out {
			out[i] = strings.Join(strings.Fields(v), " ")
		}
		return out
	case "abspath":
		out := e.expand(args)
		for i, v := range out {
			words := strings.Fields(v)
			for k, w := range words {
				words[k] = joinDir(e.m.dir, w)
			}
			out[i] = strings.Join(words, " ")
		}
		return out
	}
	if e.deletes == "" && e.namesDelete(inner) {
		e.deletes = "$(" + inner + ")"
	}
	return []string{makePlaceholder(fn)}
}

// call expands $(call name,args...): the variable name with $(1) and on
// set to each argument and $(0) to the name.
func (e *makeExpand) call(args string) []string {
	parts := splitMakeArgs(args)
	name := strings.TrimSpace(e.expand(parts[0])[0])
	local := map[string]string{"0": name}
	for i, a := range parts[1:] {
		local[strconv.Itoa(i+1)] = e.expand(a)[0]
	}
	saved := e.local
	e.local = local
	defer func() { e.local = saved }()
	return e.value(name)
}

// splitMakeArgs splits a function's arguments at the commas outside
// references.
func splitMakeArgs(args string) []string {
	var out []string
	for {
		i := indexOutsideRefs(args, ',')
		if i < 0 {
			return append(out, args)
		}
		out, args = append(out, args[:i]), args[i+1:]
	}
}

// makeRefName finds the variables a text names: $(NAME), ${NAME}, and the
// name a $(call) takes.
var makeRefName = regexp.MustCompile(`\$[({](?:call\s+)?([^\s:=$(){},]+)`)

// namesDelete reports whether text, or the value of a variable it names,
// names a delete.
func (e *makeExpand) namesDelete(text string) bool {
	if namesDelete(text) {
		return true
	}
	for _, m := range makeRefName.FindAllStringSubmatch(text, -1) {
		v := e.overlay[m[1]]
		if v == nil {
			v = e.m.vars[m[1]]
		}
		if v == nil {
			continue
		}
		for _, val := range v.values {
			if namesDelete(val.text) {
				return true
			}
		}
	}
	return false
}

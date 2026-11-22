package guard

import (
	"cmp"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// This file reads a justfile for file-discard (justFile): its recipes with
// their parameters, dependencies and bodies, and its variables, as far as
// a recipe's deletes need. An expression belt cannot read becomes a shell
// variable no shell sets (justPlaceholder).

// justFile is what belt read of a justfile and the files it imports: the
// recipes by name, the first one (the default), the aliases, the
// variables as their expressions, the commands just runs as it evaluates
// them (backticks), and the files of its modules by name (mods). dir is
// the folder recipes run in. dotenv is what its dotenv settings ask for,
// and positional marks `set positional-arguments`.
type justFile struct {
	view       *buildView
	lang       string
	unread     string
	dir        string
	file       string
	files      []string
	text       strings.Builder
	recipes    map[string]*justRecipe
	first      string
	aliases    map[string]string
	vars       map[string]string
	backticks  []string
	mods       map[string]string
	dotenv     justDotenv
	positional bool
}

// justDotenv is what the dotenv settings or options ask for: load marks a
// .env file just loads, name the file it looks for from its working folder
// up (dotenv-filename, .env by default), path the one file it loads
// (dotenv-path).
type justDotenv struct {
	load       bool
	name, path string
}

// file returns the .env file just loads when it works from the folder dir,
// "" for none: the dotenv-path file, else the first file of the name the
// tool finds (buildView.exists) in dir or a folder above it.
func (d justDotenv) file(dir string, v *buildView) string {
	if !d.load {
		return ""
	}
	if d.path != "" {
		if p := joinDir(dir, d.path); v.exists(p) {
			return p
		}
		return ""
	}
	name := cmp.Or(d.name, ".env")
	for {
		if p := filepath.Join(dir, name); v.exists(p) {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// justRecipe is one recipe: its parameters, the recipes it runs before and
// after its body, its body lines, where it runs and whether its body is
// one script (a #! line or [script]) and in what. positional marks the
// [positional-arguments] attribute.
type justRecipe struct {
	name, file string
	line       int
	params     []justParam
	deps, post []justDep
	body       []makeLine
	dir        string
	script     string
	positional bool
}

// justParam is one parameter: its name, its default expression ("" for
// none) and whether it takes every argument left (+ or *).
type justParam struct {
	name, def string
	variadic  bool
}

// justDep is a recipe a recipe runs, with its argument expressions.
type justDep struct {
	name string
	args []string
}

// maxJustFiles bounds the justfiles one read takes in.
const maxJustFiles = 16

var (
	justAssign = regexp.MustCompile(`^(?:export\s+)?([A-Za-z_][\w-]*)\s*:=\s*(.*)$`)
	justHeader = regexp.MustCompile(`^@?([A-Za-z_][\w-]*)((?:\s+[^:]*?)?)\s*:([^=].*|)$`)
	justAlias  = regexp.MustCompile(`^alias\s+([A-Za-z_][\w-]*)\s*:=\s*([A-Za-z_][\w-]*)`)
	// justMod is `mod foo` or `mod? foo 'path'`, with the name as submatch
	// 1 and the path as submatch 2.
	justMod    = regexp.MustCompile(`^mod\??\s+([A-Za-z_][\w-]*)(?:\s+(\S.*))?$`)
	justSetDir = regexp.MustCompile(`^set\s+working-directory\s*:=\s*(.*)$`)
	// justSetShell is `set shell := ["python3", "-c"]`, with the program as
	// submatch 1.
	justSetShell = regexp.MustCompile(`^set\s+shell\s*:=\s*\[\s*['"]([^'"]*)['"]`)
	// justSetting is `set NAME` or `set NAME := VALUE`, with the name as
	// submatch 1 and the value as submatch 2.
	justSetting = regexp.MustCompile(`^set\s+([a-z-]+)\s*(?::=\s*(.*))?$`)
)

// setting reads the dotenv settings and positional-arguments. A boolean
// setting with no value is true.
func (jf *justFile) setting(t string) {
	m := justSetting.FindStringSubmatch(t)
	if m == nil {
		return
	}
	value := strings.TrimSpace(m[2])
	on := value == "" || value == "true"
	switch m[1] {
	case "dotenv-load", "dotenv-required", "dotenv-override":
		jf.dotenv.load = jf.dotenv.load || on
	case "dotenv-filename":
		jf.dotenv.name, _ = justString(value)
		jf.dotenv.load = true
	case "dotenv-path":
		jf.dotenv.path, _ = justString(value)
		jf.dotenv.load = true
	case "positional-arguments":
		jf.positional = on
	}
}

// shell takes the program a `set shell` names: recipe lines then run in a
// script language belt reads (langDeleteCalls), in a POSIX shell, or in
// one belt does not read (unread).
func (jf *justFile) shell(program string) {
	name := filepath.Base(program)
	lang := interpreterLang(name)
	_, read := langDeleteCalls[lang]
	switch jf.lang, jf.unread = "", ""; {
	case posixShells[name]:
	case read:
		jf.lang = lang
	default:
		jf.unread = "belt does not read recipes just runs with the shell `" + program + "`"
	}
}

// readJustFile reads the justfile at file through v, whose recipes run in
// its folder.
func readJustFile(file string, v *buildView) (*justFile, bool) {
	jf := &justFile{
		view: v, dir: filepath.Dir(file), file: file, recipes: map[string]*justRecipe{}, aliases: map[string]string{},
		vars: map[string]string{}, mods: map[string]string{},
	}
	if !jf.read(file) {
		return nil, false
	}
	return jf, true
}

// read reads one justfile into jf: settings, aliases, imports, variables
// and recipes, each recipe with the attributes on the lines above it.
func (jf *justFile) read(file string) bool {
	text, ok := jf.view.read(file)
	if !ok || len(jf.files) >= maxJustFiles {
		return false
	}
	jf.files = append(jf.files, file)
	jf.text.WriteString(text + "\n")
	lines := strings.Split(text, "\n")
	var attrs []string
	for i := 0; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t\r")
		t := strings.TrimSpace(l)
		switch {
		case t == "" || strings.HasPrefix(t, "#") || l[0] == ' ' || l[0] == '\t':
		case strings.HasPrefix(t, "["):
			attrs = append(attrs, strings.Trim(t, "[]"))
		case strings.HasPrefix(t, "set "):
			if m := justSetDir.FindStringSubmatch(t); m != nil {
				d, _ := jf.eval(m[1], nil)
				jf.dir = joinDir(filepath.Dir(file), d)
			}
			if m := justSetShell.FindStringSubmatch(t); m != nil {
				jf.shell(m[1])
			}
			jf.setting(t)
		case strings.HasPrefix(t, "import"):
			jf.importFile(file, t)
		case justMod.MatchString(t) && !justHeader.MatchString(t):
			m := justMod.FindStringSubmatch(t)
			jf.mods[m[1]] = jf.modFile(file, m[1], m[2])
		case justAlias.MatchString(t):
			m := justAlias.FindStringSubmatch(t)
			jf.aliases[m[1]] = m[2]
		case justAssign.MatchString(t):
			m := justAssign.FindStringSubmatch(t)
			jf.vars[m[1]] = m[2]
		case justHeader.MatchString(t):
			i = jf.recipe(file, lines, i, attrs)
			attrs = nil
		}
	}
	return true
}

// modFile returns the file of the module a mod line names, from the folder
// of the file that names it: the path it gives, a file or a folder, else
// NAME.just or a justfile in the folder NAME; "" for none.
func (jf *justFile) modFile(from, name, path string) string {
	dir := filepath.Dir(from)
	var tries []string
	if p, ok := justString(path); ok {
		p = joinDir(dir, p)
		tries = []string{p, filepath.Join(p, "mod.just"), filepath.Join(p, "justfile"), filepath.Join(p, ".justfile")}
	} else {
		sub := filepath.Join(dir, name)
		tries = []string{sub + ".just", filepath.Join(sub, "mod.just"), filepath.Join(sub, "justfile"),
			filepath.Join(sub, ".justfile")}
	}
	for _, p := range tries {
		if jf.view.exists(p) {
			return p
		}
	}
	return ""
}

// importFile reads the justfile an import line names, from the folder of
// the file that imports it.
func (jf *justFile) importFile(from, line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "import"), "?"))
	if p, ok := justString(rest); ok {
		jf.read(joinDir(filepath.Dir(from), p))
	}
}

// recipe reads the recipe whose header is lines[i] and the body under it,
// and returns the index of its last line. A recipe for another system
// than this one is dropped.
func (jf *justFile) recipe(file string, lines []string, i int, attrs []string) int {
	m := justHeader.FindStringSubmatch(strings.TrimSpace(lines[i]))
	r := &justRecipe{name: m[1], file: file, line: i + 1, params: justParams(m[2]), dir: jf.dir}
	r.deps, r.post = justDeps(m[3])
	end := i
	for k := i + 1; k < len(lines); k++ {
		l := strings.TrimRight(lines[k], " \t\r")
		if l != "" && l[0] != ' ' && l[0] != '\t' {
			break
		}
		if l != "" {
			end = k
		}
	}
	indent := ""
	for k := i + 1; k <= end; k++ {
		l := strings.TrimRight(lines[k], "\r")
		if strings.TrimSpace(l) == "" {
			continue
		}
		if indent == "" {
			indent = l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		}
		r.body = append(r.body, makeLine{text: strings.TrimPrefix(l, indent), line: k + 1})
	}
	if !jf.applyAttrs(r, attrs) {
		return end
	}
	if len(r.body) > 0 && strings.HasPrefix(r.body[0].text, "#!") {
		r.script = strings.TrimPrefix(r.body[0].text, "#!")
	}
	if jf.first == "" || hasAttr(attrs, "default") {
		jf.first = r.name
	}
	jf.recipes[r.name] = r
	return end
}

// justSystems are the attributes that limit a recipe to some systems, and
// justHere the ones this system answers to.
var (
	justSystems = wordSet("linux macos windows unix openbsd freebsd netbsd dragonfly android ios")
	justHere    = map[string]bool{runtime.GOOS: true, "unix": runtime.GOOS != "windows",
		"macos": runtime.GOOS == "darwin"}
)

// applyAttrs applies a recipe's attributes: [no-cd] runs it in the folder
// just was run from, [working-directory] in another, [script] runs its body
// as one script. It reports false for a recipe for other systems.
func (jf *justFile) applyAttrs(r *justRecipe, attrs []string) bool {
	limited, here := false, false
	for _, line := range attrs {
		for _, a := range strings.Split(line, ",") {
			a = strings.TrimSpace(a)
			name, arg := a, ""
			if i := strings.IndexAny(a, "(:"); i >= 0 {
				name, arg = strings.TrimSpace(a[:i]), strings.Trim(strings.TrimSpace(a[i+1:]), "()")
			}
			switch {
			case justSystems[name]:
				limited, here = true, here || justHere[name]
			case name == "no-cd":
				r.dir = ""
			case name == "positional-arguments":
				r.positional = true
			case name == "working-directory":
				d, _ := justString(strings.TrimSpace(arg))
				r.dir = joinDir(jf.dir, d)
			case name == "script":
				r.script = "sh -eu"
				if s, ok := justString(strings.TrimSpace(arg)); ok {
					r.script = s
				}
			}
		}
	}
	return !limited || here
}

// hasAttr reports whether a recipe's attribute lines name an attribute.
func hasAttr(attrs []string, name string) bool {
	for _, line := range attrs {
		for _, a := range strings.Split(line, ",") {
			if strings.TrimSpace(a) == name {
				return true
			}
		}
	}
	return false
}

// justParams reads a recipe's parameters: `name`, `name='default'`,
// `+name` and `*name`, with $ marking one exported.
func justParams(text string) []justParam {
	var out []justParam
	for _, word := range justWords(text) {
		p := justParam{}
		word = strings.TrimLeft(word, "$")
		if strings.HasPrefix(word, "+") || strings.HasPrefix(word, "*") {
			p.variadic, word = true, strings.TrimLeft(word[1:], "$")
		}
		p.name, p.def, _ = strings.Cut(word, "=")
		out = append(out, p)
	}
	return out
}

// justDeps reads the recipes a header names after its colon: those before
// `&&` run before the body, the rest after it. `(name args...)` passes
// arguments.
func justDeps(text string) (deps, post []justDep) {
	if i := strings.Index(text, "#"); i >= 0 && !strings.ContainsAny(text[:i], `'"`) {
		text = text[:i]
	}
	after := false
	for _, word := range justWords(text) {
		if word == "&&" {
			after = true
			continue
		}
		d := justDep{name: word}
		if strings.HasPrefix(word, "(") {
			parts := justWords(strings.Trim(word, "()"))
			if len(parts) == 0 {
				continue
			}
			d = justDep{name: parts[0], args: parts[1:]}
		}
		if after {
			post = append(post, d)
		} else {
			deps = append(deps, d)
		}
	}
	return deps, post
}

// justWords splits text at spaces outside quotes, backticks and
// parentheses.
func justWords(text string) []string {
	var out []string
	var b strings.Builder
	depth, quote := 0, byte(0)
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case (c == ' ' || c == '\t') && depth == 0:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
			continue
		}
		b.WriteByte(c)
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// justString reads a just string literal: 'raw' or "with escapes"
// (\n, \t, \\ and \"). ok is false for anything else.
func justString(text string) (string, bool) {
	if len(text) < 2 || text[0] != text[len(text)-1] || text[0] != '\'' && text[0] != '"' {
		return "", false
	}
	body := text[1 : len(text)-1]
	if text[0] == '\'' {
		return body, !strings.Contains(body, "'")
	}
	r := strings.NewReplacer(`\n`, "\n", `\t`, "\t", `\\`, `\`, `\"`, `"`)
	return r.Replace(body), true
}

// justPlaceholder is the shell text for a just expression belt cannot
// read: a variable no shell sets.
func justPlaceholder() string { return "${__just_value}" }

// eval evaluates a just expression with the recipe's arguments (args):
// string literals, variables and parameters joined by + and /, and the
// functions that name a folder. A backtick runs its command, which belt
// walks, and gives what belt cannot read. ok is false for any other
// expression, which is a placeholder.
func (jf *justFile) eval(expr string, args map[string]string) (string, bool) {
	return jf.evalDepth(expr, args, 0)
}

// maxJustDepth bounds how deep variables name one another.
const maxJustDepth = 16

func (jf *justFile) evalDepth(expr string, args map[string]string, depth int) (string, bool) {
	terms, ops := splitJustExpr(strings.TrimSpace(expr))
	if terms == nil || depth > maxJustDepth {
		return justPlaceholder(), false
	}
	out, known := "", true
	for i, t := range terms {
		v, ok := jf.term(t, args, depth)
		known = known && ok
		switch {
		case i == 0:
			out = v
		case ops[i-1] == '/':
			out = strings.TrimSuffix(out, "/") + "/" + strings.TrimPrefix(v, "/")
		default:
			out += v
		}
	}
	return out, known
}

// term evaluates one term of an expression.
func (jf *justFile) term(t string, args map[string]string, depth int) (string, bool) {
	if s, ok := justString(t); ok {
		return s, true
	}
	if strings.HasPrefix(t, "`") && strings.HasSuffix(t, "`") && len(t) > 1 {
		jf.backticks = append(jf.backticks, t[1:len(t)-1])
		return justPlaceholder(), false
	}
	if v, ok := args[t]; ok {
		return v, true
	}
	if v, ok := jf.vars[t]; ok {
		return jf.evalDepth(v, nil, depth+1)
	}
	switch strings.ReplaceAll(t, " ", "") {
	case "justfile_directory()", "justfile_dir()", "source_directory()", "source_dir()":
		return filepath.Dir(jf.file), true
	}
	return justPlaceholder(), false
}

// splitJustExpr splits an expression at the + and / outside quotes,
// backticks and parentheses, into its terms and the operators between
// them. terms is nil for an empty expression.
func splitJustExpr(expr string) (terms []string, ops []byte) {
	if expr == "" {
		return nil, nil
	}
	depth, quote, start := 0, byte(0), 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"' || c == '`':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case depth == 0 && (c == '+' || c == '/'):
			terms, ops = append(terms, strings.TrimSpace(expr[start:i])), append(ops, c)
			start = i + 1
		}
	}
	return append(terms, strings.TrimSpace(expr[start:])), ops
}

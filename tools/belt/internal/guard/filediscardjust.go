package guard

import (
	"cmp"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// This file holds file-discard's reading of a just command (readJust): its
// options, its justfile, and the body each recipe it runs and each recipe
// those run first and after, with their arguments filled in.

// justValueOptions are just's options that take the next word as their
// value, and justInfo the ones that run no recipe.
var (
	justValueOptions = wordSet("-f --justfile -d --working-directory --shell --shell-arg " +
		"--dotenv-filename --dotenv-path -E --color --chooser --command-color --list-heading --list-prefix " +
		"--timestamp-format -s --show --completions --set")
	justInfo = wordSet("-l --list --summary -s --show --dump --evaluate --variables --fmt --init --edit " +
		"--choose --completions -h --help -V --version --changelog --groups -n --dry-run --check -c " +
		"--command -u --unsorted")
)

// justArgs is what a just command asks for: its justfile, the folder it
// was run from, the variables it sets, the .env file its options name and
// the recipe words.
type justArgs struct {
	file, dir string
	vars      map[string]string
	dotenv    justDotenv
	words     []shWord
	noDeps    bool
}

// readJust reads what a just command run from dir runs: each recipe it
// names with its arguments, or the default recipe, and the backticks just
// runs as it evaluates the justfile's variables. A command with no
// justfile to read runs nothing, and so does one naming a recipe the
// justfile does not hold. The .env file just loads (justDotenv) is one of
// the files the run reads, from just's working folder: the justfile's, or
// the one -d names.
func readJust(args []shWord, _ map[string]string, dir string, v *buildView) (buildRun, bool) {
	a, ok := readJustArgs(args, dir)
	if !ok {
		return buildRun{}, false
	}
	if a.file == "" {
		a.file = findJustfile(dir, v)
	}
	jf, ok := readJustFile(a.file, v)
	if !ok {
		return buildRun{}, false
	}
	work := filepath.Dir(a.file)
	if a.dir != dir {
		jf.dir, work = a.dir, a.dir
	}
	for name, v := range a.vars {
		jf.vars[name] = "'" + strings.ReplaceAll(v, "'", "") + "'"
	}
	run := buildRun{file: a.file, files: slices.Clip(jf.files), text: jf.text.String(), unread: jf.unread}
	if env := jf.dotenvFrom(a.dotenv).file(work, v); env != "" {
		run.files = append(run.files, env)
	}
	jr := &justRun{jf: jf, invoked: dir, seen: map[string]bool{}, noDeps: a.noDeps, run: &run}
	jr.evaluate()
	if !jr.calls(a.words) {
		return buildRun{}, false
	}
	return run, true
}

// readJustArgs reads just's options and the NAME=VALUE words in front of
// the recipes. ok is false for an option that runs no recipe.
func readJustArgs(args []shWord, dir string) (justArgs, bool) {
	a := justArgs{dir: dir, vars: map[string]string{}}
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		name, value, hasValue := strings.Cut(v, "=")
		switch {
		case !lit || !strings.HasPrefix(v, "-"):
			if lit && hasValue && justName.MatchString(name) && len(a.words) == 0 {
				a.vars[name] = value
				continue
			}
			a.words = append(a.words, args[i:]...)
			return a, true
		case justInfo[name]:
			return a, false
		case name == "--no-deps":
			a.noDeps = true
		case name == "--set" && i+2 < len(args):
			// --set NAME VALUE sets a variable, as NAME=VALUE does.
			set, _ := args[i+1].literal()
			a.vars[set], _ = args[i+2].literal()
			i += 2
			continue
		case justValueOptions[name] && !hasValue && i+1 < len(args):
			i++
			value, _ = args[i].literal()
		}
		switch name {
		case "-f", "--justfile":
			a.file = joinDir(dir, value)
		case "-d", "--working-directory":
			a.dir = joinDir(dir, value)
		case "--dotenv-filename":
			a.dotenv = justDotenv{load: true, name: value, path: a.dotenv.path}
		case "-E", "--dotenv-path":
			a.dotenv = justDotenv{load: true, name: a.dotenv.name, path: value}
		}
	}
	return a, true
}

// dotenvFrom returns the dotenv settings with the options' over them.
func (jf *justFile) dotenvFrom(opts justDotenv) justDotenv {
	d := jf.dotenv
	d.load = d.load || opts.load
	d.name, d.path = cmp.Or(opts.name, d.name), cmp.Or(opts.path, d.path)
	return d
}

// justName is a just variable or recipe name.
var justName = regexp.MustCompile(`^[A-Za-z_][\w-]*$`)

// findJustfile returns the justfile just finds from dir: the first file
// named justfile or .justfile, in any case, that a step before it writes
// (discardJudge.writes) or that is on disk, in dir or the nearest folder
// above it that holds one; "" for none.
func findJustfile(dir string, v *buildView) string {
	for {
		for _, name := range []string{"justfile", ".justfile"} {
			if p := filepath.Join(dir, name); v.j.writes(p) {
				return p
			}
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if n := strings.ToLower(e.Name()); (n == "justfile" || n == ".justfile") && !e.IsDir() {
				return filepath.Join(dir, e.Name())
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// justRun gathers the bodies of the recipes one just command runs, each
// recipe with the same arguments once.
type justRun struct {
	jf      *justFile
	invoked string
	seen    map[string]bool
	noDeps  bool
	run     *buildRun
}

// evaluate evaluates the justfile's variables, as just does before it runs
// a recipe, and adds the backticks they run.
func (r *justRun) evaluate() {
	names := make([]string, 0, len(r.jf.vars))
	for name := range r.jf.vars {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r.jf.eval(r.jf.vars[name], nil)
	}
	for _, b := range r.jf.backticks {
		r.run.bodies = append(r.run.bodies, buildBody{
			what: "reading " + shownPath(r.jf.file, r.invoked) + ", just runs `" + shortLine(b) + "`", text: b,
			dir: r.jf.dir, file: r.jf.file,
		})
	}
}

// calls reads the recipe words: each recipe takes as many arguments as it
// has parameters (a variadic one takes the rest), and no word means the
// default recipe. It reports false when a word names no recipe.
func (r *justRun) calls(words []shWord) bool {
	if len(words) == 0 {
		return r.jf.first != "" && r.recipe(r.jf.first, nil)
	}
	for len(words) > 0 {
		rest, ok := r.call(words)
		if !ok {
			return false
		}
		words = rest
	}
	return true
}

// call reads the first recipe word with its arguments, and returns the
// words after them. A word that names a module (`foo::clean`, or `foo
// clean` for a module foo) runs the recipe in that module.
func (r *justRun) call(words []shWord) ([]shWord, bool) {
	name, ok := words[0].literal()
	if !ok {
		r.run.unread = "belt cannot tell which recipe `" + words[0].raw + "` names"
		return nil, true
	}
	rec := r.lookup(name)
	if rec == nil {
		return r.module(name, words[1:])
	}
	words = words[1:]
	var args []string
	for _, p := range rec.params {
		if len(words) == 0 {
			break
		}
		take := 1
		if p.variadic {
			take = len(words)
		}
		for _, w := range words[:take] {
			args = append(args, argText(w))
		}
		words = words[take:]
	}
	return words, r.recipe(name, args)
}

// module runs the recipe a module word names, in the module's own
// justfile, whose recipes run in its folder: the rest of a `foo::clean`
// word, else the next word, else the module's default recipe. ok is false
// for a word that names no module.
func (r *justRun) module(name string, rest []shWord) ([]shWord, bool) {
	mod, sub, path := name, "", false
	if before, after, found := strings.Cut(name, "::"); found {
		mod, sub, path = before, after, true
	}
	file, ok := r.jf.mods[mod]
	if !ok {
		return nil, false
	}
	jf, ok := readJustFile(file, r.jf.view)
	if file == "" || !ok {
		r.run.unread = "belt cannot read the just module `" + mod + "`"
		return nil, true
	}
	r.run.files = append(r.run.files, jf.files...)
	if r.run.unread == "" {
		r.run.unread = jf.unread
	}
	m := &justRun{jf: jf, invoked: r.invoked, seen: map[string]bool{}, noDeps: r.noDeps, run: r.run}
	m.evaluate()
	switch {
	case path:
		return m.call(append([]shWord{literalWord(sub)}, rest...))
	case len(rest) == 0:
		return nil, jf.first != "" && m.recipe(jf.first, nil)
	}
	return m.call(rest)
}

// lookup returns the recipe a name or an alias names, nil for none.
func (r *justRun) lookup(name string) *justRecipe {
	if target, ok := r.jf.aliases[name]; ok {
		name = target
	}
	return r.jf.recipes[name]
}

// recipe adds one recipe's bodies with its arguments: the recipes it runs
// first, its body, then the recipes after its &&.
func (r *justRun) recipe(name string, args []string) bool {
	rec := r.lookup(name)
	if rec == nil {
		return false
	}
	key := rec.name + "\x00" + strings.Join(args, "\x00")
	if r.seen[key] {
		return true
	}
	if len(r.run.bodies) >= maxBuildBodies {
		r.run.unread = overBodies(r.run.file).why
		return true
	}
	r.seen[key] = true
	vals := r.params(rec, args)
	if !r.noDeps {
		r.deps(rec.deps, vals)
	}
	r.body(rec, vals)
	if !r.noDeps {
		r.deps(rec.post, vals)
	}
	return true
}

// params binds a recipe's parameters: each argument, else its default.
// A variadic parameter takes every argument left.
func (r *justRun) params(rec *justRecipe, args []string) map[string]string {
	vals := map[string]string{}
	for i, p := range rec.params {
		switch {
		case p.variadic && i < len(args):
			vals[p.name] = strings.Join(args[i:], " ")
		case i < len(args):
			vals[p.name] = args[i]
		case p.def != "":
			vals[p.name], _ = r.jf.eval(p.def, vals)
		default:
			vals[p.name] = ""
		}
	}
	return vals
}

// deps adds the recipes a recipe runs, their arguments evaluated with the
// recipe's own.
func (r *justRun) deps(deps []justDep, vals map[string]string) {
	for _, d := range deps {
		args := make([]string, len(d.args))
		for i, a := range d.args {
			args[i], _ = r.jf.eval(a, vals)
		}
		r.recipe(d.name, args)
	}
}

// justInterp finds the {{ }} interpolations of a recipe line.
var justInterp = regexp.MustCompile(`\{\{(.*?)\}\}`)

// body adds a recipe's body: each line in a shell of its own, a line that
// ends in a backslash joined with the next; or, for a script recipe, the
// whole body fed to its interpreter.
func (r *justRun) body(rec *justRecipe, vals map[string]string) {
	dir := rec.dir
	if dir == "" {
		dir = r.invoked
	}
	var lines []string
	for _, l := range rec.body {
		lines = append(lines, r.interpolate(l.text, vals))
	}
	if rec.script != "" {
		r.add(rec, rec.line, dir, scriptFeed(rec.script, lines), "#!"+rec.script, "")
		return
	}
	var pos []string
	if (r.jf.positional || rec.positional) && r.jf.lang == "" {
		pos = justPositionals(rec, vals)
	}
	for i := 0; i < len(lines); i++ {
		text, line := strings.TrimLeft(lines[i], "@-"), rec.body[i].line
		for continues(text) && i+1 < len(lines) {
			i++
			text = text[:len(text)-1] + strings.TrimLeft(lines[i], " \t")
		}
		r.add(rec, line, dir, fillPositional(text, pos), text, r.jf.lang)
	}
}

// justPositionals returns what a recipe's shell gets as $1 and on under
// positional-arguments: its parameters' values in order, up to a variadic
// one, whose words belt does not split.
func justPositionals(rec *justRecipe, vals map[string]string) []string {
	var out []string
	for _, p := range rec.params {
		if p.variadic {
			break
		}
		out = append(out, vals[p.name])
	}
	return out
}

// justPositionalRef is a shell reference to a positional parameter at the
// start of a text: $1 to $9, or ${N}.
var justPositionalRef = regexp.MustCompile(`^\$(?:([1-9])|\{([1-9][0-9]*)\})`)

// fillPositional fills a shell line's references to its positional
// parameters (justPositionalRef) outside single quotes with the values pos
// gives them, each one a plain word (quoteWord leaves it as it is). A
// reference past them, or to a value that is not plain, stays for the walk.
func fillPositional(line string, pos []string) string {
	if len(pos) == 0 {
		return line
	}
	var b strings.Builder
	single, double := false, false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == '\\' && !single && i+1 < len(line):
			b.WriteString(line[i : i+2])
			i++
			continue
		case c == '\'' && !double:
			single = !single
		case c == '"' && !single:
			double = !double
		case c == '$' && !single:
			if v, n := positionalAt(line[i:], pos); n > 0 {
				b.WriteString(v)
				i += n - 1
				continue
			}
		}
		b.WriteByte(c)
	}
	return b.String()
}

// positionalAt returns the value pos gives the positional reference at the
// start of text (justPositionalRef) and the reference's length, or a
// length of 0 when there is none or its value is no plain word.
func positionalAt(text string, pos []string) (string, int) {
	m := justPositionalRef.FindStringSubmatch(text)
	if m == nil {
		return "", 0
	}
	n, err := strconv.Atoi(cmp.Or(m[1], m[2]))
	if err != nil || n > len(pos) || pos[n-1] == "" || quoteWord(pos[n-1]) != pos[n-1] {
		return "", 0
	}
	return pos[n-1], len(m[0])
}

// add adds one body of a recipe: its text, the line a denial shows, and
// the script language just's shell runs it in (buildBody.lang).
func (r *justRun) add(rec *justRecipe, line int, dir, text, shownLine, lang string) {
	shown := shownPath(rec.file, r.invoked)
	r.run.bodies = append(r.run.bodies, buildBody{
		what: bodyWhat("just recipe", rec.name, shown+":"+strconv.Itoa(line), shownLine), text: text,
		dir: dir, file: rec.file, lang: lang,
	})
}

// interpolate fills a recipe line's {{ }} with the values belt can read,
// and a placeholder for the rest. {{{{ is a literal {{.
func (r *justRun) interpolate(line string, vals map[string]string) string {
	const open = "\x00open\x00"
	line = strings.ReplaceAll(line, "{{{{", open)
	line = justInterp.ReplaceAllStringFunc(line, func(m string) string {
		v, _ := r.jf.eval(m[2:len(m)-2], vals)
		return v
	})
	return strings.ReplaceAll(line, open, "{{")
}

// justScriptEnd ends the heredoc a script recipe's body is fed through.
const justScriptEnd = "BELT_JUST_SCRIPT_END"

// scriptFeed writes a script recipe's body as the shell text that feeds it
// to its interpreter, so the walk reads it as that interpreter's script:
// a #! line names it, and [script] gives it.
func scriptFeed(interp string, lines []string) string {
	if strings.HasPrefix(lines[0], "#!") {
		lines = lines[1:]
	}
	words := strings.Fields(interp)
	if len(words) > 1 && filepath.Base(words[0]) == "env" {
		words = words[1:]
	}
	if len(words) == 0 {
		return ""
	}
	head := filepath.Base(words[0])
	return head + " - <<'" + justScriptEnd + "'\n" + strings.Join(lines, "\n") + "\n" + justScriptEnd + "\n"
}

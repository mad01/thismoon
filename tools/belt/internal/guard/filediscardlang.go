package guard

import (
	"bufio"
	"cmp"
	"io"
	"os"
	"path"
	"regexp"
	"slices"
	"strings"
)

// This file holds file-discard's reading of a recursive delete that a
// script in another language makes: Python's shutil.rmtree, Node's fs rm,
// rmSync, rmdir and rmdirSync with recursive set (fs.promises.rm among
// them), Ruby's FileUtils rm_rf, rm_r, rmtree, remove_dir, remove_entry and
// remove_entry_secure, and Perl's File::Path rmtree and remove_tree. The
// script is one the walk found (walkStep.scripts) or a file the command
// names. Each path such a call names takes the ladder an rm -r operand
// takes, from the shell's directory. A path belt cannot read from the
// script, such as a variable or a computed string, is unknown and denies,
// unless the script is committed code (discardJudge.unknown).

// langScript is one script an interpreter runs: its language
// (interpreterLang), what runs it, for a denial, and its text. ok is false
// when belt could not read all of it. file is the file its code was read
// from, for the trust rule ("" for the command's own text), and changed
// marks a file an earlier step of the command writes or edits, whose text
// on disk is not what runs.
type langScript struct {
	lang, runner, text string
	ok                 bool
	file               string
	changed            bool
}

// langDeleteCalls find the recursive delete calls of a script, per
// language, with the call's name as submatch 1. Python and Node calls need
// their parenthesis; Ruby and Perl may call without one.
var langDeleteCalls = map[string]*regexp.Regexp{
	"python": regexp.MustCompile(`\b(rmtree)\s*\(`),
	"node":   regexp.MustCompile(`\b(rmSync|rmdirSync|rm|rmdir)\s*\(`),
	"ruby": regexp.MustCompile(
		`\b(rm_rf|rm_r|rmtree|remove_dir|remove_entry_secure|remove_entry)\b`,
	),
	"perl": regexp.MustCompile(`\b(rmtree|remove_tree)\b`),
}

// langDeleteText matches text that names one of those calls in any
// language, for a script belt cannot read.
var langDeleteText = regexp.MustCompile(
	`\b(?:rmtree|remove_tree|rm_rf|rm_r|remove_dir|remove_entry(?:_secure)?|rmSync|rmdirSync)\b|` +
		`\b(?:rm|rmdir)\s*\([^)]*\brecursive\b`,
)

// langChdir matches a call that changes a script's directory, after which
// a relative path names a folder belt cannot tell.
var langChdir = regexp.MustCompile(`\bf?chdir\b|\bFileUtils\s*(?:\.|::)\s*cd\b`)

// scriptDeletes judges the recursive deletes the scripts of a step make
// (stepScripts), first loss wins.
func (j *discardJudge) scriptDeletes(s walkStep) *fileLoss {
	for _, sc := range j.stepScripts(s) {
		if loss := j.scriptLoss(sc, s); loss != nil {
			return loss
		}
	}
	return nil
}

// stepScripts returns the scripts a step runs: the ones the walk found,
// else the file it names (fileScript). A script the command appends to a
// file on disk is code of that file (feed.file).
func (j *discardJudge) stepScripts(s walkStep) []langScript {
	out := make([]langScript, 0, len(s.scripts))
	for _, f := range s.scripts {
		file := cmp.Or(f.file, j.codeFile)
		out = append(out, langScript{lang: f.lang, runner: f.runner, text: f.text, ok: f.ok, file: file})
	}
	if len(out) == 0 {
		if sc, ok := j.fileScript(s); ok {
			out = append(out, sc)
		}
	}
	return out
}

// fileScript returns the script file a step runs, read from disk: an
// interpreter's operand (`python3 x.py`), or a command word that names a
// file with a #! line (`./x.py`). Node's -p and --print take the operand
// as the script itself. A file belt cannot place or read adds nothing.
func (j *discardJudge) fileScript(s walkStep) (langScript, bool) {
	at := runAt(s.tokens)
	if at >= len(s.tokens) || len(s.words) != len(s.tokens) {
		return langScript{}, false
	}
	head := s.tokens[at]
	r, ok := readsScript(shellCmd{}, s.tokens)
	switch {
	case ok && !r.shell && r.operand > at:
		lang, w := interpreterLang(r.name), s.words[r.operand].shWord
		if lang == "node" && slices.ContainsFunc(s.tokens[at+1:r.operand], nodePrintFlag) {
			v, lit := w.literal()
			return langScript{lang: lang, runner: "the `" + head + "` script", text: v, ok: lit,
				file: j.codeFile}, true
		}
		return j.readScript(lang, w, s)
	case !ok && strings.Contains(head, "/"):
		return j.readScript("", s.words[at].shWord, s)
	}
	return langScript{}, false
}

// consumedScripts judges the scripts the command a consumer runs makes
// its recursive deletes in (scriptDeletes): its inline script, or the file
// it names. The inputs are the script's arguments, so a path it reads from
// them is unknown. Under find's -execdir a relative path is too, since it
// starts in the folder of each match.
func (j *discardJudge) consumedScripts(r consumerRun, s walkStep) *fileLoss {
	tokens := consumerTokens(r.command)
	words := make([]placedWord, len(r.command))
	for i, w := range r.command {
		words[i] = placedWord{shWord: w}
	}
	cs := walkStep{tokens: tokens, words: words, cwd: s.cwd, cwdSure: s.cwdSure, pwd: s.pwd}
	if slices.ContainsFunc(r.feeds, func(f *pathFeed) bool { return f.inMatchDir }) {
		cs.cwd, cs.pwd = "", ""
	}
	if at := interpreterScriptAt(tokens); at >= 0 {
		head := tokens[runCommandAt(tokens)]
		v, lit := r.command[at].literal()
		cs.scripts = []feed{{
			runner: "the `" + head + "` script " + r.who + " runs", text: v, ok: lit,
			interp: true, lang: interpreterLang(head),
		}}
	}
	return j.scriptDeletes(cs)
}

// nodePrintFlag reports whether a word is node's -p or --print, which
// runs the next word as code.
func nodePrintFlag(w string) bool { return w == "-p" || w == "--print" }

// readScript reads the script file a word names, from the shell's
// directory, in lang, or in the language of its #! line when lang is "".
// Only a language with recursive deletes (langDeleteCalls) is read, and
// past maxScriptSize the script is unread.
func (j *discardJudge) readScript(lang string, w shWord, s walkStep) (langScript, bool) {
	p, glob, ok := j.scan.staticPath(w)
	switch {
	case !ok || glob:
		return langScript{}, false
	case !path.IsAbs(p) && s.cwd == "":
		return langScript{}, false
	case !path.IsAbs(p):
		p = path.Join(s.cwd, p)
	}
	if text, state := j.newBody(p); state == bodyWritten {
		return writtenScript(lang, w, p, text)
	}
	// A FIFO would block the open, so only a regular file is opened.
	if fi, err := os.Stat(p); err != nil || !fi.Mode().IsRegular() {
		return langScript{}, false
	}
	f, err := os.Open(p)
	if err != nil {
		return langScript{}, false
	}
	defer f.Close()
	r := bufio.NewReader(io.LimitReader(f, maxScriptSize+1))
	if lang == "" {
		head, _ := r.Peek(shebangPeek)
		lang, _ = shebangInterpreter(string(head))
	}
	if _, known := langDeleteCalls[lang]; !known {
		return langScript{}, false
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return langScript{}, false
	}
	sc := langScript{lang: lang, runner: "the script `" + w.raw + "`", text: string(b), ok: true, file: p}
	if len(b) > maxScriptSize {
		sc.text, sc.ok = sc.text[:maxScriptSize], false
	}
	if by, touched := j.touched(p); touched {
		sc.text, sc.ok, sc.changed = sc.text+"\n"+by, false, true
	}
	return sc, true
}

// writtenScript is the script file at p as a step before this one wrote it
// (discardJudge.newBody), as with a cp, in lang or its #! line's.
func writtenScript(lang string, w shWord, p, text string) (langScript, bool) {
	if lang == "" {
		lang, _ = shebangInterpreter(text)
	}
	if _, known := langDeleteCalls[lang]; !known {
		return langScript{}, false
	}
	sc := langScript{lang: lang, runner: "the script `" + w.raw + "`", text: text, ok: true, file: p}
	if len(text) > maxScriptSize {
		sc.text, sc.ok = text[:maxScriptSize], false
	}
	return sc, true
}

// shebangPeek is how much of a file readScript looks at for its #! line.
const shebangPeek = 512

// scriptLoss judges one script, as code read from its file (inFile). One
// belt could not read denies when the text that builds it names a
// recursive delete (langDeleteText): the step's words, the pipeline in
// front of it, its heredoc bodies and what belt read of the script, with
// the text of the steps that changed it. One it read is judged call by
// call.
func (j *discardJudge) scriptLoss(sc langScript, s walkStep) *fileLoss {
	if _, ok := langDeleteCalls[sc.lang]; !ok {
		return nil
	}
	return j.inFile(sc.file, func() *fileLoss {
		if !sc.ok {
			return j.unreadScript(sc, s)
		}
		text := dropCommentLines(sc.text)
		moves := langChdir.MatchString(text)
		for _, c := range langCalls(sc.lang, text) {
			if loss := j.langCall(sc, c, moves, s); loss != nil {
				return loss
			}
		}
		return nil
	})
}

// unreadScript is the loss for a script belt could not read in full, when
// the text that builds it names a recursive delete (scriptLoss).
func (j *discardJudge) unreadScript(sc langScript, s walkStep) *fileLoss {
	what := "belt cannot read all of " + sc.runner
	if sc.changed {
		what = "this command changes " + sc.runner + " before it runs it, so belt cannot read what runs"
	}
	for _, text := range []string{strings.Join(s.tokens, " "), s.pipeText, s.docText, sc.text} {
		if m := langDeleteText.FindString(text); m != "" {
			return j.unknown(&fileLoss{why: what + ", and its text names `" + m +
				"`, a recursive delete, so it cannot tell what that deletes"})
		}
	}
	return nil
}

// langCall is one recursive delete call in a script: its name, the paths
// it names, and known false when belt cannot read each of them.
type langCall struct {
	name  string
	paths []string
	known bool
}

// langCall judges the paths one call deletes, each as an rm -r operand
// written as a literal, from the shell's directory. A relative path in a
// script that changes its directory is unknown.
func (j *discardJudge) langCall(sc langScript, c langCall, moves bool, s walkStep) *fileLoss {
	where := "the `" + c.name + "` call in " + sc.runner
	call := &lossCall{}
	if !c.known {
		return j.unknown(&fileLoss{call: call, why: "belt cannot tell which path " + where + " deletes"})
	}
	for _, p := range c.paths {
		if moves && !path.IsAbs(p) {
			if loss := j.unknown(&fileLoss{call: call, why: where + " deletes `" + p + "` from the folder " +
				"the script changes to, so belt cannot tell which path that is"}); loss != nil {
				return loss
			}
		}
		if p != "" {
			call.targets = append(call.targets, quoteTarget(p))
		}
	}
	for _, p := range c.paths {
		if p == "" || moves && !path.IsAbs(p) {
			continue
		}
		if loss := j.operand(call, lossOperand{word: literalWord(p), tree: true}, s); loss != nil {
			return loss
		}
	}
	return nil
}

// quoteTarget writes a path as one shell word for the toss-bin form.
func quoteTarget(p string) string {
	const plain = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./+,:@%="
	if strings.Trim(p, plain) == "" {
		return p
	}
	return "'" + p + "'"
}

// dropCommentLines drops the lines of a script that hold only a comment:
// # in Python, Ruby and Perl, // in Node.
func dropCommentLines(text string) string {
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, l := range lines {
		if t := strings.TrimSpace(l); !strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "//") {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

// langAliases find the names a Python or Node script imports or assigns a
// recursive delete under (`from shutil import rmtree as zap`, `{ rmSync:
// zap }`, `zap = shutil.rmtree`, `const zap = fs.rmSync`), as submatch 1
// or 2.
var langAliases = map[string]*regexp.Regexp{
	"python": regexp.MustCompile(
		`\brmtree\s+as\s+([A-Za-z_]\w*)|\b([A-Za-z_]\w*)\s*=\s*(?:[A-Za-z_]\w*\s*\.\s*)*rmtree\b`,
	),
	"node": regexp.MustCompile(
		`\b(?:rmSync|rmdirSync|rm|rmdir)\s*(?::|\s+as\s)\s*([A-Za-z_$][\w$]*)|` +
			`([A-Za-z_$][\w$]*)\s*=\s*(?:[A-Za-z_$][\w$]*\s*\.\s*)*(?:rmSync|rmdirSync|rm|rmdir)\b`,
	),
}

// langCalls finds the recursive delete calls of a script in lang, by
// name and by each alias the script imports it under (langAliases).
func langCalls(lang, text string) []langCall {
	out := findCalls(lang, text, langDeleteCalls[lang])
	if re, ok := langAliases[lang]; ok {
		for _, m := range re.FindAllStringSubmatch(text, -1) {
			call := regexp.MustCompile(`\b(` + regexp.QuoteMeta(cmp.Or(m[1], m[2])) + `)\s*\(`)
			out = append(out, findCalls(lang, text, call)...)
		}
	}
	return out
}

// findCalls finds the calls one pattern matches in a script in lang. A
// Node rm or rmdir counts only with its recursive option set
// (nodeRecursive). A Ruby symbol and a Perl import name a call without
// making it (namesOnly).
func findCalls(lang, text string, calls *regexp.Regexp) []langCall {
	var out []langCall
	for _, m := range calls.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[2]:m[3]]
		if namesOnly(lang, text, m[2]) {
			continue
		}
		args, ok := langArgs(lang, text, m[1])
		switch {
		case !ok:
			out = append(out, langCall{name: name})
		case lang == "node" && !nodeRecursive(args):
		default:
			paths, known := langPaths(lang, args)
			out = append(out, langCall{name: name, paths: paths, known: known})
		}
	}
	return out
}

// perlImport matches the start of a Perl statement that imports names.
var perlImport = regexp.MustCompile(`^\s*(?:use|no|require)\b`)

// namesOnly reports whether a call name at text[at] is no call: a Ruby
// symbol (`:rm_rf`, not `FileUtils::rm_rf`), or a name in a Perl use, no
// or require statement.
func namesOnly(lang, text string, at int) bool {
	switch lang {
	case "ruby":
		return at > 0 && text[at-1] == ':' && (at < 2 || text[at-2] != ':')
	case "perl":
		start := strings.LastIndexAny(text[:at], ";{}") + 1
		return perlImport.MatchString(text[start:at])
	}
	return false
}

// langArgs reads a call's arguments from text at i: just past the opening
// parenthesis in Python and Node, just past the name in Ruby and Perl,
// where the list may stand in parentheses or bare to the end of the
// statement. A bare list ends before a statement modifier (`if`, `or`).
func langArgs(lang, text string, i int) ([]string, bool) {
	if lang == "python" || lang == "node" {
		return splitArgs(text, i, true)
	}
	rest := strings.TrimLeft(text[i:], " \t")
	i = len(text) - len(rest)
	if strings.HasPrefix(rest, "(") {
		return splitArgs(text, i+1, true)
	}
	args, ok := splitArgs(text, i, false)
	if ok && len(args) > 0 {
		last := len(args) - 1
		args[last] = strings.TrimSpace(statementModifier.ReplaceAllString(args[last], ""))
	}
	return args, ok
}

// statementModifier is a Ruby or Perl statement modifier and all after it.
var statementModifier = regexp.MustCompile(`(?s)\s(?:if|unless|or|and|\|\||&&)(?:\s.*)?$`)

// splitArgs splits a call's arguments, from text[i:], at the commas
// outside quotes and brackets. paren marks a list that ends at its closing
// parenthesis; a bare list ends at a newline, a ; or a closing bracket it
// did not open. ok is false when a quote or a parenthesized list runs off
// the end.
func splitArgs(text string, i int, paren bool) (args []string, ok bool) {
	depth, start := 0, i
	add := func(end int) {
		if a := strings.TrimSpace(text[start:end]); a != "" {
			args = append(args, a)
		}
	}
	for ; i < len(text); i++ {
		switch c := text[i]; {
		case c == '\'' || c == '"' || c == '`':
			if i = quoteEnd(text, i); i < 0 {
				return nil, false
			}
		case strings.IndexByte("([{", c) >= 0:
			depth++
		case strings.IndexByte(")]}", c) >= 0 && depth > 0:
			depth--
		case strings.IndexByte(")]}", c) >= 0,
			!paren && depth == 0 && (c == ';' || c == '\n'):
			add(i)
			return args, true
		case c == ',' && depth == 0:
			add(i)
			start = i + 1
		}
	}
	if paren {
		return nil, false
	}
	add(len(text))
	return args, true
}

// quoteEnd returns the index of the quote that closes the one at text[i],
// past backslash escapes, or -1.
func quoteEnd(text string, i int) int {
	for k := i + 1; k < len(text); k++ {
		switch text[k] {
		case '\\':
			k++
		case text[i]:
			return k
		}
	}
	return -1
}

// jsName is a JavaScript variable or property path, as options belt
// cannot read.
var jsName = regexp.MustCompile(`^[A-Za-z_$][\w$]*(?:\.[A-Za-z_$][\w$]*)*$`)

// nodeRecursiveOpt is the recursive option in a Node options object, with
// its value as submatch 1 ("" for the shorthand `{ recursive }`).
var nodeRecursiveOpt = regexp.MustCompile(`["']?\brecursive\b["']?\s*(?::\s*([^,}\s]+))?`)

// nodeRecursive reports whether the options of a Node rm or rmdir call
// make it recursive: recursive set to anything but false, 0, null or
// undefined, or options belt cannot read (a variable, a spread).
func nodeRecursive(args []string) bool {
	if len(args) < 2 {
		return false
	}
	opts := args[1]
	switch {
	case jsName.MatchString(opts):
		return true
	case !strings.HasPrefix(opts, "{"):
		return false
	case strings.Contains(opts, "..."):
		return true
	}
	m := nodeRecursiveOpt.FindStringSubmatch(opts)
	return m != nil && !jsFalsy[m[1]]
}

// jsFalsy are the option values that leave recursive off.
var jsFalsy = wordSet("false 0 null undefined")

// perlNumber is a bare number, as rmtree's old verbose and safe flags.
var perlNumber = regexp.MustCompile(`^[0-9]+$`)

// langPaths reads the paths a call's arguments name, each a plain string
// literal (langLiteral) or, in Ruby and Perl, a bracketed list of them.
// Python, Node and Ruby name their first argument. Perl names each but a
// trailing option hash and a bare number. Python's dir_fd makes the path
// relative to a folder belt cannot see.
func langPaths(lang string, args []string) ([]string, bool) {
	if len(args) == 0 {
		return nil, false
	}
	switch lang {
	case "perl":
		var out []string
		for _, a := range args {
			if strings.HasPrefix(a, "{") || perlNumber.MatchString(a) {
				continue
			}
			ps, ok := literalList(lang, a)
			if !ok {
				return nil, false
			}
			out = append(out, ps...)
		}
		return out, true
	case "python":
		if slices.ContainsFunc(args, func(a string) bool { return strings.HasPrefix(a, "dir_fd") }) {
			return nil, false
		}
		return literalList(lang, pyPathKeyword.ReplaceAllString(args[0], ""))
	}
	return literalList(lang, args[0])
}

// pyPathKeyword is rmtree's path keyword in front of its value.
var pyPathKeyword = regexp.MustCompile(`^path\s*=\s*`)

// literalList reads one argument as the paths it names: a literal, or in
// Ruby and Perl a bracketed list of literals.
func literalList(lang, arg string) ([]string, bool) {
	if (lang == "ruby" || lang == "perl") && strings.HasPrefix(arg, "[") {
		items, ok := splitArgs(arg, 1, true)
		if !ok {
			return nil, false
		}
		out := make([]string, 0, len(items))
		for _, item := range items {
			v, ok := langLiteral(lang, item)
			if !ok {
				return nil, false
			}
			out = append(out, v)
		}
		return out, true
	}
	v, ok := langLiteral(lang, arg)
	return []string{v}, ok
}

// pyPathCall is a pathlib Path made from one argument.
var pyPathCall = regexp.MustCompile(`^(?:pathlib\s*\.\s*)?Path\s*\(\s*(.*?)\s*\)$`)

// langLiteral reads one argument as a plain string literal in lang:
// quoted text with no escape, no quote, and no $ or backtick, which a
// shell or the language may fill in. Python may prefix r, u or b, or f
// with no brace in the text, and may make a Path of the literal. Node may
// quote with backticks. Ruby's and Perl's double quotes may not hold #{ or
// @.
func langLiteral(lang, arg string) (string, bool) {
	if lang == "python" {
		if m := pyPathCall.FindStringSubmatch(arg); m != nil {
			arg = m[1]
		}
		prefix := arg[:len(arg)-len(strings.TrimLeft(arg, "rRuUbBfF"))]
		arg = arg[len(prefix):]
		if len(prefix) > 2 || strings.ContainsAny(prefix, "fF") && strings.ContainsAny(arg, "{}") {
			return "", false
		}
	}
	if len(arg) < 2 || arg[0] != arg[len(arg)-1] {
		return "", false
	}
	q, body := arg[0], arg[1:len(arg)-1]
	switch {
	case q != '\'' && q != '"' && (q != '`' || lang != "node"),
		strings.ContainsAny(body, "\\$`'\""),
		q == '"' && lang == "ruby" && strings.Contains(body, "#{"),
		q == '"' && lang == "perl" && strings.Contains(body, "@"):
		return "", false
	}
	return body, true
}

// namesScriptDelete reports whether the tokens of a step belt could not
// read run an interpreter of one of these languages and name one of their
// recursive deletes (langDeleteText).
func namesScriptDelete(tokens []string) bool {
	runs := slices.ContainsFunc(tokens, func(t string) bool {
		_, ok := langDeleteCalls[interpreterLang(t)]
		return ok
	})
	return runs && langDeleteText.MatchString(strings.Join(tokens, " "))
}

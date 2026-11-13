package guard

import (
	"cmp"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"
)

// walkMemory is what one walk learns from the commands it has met that a
// later command may run as a script: the git aliases the command names or
// sets, the variables it sets to text the walk can read, and the files it
// writes. Every walk of one command shares it, nested ones included. A
// config write or a file outlives a subshell, and a variable that seems set
// where the shell would not keep it only makes the walk read more.
type walkMemory struct {
	aliasNames  map[string]bool     // names the command mentions as alias.<name>
	aliasBodies map[string][]script // bodies git config writes set, by name
	vars        map[string]string   // variables set to text read whole
	files       map[string]script   // files written, by fileKeys
	budget      int                 // derived texts the walk may still read
}

// maxDerivedTexts bounds how many texts one command's walk reads that it
// built itself: alias bodies, printed text, a variable's or a file's
// content. Each may hold more of them, so a crafted command could otherwise
// multiply the walk. Past the bound the text counts as unread.
const maxDerivedTexts = 64

// newWalkMemory starts the memory of one command's walk, with the alias
// names its texts mention.
func newWalkMemory(texts ...string) *walkMemory {
	m := &walkMemory{
		aliasNames:  map[string]bool{},
		aliasBodies: map[string][]script{},
		vars:        map[string]string{},
		files:       map[string]script{},
		budget:      maxDerivedTexts,
	}
	for _, t := range texts {
		m.noteAliasNames(t)
	}
	return m
}

// script is text a shell or an interpreter may run; ok is false when the
// walk could not read it.
type script struct {
	text string
	ok   bool
}

// feed is text a command hands to a shell or an interpreter to run, beyond
// what walkNested and the -c and eval walks read: a git alias body, text
// printed into a shell, a file the command wrote, a variable it set.
type feed struct {
	runner string // what runs the text, for a denial
	text   string
	ok     bool
	interp bool     // an interpreter runs it: only a push named in it counts
	source bool     // source, . or eval runs it in this shell
	alias  string   // the git alias it is a body of, or whose body is unread
	dir    string   // the directory it runs in
	pre    []string // the words in front of what runs it
}

// feedsOf returns the feeds of one command: what a git alias runs, what a
// shell or an interpreter reads as its script (readsScript), a script file
// the command wrote and runs by path, and the -c, eval and interpreter
// scripts whose substitutions print text the walk can read (inlineFeeds).
func (w *shellWalk) feedsOf(
	text string,
	c shellCmd,
	tokens []string,
	in pipeIn,
	dir string,
) []feed {
	var out []feed
	if call, ok := readGitCall(tokens, c.words); ok {
		gitDir := dir
		if g, ok := segmentGitCommand(tokens, dir); ok {
			gitDir = g.dir
		}
		out = append(out, w.aliasFeeds(call, gitDir, dir)...)
	}
	if r, ok := readsScript(c, tokens); ok {
		out = append(out, w.runnerFeeds(text, c, in.from, r, dir)...)
	}
	if head := len(tokens) - len(stripWrappers(tokens)); head < len(tokens) &&
		strings.Contains(tokens[head], "/") {
		if s, ok := w.mem.file(dir, tokens[head]); ok {
			out = append(out, feed{
				runner: fmt.Sprintf("the script %s", tokens[head]),
				text:   s.text, ok: s.ok, interp: interpreterShebang(s.text), dir: dir,
			})
		}
	}
	return append(out, w.inlineFeeds(text, c, tokens, dir)...)
}

// opacity is what a command runs that the walk could not read
// (walkStep.opaque). textOnly marks a command whose only such text is shell
// text a shell reads (feed.alias empty, not an interpreter's), which the
// push guard denies only in a repo it guards (GitPushMain.opaqueDenial).
type opacity struct {
	what     string
	textOnly bool
}

// claim decides what the walk does with a command's feeds: it reads every
// shell text, and a shell text it cannot read, an alias body it cannot
// read, or an interpreter script that names a git push makes the command
// opaque. An interpreter script the walk cannot read counts as one on disk
// does: the walk never reads the language, so it has nothing to look in.
// Past maxDerivedTexts the walk reads none of them and the command is
// opaque. A cause other than unread shell text names the opacity first.
func (m *walkMemory) claim(feeds []feed) (opacity, bool) {
	var text, other string
	walkable := 0
	for _, f := range feeds {
		switch {
		case f.interp && f.ok && mentionsPush(f.text):
			other = cmp.Or(other, f.runner+", which names a git push")
		case f.interp:
		case !f.ok && f.alias == "":
			text = cmp.Or(text, f.runner)
		case !f.ok:
			other = cmp.Or(other, f.runner)
		default:
			walkable++
		}
	}
	walk := walkable <= m.budget
	if walk {
		m.budget -= walkable
	} else {
		other = cmp.Or(other, "the text this command builds and runs, past what belt reads in one command")
	}
	return opacity{what: cmp.Or(other, text), textOnly: other == "" && text != ""}, walk
}

// walkFeeds walks the shell texts of a command's feeds as code: in a child
// shell, or in this one for source and eval. An alias body is read with
// the alias marked, so the walk never reads it inside itself, and with its
// config counted as overridden, since git hands its -c options on to every
// git the body runs.
func (w *shellWalk) walkFeeds(feeds []feed, fn func(walkStep)) {
	for _, f := range feeds {
		if !f.ok || f.interp {
			continue
		}
		w.mem.noteAliasNames(f.text)
		if f.source {
			w.walkInShell(f.pre, f.text, fn)
			continue
		}
		inner := w.childFor(f.dir, f.pre)
		if f.alias != "" {
			inner.configEnv = true
			inner.expanding = append(slices.Clone(w.expanding), f.alias)
		}
		inner.walkRead(f.text, true, fn)
	}
}

// runner says how a command runs a script it reads from an operand or from
// standard input.
type runner struct {
	name    string // the shell or interpreter as written, for a denial
	shell   bool   // a shell, whose script the walk reads as code
	source  bool   // source or ., which run it in this shell
	operand int    // the script operand's index in the tokens, -1 for standard input
}

// readsScript reports whether a command runs a script from a file operand or
// from standard input: a shell (shellInterpreters, `busybox sh`, $SHELL) or
// an interpreter (isInterpreter) with no inline script and no -m module, or
// source or . with an operand. /dev/stdin and its siblings, - and a shell's
// -s name standard input.
func readsScript(c shellCmd, tokens []string) (runner, bool) {
	i := len(tokens) - len(stripWrappers(tokens))
	if i >= len(tokens) {
		return runner{}, false
	}
	head := filepath.Base(tokens[i])
	switch {
	case head == "source" || head == ".":
		if i+1 >= len(tokens) {
			return runner{}, false
		}
		r := runner{name: head, shell: true, source: true, operand: i + 1}
		if stdinPaths[tokens[i+1]] {
			r.operand = -1
		}
		return r, true
	case head == "busybox" && i+1 < len(tokens) && shellInterpreters[filepath.Base(tokens[i+1])]:
		i++
		head = filepath.Base(tokens[i])
	case shellVarWords[tokens[i]]:
		head = "sh"
	}
	flags, ok := isInterpreter(head)
	if !ok {
		return runner{}, false
	}
	r := runner{name: tokens[i], shell: shellInterpreters[head]}
	operand, ok := scriptOperand(tokens[i+1:], flags, r.shell)
	if !ok {
		return runner{}, false
	}
	r.operand = operand
	if operand >= 0 {
		r.operand = i + 1 + operand
	}
	return r, true
}

// shellVarWords are the words that run the user's shell.
var shellVarWords = wordSet(`$SHELL ${SHELL} "$SHELL" "${SHELL}"`)

// stdinPaths are the operands that name standard input.
var stdinPaths = wordSet("- /dev/stdin /dev/fd/0 /proc/self/fd/0")

// scriptOperand returns the index in args of the script operand, -1 when the
// script comes from standard input, and ok false when the command runs an
// inline script or a module instead.
func scriptOperand(args []string, inlineFlags map[string]bool, shell bool) (int, bool) {
	for j := 0; j < len(args); j++ {
		switch a := args[j]; {
		case inlineFlags[a], shell && shellCFlag.MatchString(a), !shell && a == "-m":
			return 0, false
		case shell && (a == "-o" || a == "+o"):
			j++
		case shell && a == "-s", stdinPaths[a]:
			return -1, true
		case a == "--":
			if j+1 < len(args) && !stdinPaths[args[j+1]] {
				return j + 1, true
			}
			return -1, true
		case strings.HasPrefix(a, "-"), shell && strings.HasPrefix(a, "+"):
		default:
			return j, true
		}
	}
	return -1, true
}

// runnerFeeds returns the script a runner reads: its operand, a here-string,
// a < redirection, a heredoc an interpreter reads, or what the pipeline in
// front of it prints. A shell's heredoc is code already (docRuns), and so is
// a heredoc cat pipes into a shell.
func (w *shellWalk) runnerFeeds(
	text string,
	c shellCmd,
	from *shellCmd,
	r runner,
	dir string,
) []feed {
	base := feed{
		runner: fmt.Sprintf("the text `%s` runs", r.name),
		interp: !r.shell, source: r.source, dir: dir,
	}
	if base.interp {
		base.runner = fmt.Sprintf("the `%s` script", r.name)
	}
	if r.operand >= 0 {
		return w.operandFeeds(text, c.words[r.operand], base)
	}
	for _, t := range c.targets {
		switch redirectOp(text, t) {
		case "<<<":
			return []feed{base.with(w.strictText(text, t, true))}
		case "<":
			return w.operandFeeds(text, t, base)
		}
	}
	switch {
	case len(c.docs) > 0 && r.shell:
		return nil
	case len(c.docs) > 0:
		return []feed{base.with(docText(text, c))}
	case from == nil:
		return nil
	case r.shell && len(from.docs) > 0 && catLike[filepath.Base(firstWord(*from))]:
		return nil
	}
	s, known := w.produced(text, *from)
	if !known {
		s = script{}
	}
	return []feed{base.with(s)}
}

// with returns the feed carrying a script.
func (f feed) with(s script) feed {
	f.text, f.ok = s.text, s.ok
	return f
}

// operandFeeds returns the script a runner's operand or < target names: the
// output of a <( ) the walk can read (a printer's text), or a file the
// command wrote. A <( ) of any other command, as in `source <(tool init)`,
// and a file belt did not see written add nothing.
func (w *shellWalk) operandFeeds(text string, word placedWord, base feed) []feed {
	if body, ok := procSubstBody(text, word); ok {
		s, known := w.bodyProduced(body)
		if !known {
			return nil
		}
		return []feed{base.with(s)}
	}
	if s, ok := w.mem.file(base.dir, word.token()); ok {
		return []feed{base.with(s)}
	}
	return nil
}

// procSubstBody returns the text inside a word that is one <( ).
func procSubstBody(text string, word placedWord) (string, bool) {
	if len(word.parts) != 1 || !strings.HasPrefix(word.raw, "<(") {
		return "", false
	}
	body := word.parts[0].body
	return text[body.from:body.to], body.to > body.from
}

// inlineFeeds returns what a shell's -c script, an eval and an
// interpreter's -c or -e script run, when the walk reads more of it than
// placedWord.script does: the text a substitution there prints with a
// printer (echo, printf, cat of a heredoc), and a variable the command set
// to such text. The -c and eval walks still read the script as written, so
// this only adds steps. An interpreter's script is always a feed, for the
// push it may name.
func (w *shellWalk) inlineFeeds(text string, c shellCmd, tokens []string, dir string) []feed {
	var out []feed
	if at := inlineScriptAt(tokens); at >= 0 {
		if s, changed := w.scriptOf(text, c.words[at:at+1]); changed {
			f := feed{runner: "the script a shell's -c runs", dir: dir, pre: tokens[:at]}
			out = append(out, f.with(s))
		}
	}
	if at := evalAt(tokens); at >= 0 {
		if s, changed := w.scriptOf(text, c.words[at+1:]); changed {
			f := feed{runner: "the text eval runs", source: true, dir: dir, pre: tokens[:at]}
			out = append(out, f.with(s))
		}
	}
	if at := interpreterScriptAt(tokens); at >= 0 {
		s, _ := w.scriptOf(text, c.words[at:at+1])
		head := tokens[len(tokens)-len(stripWrappers(tokens))]
		f := feed{runner: fmt.Sprintf("the `%s` script", head), interp: true, dir: dir}
		out = append(out, f.with(s))
	}
	return out
}

// interpreterScriptAt returns the index of the inline script word of an
// interpreter that is no shell (python -c, perl -e, perl -ne), or -1.
func interpreterScriptAt(tokens []string) int {
	i := len(tokens) - len(stripWrappers(tokens))
	if i >= len(tokens) || shellInterpreters[filepath.Base(tokens[i])] {
		return -1
	}
	flags, ok := isInterpreter(filepath.Base(tokens[i]))
	if !ok {
		return -1
	}
	for j := i + 1; j < len(tokens); j++ {
		switch tok := tokens[j]; {
		case flags[tok] || inlineBundle.MatchString(tok):
			if j+1 < len(tokens) {
				return j + 1
			}
			return -1
		case !strings.HasPrefix(tok, "-"):
			return -1
		}
	}
	return -1
}

// inlineBundle is an interpreter option bundle that ends in its inline
// script flag (`perl -ne`, `python -Ic`).
var inlineBundle = regexp.MustCompile(`^-[A-Za-z]*[ceE]$`)

// scriptOf returns the text words run as a script, read the way scriptText
// reads each, and whether that reads more than placedWord.script does.
func (w *shellWalk) scriptOf(text string, words []placedWord) (script, bool) {
	s := script{ok: true}
	changed := false
	parts := make([]string, 0, len(words))
	for _, word := range words {
		t, more, ok := w.scriptText(text, word)
		parts = append(parts, t)
		changed = changed || more
		s.ok = s.ok && ok
	}
	s.text = strings.Join(parts, " ")
	return s, changed || !s.ok
}

// scriptText reads one word as script text. A substitution whose command
// prints text the walk can read (bodyProduced) is that text, and a variable
// the command set to read text is that text: both read more than script
// does. A printer whose text the walk cannot read leaves the word unread
// (ok false). Any other expansion stays as script writes it.
func (w *shellWalk) scriptText(text string, word placedWord) (out string, more, ok bool) {
	var b strings.Builder
	ok = true
	for _, p := range word.parts {
		switch p.kind {
		case partLit:
			b.WriteString(p.text)
		case partVar:
			v, set := w.mem.vars[p.text]
			if !set {
				v = "${" + p.text + "}"
			}
			b.WriteString(v)
			more = more || set
		case partSubst:
			s, known := w.bodyProduced(text[p.body.from:p.body.to])
			switch {
			case !known:
				b.WriteString("$?")
			case !s.ok:
				ok = false
			default:
				b.WriteString(s.text)
				more = true
			}
		default:
			b.WriteString("$?")
		}
	}
	return b.String(), more, ok
}

// strictText reads one word as printed text: literal text, a variable the
// command set to read text, and a substitution of a printer whose text the
// walk can read. Any other expansion leaves the text unread. trailing keeps
// the newline a here-string adds.
func (w *shellWalk) strictText(text string, word placedWord, trailing bool) script {
	var b strings.Builder
	for _, p := range word.parts {
		switch p.kind {
		case partLit:
			b.WriteString(p.text)
		case partVar:
			v, set := w.mem.vars[p.text]
			if !set {
				return script{}
			}
			b.WriteString(v)
		case partSubst:
			s, known := w.bodyProduced(text[p.body.from:p.body.to])
			if !known || !s.ok {
				return script{}
			}
			b.WriteString(s.text)
		default:
			return script{}
		}
	}
	if trailing {
		b.WriteString("\n")
	}
	return script{text: b.String(), ok: true}
}

// bodyProduced returns what the commands inside a substitution or a <( )
// print, when they are one printer, optionally piped through cat or tee
// (produced). known is false for any other command. The shell drops the
// trailing newlines of a $( ).
func (w *shellWalk) bodyProduced(body string) (script, bool) {
	cmds, ok := readShell(body)
	if !ok || len(cmds) == 0 {
		return script{}, false
	}
	j := len(cmds) - 1
	for j > 0 && cmds[j].before == jointPipe && passesStdin(cmds[j]) {
		j--
	}
	if j != 0 {
		return script{}, false
	}
	s, known := w.produced(body, cmds[0])
	s.text = strings.TrimRight(s.text, "\n")
	return s, known
}

// catLike are the commands that print a heredoc or here-string they read
// when they name no file.
var catLike = wordSet("cat tee head tail")

// firstWord returns a command's first word after wrappers, "" when none.
func firstWord(c shellCmd) string {
	if words := stripWrappers(c.tokens()); len(words) > 0 {
		return words[0]
	}
	return ""
}

// produced returns the text a printer prints: echo, printf without -v, or
// cat, tee, head or tail with no file operand printing a here-string or
// heredoc. known is false for any other command, or one with a ( ) in it.
// A printer word with an expansion the walk cannot read leaves the text
// unread (strictText).
func (w *shellWalk) produced(text string, p shellCmd) (script, bool) {
	tokens := p.tokens()
	i := len(tokens) - len(stripWrappers(tokens))
	if i >= len(tokens) || len(p.subshells) > 0 {
		return script{}, false
	}
	words := p.words[i+1:]
	switch head := filepath.Base(tokens[i]); {
	case head == "echo":
		return w.echoText(text, words), true
	case head == "printf" && printsWords(tokens[i:]):
		return w.printfText(text, words), true
	case catLike[head] && !slices.ContainsFunc(tokens[i+1:], func(t string) bool {
		return !strings.HasPrefix(t, "-")
	}):
		for _, t := range p.targets {
			if redirectOp(text, t) == "<<<" {
				return w.strictText(text, t, true), true
			}
		}
		if len(p.docs) > 0 {
			return docText(text, p), true
		}
	}
	return script{}, false
}

// docText returns a command's heredoc bodies as one text. A body with an
// unquoted delimiter and a $ or backtick in it holds text only the shell
// can expand, so it is unread.
func docText(text string, c shellCmd) script {
	s := script{ok: true}
	for _, d := range c.docs {
		body := text[d.body.from:d.body.to]
		s.ok = s.ok && (d.quoted || !strings.ContainsAny(body, "$`"))
		s.text += body
	}
	return s
}

// echoText returns what echo prints: its words after any -n, -e or -E,
// joined by spaces, with escapes read the way zsh's echo reads them.
func (w *shellWalk) echoText(text string, words []placedWord) script {
	for len(words) > 0 && echoFlag.MatchString(words[0].raw) {
		words = words[1:]
	}
	parts := make([]string, 0, len(words))
	for _, word := range words {
		s := w.strictText(text, word, false)
		if !s.ok {
			return script{}
		}
		parts = append(parts, s.text)
	}
	return script{text: unescape(strings.Join(parts, " ")) + "\n", ok: true}
}

// echoFlag is an echo option word.
var echoFlag = regexp.MustCompile(`^-[neE]+$`)

// printfText returns what printf prints: its format with each directive
// taking the next argument, run again while arguments remain, and escapes
// read in all of it, which reads at least as much as printf's %b.
func (w *shellWalk) printfText(text string, words []placedWord) script {
	if len(words) > 0 && words[0].raw == "--" {
		words = words[1:]
	}
	values := make([]string, 0, len(words))
	for _, word := range words {
		s := w.strictText(text, word, false)
		if !s.ok {
			return script{}
		}
		values = append(values, s.text)
	}
	if len(values) == 0 {
		return script{ok: true}
	}
	return script{text: unescape(formatPrintf(values[0], values[1:])), ok: true}
}

// maxPrintfPasses bounds how often formatPrintf runs the format again for
// arguments left over.
const maxPrintfPasses = 64

// formatPrintf fills a printf format's directives with args in order, as
// printf does, and runs the format again while arguments remain. A
// directive with no argument left prints nothing; %% prints %.
func formatPrintf(format string, args []string) string {
	var b strings.Builder
	for pass := 0; pass < maxPrintfPasses; pass++ {
		used := false
		for i := 0; i < len(format); i++ {
			if format[i] != '%' || i+1 == len(format) {
				b.WriteByte(format[i])
				continue
			}
			if format[i+1] == '%' {
				b.WriteByte('%')
				i++
				continue
			}
			for i+1 < len(format) && strings.IndexByte("-+ #0123456789.*", format[i+1]) >= 0 {
				i++
			}
			i++ // the conversion letter
			used = true
			if len(args) > 0 {
				b.WriteString(args[0])
				args = args[1:]
			}
		}
		if !used || len(args) == 0 {
			break
		}
	}
	return b.String()
}

// unescape reads the backslash escapes echo and printf print: \n and its
// line-breaking kin as a newline, \t as a tab, \xHH and \0NNN or \NNN as the
// byte they name, \\ as a backslash. A control escape with no text (\a, \b,
// \c, \e) is dropped, so text around it joins, which reads more words, not
// fewer. Any other escape stays as written.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch c := s[i]; {
		case strings.IndexByte("nrvf", c) >= 0:
			b.WriteByte('\n')
		case c == 't':
			b.WriteByte('\t')
		case c == '\\':
			b.WriteByte('\\')
		case strings.IndexByte("abce", c) >= 0:
		case c == 'x':
			n := escapeDigits(s[i+1:], 2, 16)
			v, _ := strconv.ParseUint(s[i+1:i+1+n], 16, 8)
			if n == 0 {
				b.WriteString(`\x`)
			} else {
				b.WriteByte(byte(v))
			}
			i += n
		case c >= '0' && c <= '7':
			from := i
			if c == '0' {
				from++
			}
			n := escapeDigits(s[from:], 3, 8)
			v, _ := strconv.ParseUint(s[from:from+n], 8, 8)
			b.WriteByte(byte(v))
			i = from + n - 1
		default:
			b.WriteByte('\\')
			b.WriteByte(c)
		}
	}
	return b.String()
}

// escapeDigits counts the digits of base at the start of s, at most max.
func escapeDigits(s string, maxDigits, base int) int {
	n := 0
	for n < len(s) && n < maxDigits {
		if _, err := strconv.ParseUint(s[n:n+1], base, 8); err != nil {
			break
		}
		n++
	}
	return n
}

// pushWindow is how many words after a git word mentionsPush looks for
// push: git's global options and their values come between them.
const pushWindow = 8

// mentionsPush reports whether an interpreter's script names a git push: a
// git word followed within pushWindow words by push, with quotes, commas,
// brackets and parentheses read as spaces, so a Python
// `subprocess.run(["git", "push"])` or a Perl `system("git push")` counts.
// The walk cannot read the language, so a script that only prints those
// words counts too.
func mentionsPush(text string) bool {
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune("_-./", r)
	})
	for i, word := range words {
		if isGitWord(word) && slices.Contains(words[i+1:min(len(words), i+1+pushWindow)], "push") {
			return true
		}
	}
	return false
}

// interpreterShebang reports whether a script's #! line names an
// interpreter that is no shell, so the walk only looks for a push in it.
func interpreterShebang(text string) bool {
	line, _, _ := strings.Cut(text, "\n")
	rest, ok := strings.CutPrefix(line, "#!")
	if !ok {
		return false
	}
	for _, word := range strings.Fields(rest) {
		name := filepath.Base(word)
		if _, isInterp := isInterpreter(name); isInterp {
			return !shellInterpreters[name]
		}
	}
	return false
}

// redirectOp returns the operator in front of a redirection target as
// written: <<<, <, >, >>, >|, &>, 2>, << and so on.
func redirectOp(text string, t placedWord) string {
	end := t.at.from
	for end > 0 && (text[end-1] == ' ' || text[end-1] == '\t') {
		end--
	}
	start := end
	for start > 0 && strings.IndexByte("<>|&-0123456789", text[start-1]) >= 0 {
		start--
	}
	return text[start:end]
}

// stdoutWrites are the redirection operators that send a command's output to
// a file.
var stdoutWrites = wordSet("> >> >| 1> 1>> 1>| &> &>>")

// fileKeys returns the keys a file is known by: the word as written, and the
// path it names from dir when that resolves.
func fileKeys(dir, word string) []string {
	keys := []string{"word:" + word}
	if p := resolveDir(dir, word); p != "" {
		keys = append(keys, "path:"+p)
	}
	return keys
}

// file returns what the command wrote to the file a word names in dir.
func (m *walkMemory) file(dir, word string) (script, bool) {
	for _, key := range fileKeys(dir, word) {
		if s, ok := m.files[key]; ok {
			return s, true
		}
	}
	return script{}, false
}

// write records what a command wrote to the file a word names in dir,
// after what an earlier write left there when appending.
func (m *walkMemory) write(dir, word string, s script, appending bool) {
	if old, ok := m.file(dir, word); ok && appending {
		s = script{text: old.text + s.text, ok: old.ok && s.ok}
	}
	for _, key := range fileKeys(dir, word) {
		m.files[key] = s
	}
}

// remember records what a command leaves for a later one to run: a git
// alias a git config write sets, a variable set to text the walk can read,
// and a file it writes with a redirection or tee. A file written by any
// command but a printer is unread, so running it is opaque.
func (w *shellWalk) remember(text string, c shellCmd, tokens []string, in pipeIn, dir string) {
	if call, ok := readGitCall(tokens, c.words); ok {
		w.mem.rememberAliasWrite(call)
	}
	w.rememberVars(text, c, tokens)
	out, _ := w.produced(text, c)
	for _, t := range c.targets {
		if op := redirectOp(text, t); stdoutWrites[op] {
			w.mem.write(dir, t.token(), out, strings.HasSuffix(op, ">>"))
		}
	}
	words := stripWrappers(tokens)
	if len(words) == 0 || filepath.Base(words[0]) != "tee" || in.from == nil {
		return
	}
	piped, known := w.produced(text, *in.from)
	if !known {
		piped = script{}
	}
	appending := slices.Contains(words[1:], "-a")
	for _, word := range words[1:] {
		if !strings.HasPrefix(word, "-") {
			w.mem.write(dir, word, piped, appending)
		}
	}
}

// varBuiltins set the variables their NAME=value words name.
var varBuiltins = wordSet("export local declare typeset readonly")

// rememberVars records the variables a command sets to text the walk can
// read whole (strictText): a command of assignments only, or one of
// varBuiltins. Any other value forgets the variable.
func (w *shellWalk) rememberVars(text string, c shellCmd, tokens []string) {
	i := 0
	switch {
	case len(tokens) > 0 && varBuiltins[tokens[0]]:
		for i = 1; i < len(tokens) && strings.HasPrefix(tokens[i], "-"); i++ {
		}
	case !slices.ContainsFunc(tokens, isAssignment):
		return
	case slices.ContainsFunc(tokens, func(t string) bool { return !isAssignment(t) }):
		return // NAME=value in front of a command sets it for that command alone
	}
	for _, word := range c.words[i:] {
		name, value, ok := splitAssignment(word)
		if !ok {
			continue
		}
		if s := w.strictText(text, value, false); s.ok && !strings.HasSuffix(name, "+") {
			w.mem.vars[name] = s.text
			continue
		}
		delete(w.mem.vars, strings.TrimSuffix(name, "+"))
	}
}

// splitAssignment splits a NAME=value word into the name and the value as a
// word of its own.
func splitAssignment(word placedWord) (string, placedWord, bool) {
	if len(word.parts) == 0 || word.parts[0].kind != partLit {
		return "", placedWord{}, false
	}
	name, rest, ok := strings.Cut(word.parts[0].text, "=")
	if !ok || !isShellName(strings.TrimSuffix(name, "+")) {
		return "", placedWord{}, false
	}
	value := placedWord{shWord: shWord{raw: word.raw}}
	if rest != "" {
		value.parts = append(value.parts, wordPart{kind: partLit, text: rest})
	}
	value.parts = append(value.parts, word.parts[1:]...)
	return name, value, true
}

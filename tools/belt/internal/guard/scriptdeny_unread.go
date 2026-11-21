package guard

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// This file denies code belt cannot read before it runs: a download or
// decoded text that a shell, eval or source runs, through a pipe
// (scriptdeny_pipe.go), a <( ) file, a $( ) script string, a here-string, a
// heredoc, a variable set to it or a file the same command writes it to
// (scriptdeny_saved.go). It is the rule `curl | sh` always had
// (checkProducer), read through readShell, so the same shapes count inside
// a -c string, an eval, a subshell, a substitution or the text a wrapper
// hands a shell. A <( ) or $( ) that only prints files with cat, head or
// tail is read instead: those files are scanned as a cat piped into an
// interpreter is. The same walk reads a name that hides a command
// (shellNames.hiddenRun) in nested code, where its words are read with
// their quotes as the shell reads them.

// decoder says which arguments make a command print decoded text: short
// option letters, alone or bundled (-d, -di), and whole words (--decode).
// always marks a command that decodes with no option.
type decoder struct {
	letters string
	words   map[string]bool
	always  bool
}

// decoders turn text belt cannot read into the text they print.
var decoders = map[string]decoder{
	"base64":   {letters: "dD", words: wordSet("--decode")},
	"base32":   {letters: "d", words: wordSet("--decode")},
	"basenc":   {letters: "d", words: wordSet("--decode")},
	"openssl":  {words: wordSet("-d -decrypt")},
	"xxd":      {letters: "r", words: wordSet("-revert")},
	"gzip":     {letters: "d", words: wordSet("--decompress --uncompress")},
	"bzip2":    {letters: "d", words: wordSet("--decompress")},
	"xz":       {letters: "d", words: wordSet("--decompress --uncompress")},
	"zstd":     {letters: "d", words: wordSet("--decompress --uncompress")},
	"uudecode": {always: true},
	"gunzip":   {always: true},
	"zcat":     {always: true},
	"gzcat":    {always: true},
	"bzcat":    {always: true},
	"bunzip2":  {always: true},
	"xzcat":    {always: true},
	"unxz":     {always: true},
	"zstdcat":  {always: true},
	"unzstd":   {always: true},
}

// stdinFiles are the file names that make an interpreter or source read its
// script from standard input.
var stdinFiles = wordSet("/dev/stdin /dev/fd/0")

// fdFile is a file name that opens one of the command's own descriptors,
// which its input redirections set (`bash /dev/fd/3 3< <(curl …)`).
var fdFile = regexp.MustCompile(`^/(?:dev|proc/self)/fd/[0-9]+$`)

// inputFile reports whether a script file name reads the command's
// standard input or another descriptor it redirects.
func inputFile(name string) bool {
	return stdinFiles[name] || fdFile.MatchString(name)
}

// jsRuntimes are the JavaScript runtimes the walk reads besides the
// interpreters isInterpreter names: each runs a file after a run subcommand
// or on its own (`deno run x.ts`, `bun x.ts`), and the words in its set run
// the next word as code.
var jsRuntimes = map[string]map[string]bool{
	"deno": wordSet("eval"),
	"bun":  wordSet("-e --eval -p --print"),
}

// scriptRunner returns the words that mark an inline script for a command
// that runs a script: an interpreter (isInterpreter) or a JavaScript
// runtime (jsRuntimes).
func scriptRunner(head string) (map[string]bool, bool) {
	if flags, ok := isInterpreter(head); ok {
		return flags, true
	}
	flags, ok := jsRuntimes[head]
	return flags, ok
}

// decodes reports whether a command, by name and arguments, prints decoded
// text.
func decodes(name string, args []string) bool {
	dec, ok := decoders[name]
	if !ok {
		return false
	}
	for _, a := range args {
		if dec.words[a] || (dec.letters != "" && shortBundle(a) && strings.ContainsAny(a[1:], dec.letters)) {
			return true
		}
	}
	return dec.always
}

// shortBundle reports whether a word is a bundle of short options: one dash,
// then ASCII letters only.
func shortBundle(word string) bool {
	if len(word) < 2 || word[0] != '-' {
		return false
	}
	for _, c := range word[1:] {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') {
			return false
		}
	}
	return true
}

// unreadableSource returns what a command prints that belt cannot read as
// code, a download or decoded text, or "" for any other command.
func unreadableSource(tokens []string) string {
	words, _ := unwrapCommand(tokens)
	if len(words) == 0 {
		return ""
	}
	switch name := commandName(words[0]); {
	case fetchProducers[name]:
		return "a " + name + " download"
	case decodes(name, words[1:]):
		return "text " + name + " decodes"
	}
	return ""
}

// unreadableDenial is the denial for code from src, which nothing can read.
func unreadableDenial(src string) *Denial {
	return Reasonf(
		ScriptDenyListID,
		"runs %s as code, so nothing can read what would run. "+
			"Save it to a file in one call and read it, then run the file in a later call, so the deny list (and the user) can check it before it executes.",
		src,
	)
}

// codeWalk reads every command a command text runs, nested ones included,
// for code the deny list cannot read as written. g and s are the guard and
// the scan whose scanTarget reads the files a readable <( ) or $( ) prints,
// and whose names a nested command is read through. held are the variables
// the text sets to a download or decoded text, by name, and saved the files
// it writes one to, by resolved path, each with what it holds. Both fill in
// as the walk meets the commands, in order, and like shellNames they read
// text, not shell scope.
type codeWalk struct {
	g     *ScriptDenyList
	s     scan
	held  map[string]string
	saved map[string]string
}

// newCodeWalk starts a walk with nothing held or saved yet.
func newCodeWalk(g *ScriptDenyList, s scan) codeWalk {
	return codeWalk{g: g, s: s, held: map[string]string{}, saved: map[string]string{}}
}

// run denies text that runs a download or decoded text as code, or a
// nested command a name hides. Text readShell cannot follow is left to the
// other checks; depth counts the nested texts read so far.
func (w codeWalk) run(text string, depth int) *Denial {
	if depth > maxShellNesting {
		return nil
	}
	cmds, ok := readShell(text)
	if !ok {
		return nil
	}
	for i := range cmds {
		if d := w.step(text, cmds, i, depth); d != nil {
			return d
		}
	}
	return nil
}

// step reads one command: the variables it sets, a pipe into its script or
// a >( ) that runs what it writes, a saved download it runs, its code
// words, the files it saves a download to, a name that hides it when it is
// nested (the top level is checkSegment's), and the code nested inside it.
// A command word that names an interpreter through a variable is read as
// that interpreter (shellNames.withRunner).
func (w codeWalk) step(text string, cmds []shellCmd, i, depth int) *Denial {
	c := cmds[i]
	tokens := c.tokens()
	words, kind := unwrapCommand(functionBody(text, c, tokens))
	at := len(tokens) - len(words)
	run := w.s.names.withRunner(words)
	w.noteHeld(text, c)
	if src := w.pipedSource(text, cmds, i, run); src != "" {
		return unreadableDenial(src)
	}
	if src := w.procSource(text, cmds, i, run); src != "" {
		return unreadableDenial(src)
	}
	if d := w.runsSaved(text, c, run); d != nil {
		return d
	}
	if d := w.readCode(text, c, run, at, kind); d != nil {
		return d
	}
	w.noteSaved(text, cmds, i, run)
	if depth > 0 {
		if d := w.s.names.hiddenRun(words, "the nested command", w.s.patterns, 0); d != nil {
			return d
		}
	}
	for _, inner := range nestedCode(text, c, slices.Concat(tokens[:at], run), run, at, kind) {
		if d := w.run(inner, depth+1); d != nil {
			return d
		}
	}
	return nil
}

// functionBody returns a function definition's tokens from the first word
// after its { on (definesFunction), so its body is read as the code it runs
// once called. The reader keeps a definition's () as a subshell, so the
// name and the { come first among its words. Any other command's tokens
// are returned whole.
func functionBody(text string, c shellCmd, tokens []string) []string {
	if !definesFunction(text, c, tokens) {
		return tokens
	}
	if i := slices.Index(tokens, "{"); i >= 0 {
		return tokens[i+1:]
	}
	return tokens
}

// readCode denies a command's code words (codeWords) that name a variable
// held to unreadable text (`eval "$X"`, `bash -c "$X"`), or hold a
// substitution that prints such text or only prints files that run a
// denied command.
func (w codeWalk) readCode(text string, c shellCmd, run []string, at int, kind wrapKind) *Denial {
	code, stdin := w.codeWords(text, c, run, at, kind)
	for _, word := range code {
		if src := w.heldIn(word); src != "" {
			return unreadableDenial(src)
		}
	}
	for _, body := range codeBodies(text, c, code, stdin) {
		if src := w.printsUnreadable(text[body.from:body.to]); src != "" {
			return unreadableDenial(src)
		}
		if d := w.readFiles(text[body.from:body.to]); d != nil {
			return d
		}
	}
	return nil
}

// noteHeld records the variables a command's assignment words set to a
// download or decoded text: a substitution that prints one (`X=$(curl …)`,
// `X="$(base64 -d f)"`), or a variable held already (`Y="$X"`). Any word
// in the shape counts (`export X=…`, `X=… bash -c "$X"`).
func (w codeWalk) noteHeld(text string, c shellCmd) {
	for _, word := range c.words {
		name, value, ok := word.assignment()
		if !ok {
			continue
		}
		for _, p := range value {
			if src := w.partSource(text, p); src != "" {
				w.held[name] = src
				break
			}
		}
	}
}

// partSource returns what one piece of a word holds that belt cannot read:
// the output of a substitution that prints it, or a held variable's value.
func (w codeWalk) partSource(text string, p wordPart) string {
	switch p.kind {
	case partSubst:
		return w.printsUnreadable(text[p.body.from:p.body.to])
	case partVar:
		return w.held[p.text]
	}
	return ""
}

// heldIn returns what a held variable a word expands holds, or "".
func (w codeWalk) heldIn(word placedWord) string {
	for _, p := range word.parts {
		if p.kind == partVar && w.held[p.text] != "" {
			return w.held[p.text]
		}
	}
	return ""
}

// readFiles scans the files a substitution body only prints with cat, head
// or tail, as checkProducer scans the files of a cat piped into an
// interpreter.
func (w codeWalk) readFiles(body string) *Denial {
	cmds, ok := readShell(body)
	if !ok || len(cmds) != 1 {
		return nil
	}
	tokens := stripWrappers(cmds[0].tokens())
	if len(tokens) == 0 || !fileProducers[filepath.Base(tokens[0])] {
		return nil
	}
	for _, arg := range tokens[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if d := w.g.scanTarget(w.s, arg); d != nil {
			return d
		}
	}
	return nil
}

// stageSource returns what one command prints that belt cannot read: it
// fetches or decodes, prints a held variable or a saved file, or a
// substitution or subshell in it prints something that does (`echo "$(curl
// …)"`, `cat <(curl …)`).
func (w codeWalk) stageSource(text string, c shellCmd) string {
	tokens := c.tokens()
	if src := unreadableSource(tokens); src != "" {
		return src
	}
	if src := w.printsSaved(tokens); src != "" {
		return src
	}
	for _, word := range c.words {
		if src := w.heldIn(word); src != "" {
			return src
		}
		for _, p := range substBodies(text, word) {
			if src := w.printsUnreadable(text[p.from:p.to]); src != "" {
				return src
			}
		}
	}
	for _, s := range c.subshells {
		if src := w.printsUnreadable(text[s.from:s.to]); src != "" {
			return src
		}
	}
	return ""
}

// printsUnreadable returns what a substitution body prints that belt cannot
// read as code, from any of its commands, or "".
func (w codeWalk) printsUnreadable(body string) string {
	cmds, ok := readShell(body)
	if !ok {
		return ""
	}
	for _, c := range cmds {
		if src := w.stageSource(body, c); src != "" {
			return src
		}
	}
	return ""
}

// interpreterScript returns the index in an interpreter's words (the name
// first) of the word it runs as its script, an inline string or a file; 0
// when it reads its script from standard input, as a shell with -s does
// whatever operands follow (`sh -s -- -y`); -1 for an inline flag with no
// string after it. A JavaScript runtime's run subcommand is read past.
func interpreterScript(run []string, flags map[string]bool, shell bool) int {
	for j := 1; j < len(run); j++ {
		switch tok := run[j]; {
		case j == 1 && tok == "run" && jsRuntimes[filepath.Base(run[0])] != nil:
		case flags[tok] || (shell && shellCFlag.MatchString(tok)):
			if j+1 < len(run) {
				return j + 1
			}
			return -1
		case shell && shortBundle(tok) && strings.ContainsRune(tok[1:], 's'):
			return 0
		case tok == "--":
			if j+1 < len(run) {
				return j + 1
			}
			return 0
		case shell && (tok == "-o" || tok == "+o"):
			j++
		case strings.HasPrefix(tok, "-") || strings.HasPrefix(tok, "+"):
		default:
			return j
		}
	}
	return 0
}

// inlineScript reports whether the word at script in an interpreter's
// words (interpreterScript) is an inline script string, not a file.
func inlineScript(run []string, script int, flags map[string]bool, shell bool) bool {
	if script < 2 {
		return false
	}
	flag := run[script-1]
	return flags[flag] || (shell && shellCFlag.MatchString(flag))
}

// scriptUse is how a command runs a script. at is the index of its script
// word among its words: 0 for none, -1 for an inline flag with no string
// after it. inline marks that word as a script string, not a file, and
// stdin a command that runs its standard input or a descriptor: no script,
// or one named /dev/stdin or /dev/fd/N.
type scriptUse struct {
	at            int
	inline, stdin bool
}

// scriptOf reads how a command, its words from the name on, runs a script:
// an interpreter or JavaScript runtime (interpreterScript), or source and .
// of a file.
func scriptOf(run []string) scriptUse {
	if len(run) == 0 {
		return scriptUse{}
	}
	head := filepath.Base(run[0])
	if head == "source" || run[0] == "." {
		if len(run) < 2 {
			return scriptUse{}
		}
		return scriptUse{at: 1, stdin: inputFile(run[1])}
	}
	flags, ok := scriptRunner(head)
	if !ok {
		return scriptUse{}
	}
	shell := shellInterpreters[head]
	script := interpreterScript(run, flags, shell)
	if script <= 0 {
		return scriptUse{at: script, stdin: script == 0}
	}
	return scriptUse{
		at:     script,
		inline: inlineScript(run, script, flags, shell),
		stdin:  inputFile(run[script]),
	}
}

// codeWords returns the words of a command whose text, or whose
// substitutions' output, runs as code: its command word, an interpreter's
// script string or file, eval's words, source's file, every word a runner
// hands a shell, and the -a file of an xargs that runs its items as code
// (`xargs -a <(curl …) sh -c`). stdin reports an interpreter or source that
// runs what it reads from standard input or a descriptor (no script, or a
// script named /dev/stdin or /dev/fd/N); its input redirection targets are
// code words then too.
func (w codeWalk) codeWords(text string, c shellCmd, run []string, at int, kind wrapKind) (code []placedWord, stdin bool) {
	if len(run) == 0 {
		return nil, false
	}
	words := c.words[at:]
	code = []placedWord{words[0]}
	if kind == runsAsText {
		code = append(code, words[1:]...)
	}
	use := scriptOf(run)
	if use.at > 0 {
		code = append(code, words[use.at])
	}
	if call, ok := xargsRunsItems(w.s.names, run); ok && call.itemFile > 0 {
		code = append(code, words[call.itemFile])
	}
	if filepath.Base(run[0]) == "eval" {
		code = append(code, words[1:]...)
	}
	if use.stdin {
		code = append(code, inputTargets(text, c)...)
	}
	return code, use.stdin
}

// codeBodies returns the substitution bodies whose output a command runs as
// code: those in its code words (codeWords), and for a command that runs
// its standard input, an unquoted heredoc's substitutions.
func codeBodies(text string, c shellCmd, code []placedWord, stdin bool) []span {
	var out []span
	for _, w := range code {
		out = append(out, substBodies(text, w)...)
	}
	if stdin {
		for _, d := range c.docs {
			if !d.quoted {
				out = append(out, bodySubstitutions(text, d.body)...)
			}
		}
	}
	return out
}

// substBodies returns the bodies of a word's $( ), backtick and <( )
// substitutions: the pieces whose output becomes the word, or the file it
// names.
func substBodies(text string, w placedWord) []span {
	var out []span
	for _, p := range w.pieces() {
		switch {
		case p.from >= 1 && text[p.from-1] == '`':
		case p.from >= 2 && text[p.from-1] == '(' && (text[p.from-2] == '$' || text[p.from-2] == '<'):
		default:
			continue
		}
		out = append(out, p)
	}
	return out
}

// inputTargets returns a command's input redirection targets: the word
// after <, <<< or a heredoc's <<.
func inputTargets(text string, c shellCmd) []placedWord {
	var out []placedWord
	for _, t := range c.targets {
		i := t.at.from - 1
		for i >= 0 && (text[i] == ' ' || text[i] == '\t') {
			i--
		}
		if i >= 0 && text[i] == '<' {
			out = append(out, t)
		}
	}
	return out
}

// nestedCode returns the code texts a command runs inside it: its ( )
// subshells and substitutions, a shell's -c string, eval's words, the words
// a runner hands a shell, and a heredoc a shell reads as its script.
func nestedCode(text string, c shellCmd, tokens, run []string, at int, kind wrapKind) []string {
	var out []string
	for _, s := range c.subshells {
		out = append(out, text[s.from:s.to])
	}
	for _, p := range c.pieces() {
		out = append(out, text[p.from:p.to])
	}
	if i := inlineScriptAt(tokens); i >= 0 {
		out = append(out, c.words[i].script())
	}
	if len(run) > 0 && run[0] == "eval" {
		out = append(out, wordsScript(c.words[at+1:]))
	}
	if kind == runsAsText {
		out = append(out, wordsScript(c.words[at:]))
	}
	if len(run) > 0 && shellInterpreters[filepath.Base(run[0])] {
		for _, d := range c.docs {
			out = append(out, text[d.body.from:d.body.to])
		}
	}
	return out
}

// wordsScript returns the text a shell runs when it gets words as one
// script, as eval joins its words (placedWord.script).
func wordsScript(words []placedWord) string {
	parts := make([]string, 0, len(words))
	for _, w := range words {
		parts = append(parts, w.script())
	}
	return strings.Join(parts, " ")
}

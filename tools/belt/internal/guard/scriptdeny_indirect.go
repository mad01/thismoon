package guard

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// This file reads the commands a segment hides from the permission system's
// prefix match, which judges a command by the words it starts with: a
// command behind a reserved word or a wrapper (`then`, `nice`, `watch`,
// `script`), a command a variable names (`X=rm; $X -rf /x`, `$RM -rf /x`),
// and an alias or function the same text defines and then calls. The deny
// list applies to each as if it were written out.

// wrapKind says how a segment reaches the command it runs.
type wrapKind int

const (
	runsDirect  wrapKind = iota // the segment's own head
	runsWrapped                 // behind a wrapper that runs it as written
	runsAsText                  // behind one that hands its words to a shell
)

// wrapperSpec is how a wrapper that runs the command after it reads its own
// options (wrapperOptions), the way getopt does. short holds the option
// letters, each followed by ':' when it takes a value (in the same word or
// the next) and by '::' when it takes one only in the same word; long holds
// the long names, space-separated, marked the same way. The options stop at
// the first word that is not one, or after --.
type wrapperSpec struct {
	short, long string
	// asText names the options that hand the command to a shell to split
	// and run (sudo -s, env -S); noRun those after which the wrapper runs
	// no command (command -v).
	asText, noRun string
	// words holds the options a wrapper reads as whole words after one dash
	// (arch -arm64, -arch NAME), marked as long names are. A wrapper with
	// words reads no short bundles.
	words string
	// loneDash marks a wrapper that reads a lone - as an option (env -).
	loneDash bool
	// operand marks a wrapper that reads one word after its options and
	// before its command: timeout's duration, flock's lock file. textNext
	// names the words after that operand that hand the next word to a shell
	// (flock FILE -c CMD).
	operand  bool
	textNext string
	// async marks a wrapper whose command may run beside the shell, which
	// then goes on without waiting for it: coproc, and setsid, which forks
	// when it leads its process group.
	async bool
}

// afterOperand reads the words after an operand wrapper's operand. A
// textNext word hands the words after it to a shell, and any other word
// that starts with - is one belt cannot read there, so the words from it on
// are shell text too (fail closed). ok is false for a command run as
// written.
func (w wrapperSpec) afterOperand(rest []string) ([]string, bool) {
	switch {
	case len(rest) == 0 || !strings.HasPrefix(rest[0], "-"):
		return nil, false
	case hasWord(w.textNext, rest[0]):
		return rest[1:], true
	}
	return rest, true
}

// option is one option a wrapper word gives, how it takes a value (optArg,
// as git's options do), and whether its value is in that word, and which.
type option struct {
	name     string
	arg      optArg
	attached bool
	value    string
}

// read reads the wrapper's options from the words after its name and
// returns the words from its command on, with how it runs them. An option
// the wrapper does not know, or a value given to one that takes none,
// leaves belt unable to tell where the command starts: then every word
// after the name is returned as shell text, so the reader scans them all.
func (w wrapperSpec) read(args []string) ([]string, wrapKind) {
	kind := runsWrapped
	for i := 0; i < len(args); i++ {
		if args[i] == "--" {
			return args[i+1:], kind
		}
		opts, ok := w.options(args[i])
		switch {
		case !ok:
			return args, runsAsText
		case opts == nil:
			return args[i:], kind
		}
		for _, o := range opts {
			switch {
			case hasWord(w.noRun, o.name):
				return nil, kind
			case hasWord(w.asText, o.name) && o.arg != argNone:
				if o.attached {
					return args[i:], runsAsText
				}
				return args[i+1:], runsAsText
			case hasWord(w.asText, o.name):
				kind = runsAsText
			}
			if o.arg == argNeeded && !o.attached {
				i++
			}
		}
	}
	return nil, kind
}

// options returns the options one word gives: nil for a word that is not an
// option, and false for one the wrapper does not know. A short bundle gives
// its letters up to the first one that takes a value.
func (w wrapperSpec) options(word string) ([]option, bool) {
	switch {
	case word == "-":
		if w.loneDash {
			return []option{{name: "-"}}, true
		}
		return nil, true
	case strings.HasPrefix(word, "--"):
		name, value, hasValue := strings.Cut(word[2:], "=")
		arg, ok := optionIn(strings.Fields(w.long), name)
		if !ok || (arg == argNone && hasValue) {
			return nil, false
		}
		return []option{{name, arg, hasValue, value}}, true
	case len(word) < 2 || word[0] != '-':
		return nil, true
	case w.words != "":
		arg, ok := optionIn(strings.Fields(w.words), word[1:])
		if !ok {
			return nil, false
		}
		return []option{{name: word[1:], arg: arg}}, true
	}
	var out []option
	for j := 1; j < len(word); j++ {
		i := strings.IndexByte(w.short, word[j])
		if i < 0 || word[j] == ':' {
			return nil, false
		}
		arg := optionTail(w.short[i+1:])
		if arg != argNone {
			return append(out, option{word[j : j+1], arg, j+1 < len(word), word[j+1:]}), true
		}
		out = append(out, option{name: word[j : j+1]})
	}
	return out, true
}

// optionIn finds a long option among a spec's marked names.
func optionIn(names []string, name string) (optArg, bool) {
	for _, n := range names {
		if bare := strings.TrimRight(n, ":"); bare == name {
			return optionTail(n[len(bare):]), true
		}
	}
	return argNone, false
}

// optionTail reads the colons after an option in a spec.
func optionTail(s string) optArg {
	switch {
	case strings.HasPrefix(s, "::"):
		return argOptional
	case strings.HasPrefix(s, ":"):
		return argNeeded
	}
	return argNone
}

// hasWord reports whether a space-separated list holds a word.
func hasWord(list, word string) bool {
	return slices.Contains(strings.Fields(list), word)
}

// shellRunners are the wrappers that hand the words after their options to
// a shell, which splits and expands them again: watch, parallel (which also
// adds the words after :::) and script -c. BSD script runs the command
// after its log file as written. stripWrappers keeps them, so a git guard
// reads a git call behind one as fed (fedOperands). Each reader takes the
// words after the runner's name and returns the words from the command on.
var shellRunners = map[string]func(args []string) ([]string, wrapKind){
	"watch": func(args []string) ([]string, wrapKind) {
		return skipOptions(args, watchValued), runsAsText
	},
	"parallel": func(args []string) ([]string, wrapKind) {
		return skipOptions(args, parallelValued), runsAsText
	},
	"script": readScript,
}

// The options of each shell runner that take the next word as their value.
var (
	watchValued    = wordSet("-n --interval -q --equexit")
	parallelValued = wordSet(`
		-a --arg-file -C --colsep -d --delimiter -E -I -j --jobs -P -L --max-lines
		-n --max-args -N -S --sshlogin --sshloginfile -s --max-chars --tmpdir
		--joblog --results --workdir --wd --timeout --delay --retries --tagstring
		--env --basefile --bf --return --memfree --load --nice --halt`)
	scriptValued = wordSet(`
		-F -t -T -E -I -O -B -m -o --echo --log-in --log-out --log-io
		--log-timing --logging-format --output-limit`)
)

// wrapperName reports whether a command name is a wrapper or a shell runner.
func wrapperName(name string) bool {
	_, runner := shellRunners[name]
	return runner || isWrapper(name)
}

// readScript reads script's words. util-linux runs a -c (--command) string,
// before or right after the log file, as shell text. BSD runs the command
// after the log file as written; with nothing after the file, either starts
// an interactive shell, which runs no command of its own.
func readScript(args []string) ([]string, wrapKind) {
	rest := skipOptions(args, scriptValued)
	opts := args[:len(args)-len(rest)]
	if i := slices.IndexFunc(opts, scriptCommandWord); i >= 0 {
		return scriptCommandText(args, i), runsAsText
	}
	if len(rest) > 1 && scriptCommandWord(rest[1]) {
		return scriptCommandText(rest, 1), runsAsText
	}
	if len(rest) < 2 {
		return nil, runsWrapped
	}
	return rest[1:], runsWrapped
}

// scriptCommandWord reports whether a script option word gives the command
// string: -c or --command with the string as the next word, --command=…,
// or a short bundle holding c (-qc, -c'…').
func scriptCommandWord(word string) bool {
	return strings.HasPrefix(word, "--command") ||
		(len(word) > 1 && word[0] == '-' && word[1] != '-' && strings.ContainsRune(word, 'c'))
}

// scriptCommandText returns the words from script's command string on: the
// word after the option when the option ends its word, else the option word
// itself, which holds the string.
func scriptCommandText(args []string, i int) []string {
	if word := args[i]; word == "--command" || (strings.HasSuffix(word, "c") && !strings.HasPrefix(word, "--")) {
		return args[i+1:]
	}
	return args[i:]
}

// funcNameWord is a function definition's name word as strings.Fields
// reads it: `f()`, or `()` for a zsh anonymous function.
var funcNameWord = regexp.MustCompile(`^[^\s(){};&|'"$` + "`" + `]*\(\)$`)

// skipOpeners drops what opens a command before its name: the reserved
// words rm-temp reads past too (shellReserved: `if`, `then`, `do`, `!`,
// `{`), a ( that strings.Fields leaves alone, and a function definition's
// head (`f()`, `function f`), whose body is read as the code it runs once
// called. It returns a suffix of tokens.
func skipOpeners(tokens []string) []string {
	for len(tokens) > 0 {
		switch t := tokens[0]; {
		case shellReserved[t], t == "(", funcNameWord.MatchString(t):
			tokens = tokens[1:]
		case t == "function" && len(tokens) > 1:
			tokens = tokens[2:]
		default:
			return tokens
		}
	}
	return tokens
}

// unwrapCommand returns the words a segment runs once what opens it is
// read past (skipOpeners) and the wrappers in front of its command are
// (peelWrappers, the shellRunners), as a suffix of tokens, and how the
// segment reaches them. What opens a command is read past after a wrapper
// too (`time { …; }`, `coproc { …; }`). Assignments and reserved words
// alone leave it direct. A runner's words that start with an option are the
// option word that holds the command string (`--command=…`) and are
// returned as they are.
func unwrapCommand(tokens []string) ([]string, wrapKind) {
	kind := runsDirect
	for {
		rest, how := peelWrappers(skipOpeners(tokens))
		kind = max(kind, how)
		if how == runsAsText || len(rest) == 0 {
			return rest, kind
		}
		if opened := skipOpeners(rest); len(opened) < len(rest) {
			tokens = opened
			continue
		}
		read, ok := shellRunners[filepath.Base(rest[0])]
		if !ok {
			return rest, kind
		}
		inner, how := read(rest[1:])
		kind = max(kind, how)
		if len(inner) > 0 && strings.HasPrefix(inner[0], "-") {
			return inner, kind
		}
		tokens = inner
	}
}

// leadingDeny returns the first deny entry a command, its words with the
// name first, starts with (deny.leads), or nil.
func leadingDeny(words []string, patterns []deny) *deny {
	for i := range patterns {
		if patterns[i].leads(words) {
			return &patterns[i]
		}
	}
	return nil
}

// leads reports whether a command, its words with the name first, runs what
// the deny entry names the way the permission system's prefix match reads a
// command: on argv for an entry the argv match reads, and as text from the
// command's first word, with quotes collapsed too. A regex entry matches
// anywhere, as it does in scanText.
func (d deny) leads(words []string) bool {
	if d.runsWords(words) {
		return true
	}
	line := strings.Join(words, " ")
	if strings.HasPrefix(d.pattern, "re:") {
		return d.re.MatchString(line)
	}
	for _, form := range []string{line, strings.TrimSpace(lineNoise.Replace(line))} {
		if loc := d.re.FindStringIndex(form); loc != nil && loc[0] == 0 {
			return true
		}
	}
	return false
}

// runsAsVariable reports whether a command a variable names, with these
// arguments, may run the deny entry: the variable's name holds the entry's
// command word, case aside (`$RM`, `${KUBECTL_BIN}` for kubectl), and the
// arguments match what the entry names after that word (`-rf /x` for `rm
// -rf`, `-n x delete pod` for `kubectl delete`). The name is all there is
// to go on: shell state does not carry between Bash calls, so a variable the
// command does not set comes from the user's profile, and `$GIT rm x` runs
// git. A regex entry and a one-word entry name no arguments to match.
func (d deny) runsAsVariable(variable string, args []string) bool {
	words := strings.Fields(d.pattern)
	if strings.HasPrefix(d.pattern, "re:") || len(words) < 2 ||
		!strings.Contains(strings.ToLower(variable), strings.ToLower(words[0])) {
		return false
	}
	return d.leads(append([]string{words[0]}, args...))
}

// maxNameDepth caps how many alias, function or variable expansions deep
// hiddenRun reads, so a function that calls itself ends.
const maxNameDepth = 4

// shellNames are the names a text gives commands of its own: the literal
// values it assigns variables, the bodies of the functions it defines and
// the values of its aliases, each by name. seen holds the expansions read
// already, so a function called many times is read once per argument list.
type shellNames struct {
	values, funcs, aliases map[string][]string
	seen                   map[string]bool
}

var (
	// literalAssign is NAME=value where a word starts, quotes included (a
	// -c string's first word): a quoted string without expansions in it, or
	// unquoted text up to a separator or an expansion.
	literalAssign = regexp.MustCompile(
		"(?:^|[\\s;&|(`'\"])([A-Za-z_]\\w*)=('[^']*'|\"[^\"$`\\\\]*\"|[^\\s;&|()<>'\"$`\\\\]+)",
	)
	// composedAssign is NAME=value where the value joins literal text and
	// variable references only (`Z="$Y -rf /x"`, `Z=$A$B`): double quoted,
	// or unquoted up to a separator.
	composedAssign = regexp.MustCompile(
		"(?:^|[\\s;&|(`'\"])([A-Za-z_]\\w*)=(\"(?:[^\"$`\\\\]|" + variableRefText + ")*\"|" +
			"(?:[^\\s;&|()<>'\"$`\\\\]|" + variableRefText + ")+)",
	)
	// variableRef is one reference in a composed value: $NAME or ${NAME}.
	variableRef = regexp.MustCompile(variableRefText)
	// aliasDef is alias NAME=value, the first pair of each alias command.
	aliasDef = regexp.MustCompile(
		`(?:^|[\s;&|('"])alias\s+(?:-\w+\s+)*([^\s=;&|()'"]+)=('[^']*'|"[^"]*"|[^\s;&|()]+)`,
	)
	// funcHead is a function definition up to its body's opening brace or
	// parenthesis: `name() {`, `function name {`, `function name() (`.
	funcHead = regexp.MustCompile(
		`(?:^|[\s;&|(){'"])(?:function\s+([^\s(){};&|'"]+)\s*(?:\(\s*\))?|([A-Za-z_][\w.:-]*)\s*\(\s*\))\s*([{(])`,
	)
	// positionalWord is a reference to a script argument, with the double
	// quotes around it: "$@", $*, $1, ${2}.
	positionalWord = regexp.MustCompile(`"?\$\{?([@*1-9])\}?"?`)
)

// variableRefText is the pattern of a $NAME or ${NAME} reference.
const variableRefText = `\$(?:([A-Za-z_]\w*)|\{([A-Za-z_]\w*)\})`

// maxComposed caps how many values one composed assignment resolves to,
// one per mix of the values its references hold.
const maxComposed = 8

// readShellNames reads the names text gives commands (shellNames). A value
// composed of literal text and variable references is resolved through the
// literals (resolve). It reads text, not shell structure, so it may find a
// name the shell never sets, which only adds text for the deny list to read.
func readShellNames(text string) shellNames {
	n := shellNames{
		values: map[string][]string{}, funcs: map[string][]string{},
		aliases: map[string][]string{}, seen: map[string]bool{},
	}
	for _, m := range literalAssign.FindAllStringSubmatch(text, -1) {
		n.values[m[1]] = append(n.values[m[1]], unquoteValue(m[2]))
	}
	for _, m := range composedAssign.FindAllStringSubmatch(text, -1) {
		if strings.Contains(m[2], "$") {
			n.values[m[1]] = append(n.values[m[1]], n.resolve(unquoteValue(m[2]))...)
		}
	}
	for _, m := range aliasDef.FindAllStringSubmatch(text, -1) {
		n.aliases[m[1]] = append(n.aliases[m[1]], unquoteValue(m[2]))
	}
	for _, m := range funcHead.FindAllStringSubmatchIndex(text, -1) {
		name := ""
		switch {
		case m[2] >= 0: // function name
			name = text[m[2]:m[3]]
		case m[4] >= 0: // name()
			name = text[m[4]:m[5]]
		}
		open := m[6]
		n.funcs[name] = append(n.funcs[name], text[open+1:bodyEnd(text, open)])
	}
	return n
}

// resolve returns the values a composed value takes: each reference
// replaced by each value the text gives that variable, as far as
// maxComposed, and kept as ${NAME} for a variable the text gives none, so
// hiddenRun reads it by its name.
func (n shellNames) resolve(value string) []string {
	loc := variableRef.FindStringSubmatchIndex(value)
	if loc == nil {
		return []string{value}
	}
	from, to := loc[2], loc[3] // $NAME
	if from < 0 {
		from, to = loc[4], loc[5] // ${NAME}
	}
	name := value[from:to]
	subs := n.values[name]
	if len(subs) == 0 {
		subs = []string{"${" + name + "}"}
	}
	rests := n.resolve(value[loc[1]:])
	var out []string
	for _, sub := range subs {
		for _, rest := range rests {
			if len(out) == maxComposed {
				return out
			}
			out = append(out, value[:loc[0]]+sub+rest)
		}
	}
	return out
}

// unquoteValue drops the quotes around a value the shell stores without
// them.
func unquoteValue(v string) string {
	if _, inner := unquote(v); inner != v {
		return inner
	}
	return v
}

// bodyEnd returns the index of the brace or parenthesis that closes the one
// at open, skipping quoted text and escapes, or len(text) when none does. A
// ${...} inside a brace body opens and closes a brace of its own.
func bodyEnd(text string, open int) int {
	opener, closer := text[open], byte('}')
	if opener == '(' {
		closer = ')'
	}
	depth := 0
	var quote byte
	for i := open; i < len(text); i++ {
		switch c := text[i]; {
		case quote != 0:
			if c == '\\' && quote == '"' {
				i++
			} else if c == quote {
				quote = 0
			}
		case c == '\\':
			i++
		case c == '\'' || c == '"':
			quote = c
		case c == opener:
			depth++
		case c == closer:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(text)
}

// withArgs returns a function body with its argument references replaced by
// the call's arguments, as the shell expands them when it runs the call.
func withArgs(body string, args []string) string {
	return positionalWord.ReplaceAllStringFunc(body, func(ref string) string {
		switch name := positionalWord.FindStringSubmatch(ref)[1]; name {
		case "@", "*":
			return strings.Join(args, " ")
		default:
			n, _ := strconv.Atoi(name)
			if n <= len(args) {
				return args[n-1]
			}
			return ""
		}
	})
}

// shellVariables are the variables a shell sets to its own name or path, so
// a command word that expands one runs a shell: $BASH, $ZSH_NAME and
// $ZSH_ARGZERO. $SHELL and any other name that ends in SHELL (LOGIN_SHELL)
// count too (isShellVariable).
var shellVariables = wordSet("BASH ZSH_NAME ZSH_ARGZERO")

// wholeVariableRef is a word that is one variable reference and nothing
// else: $NAME or ${NAME}, its quotes dropped (commandName).
var wholeVariableRef = regexp.MustCompile(`^` + variableRefText + `$`)

// isShellVariable reports whether a variable holds a shell by its name
// (shellVariables), case aside.
func isShellVariable(name string) bool {
	upper := strings.ToUpper(name)
	return shellVariables[upper] || strings.HasSuffix(upper, "SHELL")
}

// runnerNamed returns the interpreter a command word names through a
// variable: "sh" for a variable a shell sets to itself (isShellVariable),
// or the interpreter a literal value the text assigns it starts with (`S=sh`,
// `P=/usr/bin/python3`, scriptRunner). ok is false for any other word, a
// literal name included, which the interpreter checks read as written.
func (n shellNames) runnerNamed(word string) (string, bool) {
	m := wholeVariableRef.FindStringSubmatch(commandName(word))
	if m == nil {
		return "", false
	}
	variable := m[1] + m[2]
	if isShellVariable(variable) {
		return "sh", true
	}
	for _, value := range n.values[variable] {
		if fields := strings.Fields(value); len(fields) > 0 {
			if _, ok := scriptRunner(filepath.Base(fields[0])); ok {
				return filepath.Base(fields[0]), true
			}
		}
	}
	return "", false
}

// withRunner returns a command's words with a command word that names an
// interpreter through a variable (runnerNamed) replaced by the
// interpreter's name, so the checks that classify a command by its name
// read `"$SHELL" -c …` and `S=sh; … | $S` as the shell they run. Any other
// command's words are returned as they are.
func (n shellNames) withRunner(run []string) []string {
	if len(run) == 0 {
		return run
	}
	name, ok := n.runnerNamed(run[0])
	if !ok {
		return run
	}
	return slices.Concat([]string{name}, run[1:])
}

// hides reports whether a command name hides the command it runs: an alias
// or function the text defines, or a name the shell expands.
func (n shellNames) hides(name string) bool {
	return len(n.aliases[name]) > 0 || len(n.funcs[name]) > 0 || strings.ContainsAny(name, "$`")
}

// hiddenRun checks one command, its words with the name first, whose name
// the deny list cannot read as written. An alias or function the text
// defines runs as its value or body with the call's arguments, read with
// scanLines. A name a variable gives is read by variableRun.
func (n shellNames) hiddenRun(words []string, where string, patterns []deny, depth int) *Denial {
	if len(words) == 0 || depth >= maxNameDepth {
		return nil
	}
	name, args := commandName(words[0]), words[1:]
	r := nameRead{names: n, name: name, where: where, patterns: patterns, depth: depth}
	for _, value := range n.aliases[name] {
		if d := r.expand("alias", value+" "+strings.Join(args, " ")); d != nil {
			return d
		}
	}
	for _, body := range n.funcs[name] {
		if d := r.expand("function", withArgs(body, args)); d != nil {
			return d
		}
	}
	if !strings.ContainsAny(name, "$`") {
		return nil
	}
	return r.variableRun(args)
}

// nameRead is one hiddenRun call: the names it reads through, the command
// name it reads, where the command stands, the deny list, and how many
// expansions deep the text already is.
type nameRead struct {
	names    shellNames
	name     string
	where    string
	patterns []deny
	depth    int
}

// expand reads text the name runs as (an alias value, a function body, a
// variable value) with scanLines, one expansion deeper and once per text.
func (r nameRead) expand(kind, text string) *Denial {
	if r.names.seen[text] {
		return nil
	}
	r.names.seen[text] = true
	through := fmt.Sprintf("%s, through the %s %s,", r.where, kind, r.name)
	return scanLines(text, through, r.patterns, r.names, r.depth+1)
}

// variableRun reads a command whose name a variable gives: it runs as each
// value the text assigns that variable. A variable the text gives no value,
// or one that still holds a reference only the shell can expand, is read by
// its name too (deny.runsAsVariable).
func (r nameRead) variableRun(args []string) *Denial {
	variable := strings.Trim(r.name, "${}")
	values := r.names.values[variable]
	literal := true
	for _, value := range values {
		if d := r.expand("variable", value+" "+strings.Join(args, " ")); d != nil {
			return d
		}
		literal = literal && !strings.ContainsAny(value, "$`")
	}
	if len(values) > 0 && literal {
		return nil
	}
	for _, d := range r.patterns {
		if d.runsAsVariable(variable, args) {
			return Reasonf(
				ScriptDenyListID,
				"%s runs a command named by %s, which only the shell can expand, with arguments that match %q on the Bash deny list. "+
					"Write the command name out so the permission system can evaluate it, or ask the user first.",
				r.where,
				r.name,
				d.pattern,
			)
		}
	}
	return nil
}

// checkHidden denies a top-level segment whose command hides from the
// permission system's prefix match. Behind a wrapper that runs it as
// written, the command may not start with a deny entry (leadingDeny).
// Behind one that hands it to a shell, its text is scanned to the end of the
// command, as a -c string is. A name a variable, alias or function gives is
// read by hiddenRun.
func (s scan) checkHidden(seg segment, wrapper string, tokens []string, kind wrapKind) *Denial {
	switch kind {
	case runsAsText:
		where := fmt.Sprintf("the shell text after %s", wrapper)
		if d := scanText(s.cmd[seg.off:], where, s.patterns); d != nil {
			return d
		}
	case runsWrapped:
		if d := leadingDeny(tokens, s.patterns); d != nil {
			return Reasonf(
				ScriptDenyListID,
				"runs %q behind %s, which the permission system's prefix match does not read past. "+
					"Run the command without the wrapper so the permission system can evaluate it, or ask the user first.",
				d.pattern,
				wrapper,
			)
		}
	}
	return s.names.hiddenRun(tokens, "the command", s.patterns, 0)
}

// firstCommandWord returns the first word of a segment that is not an
// assignment: the wrapper's name when the segment has one.
func firstCommandWord(fields []string) string {
	i := slices.IndexFunc(fields, func(f string) bool { return !strings.Contains(f, "=") })
	if i < 0 {
		return ""
	}
	return fields[i]
}

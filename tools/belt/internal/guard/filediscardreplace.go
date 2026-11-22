package guard

import (
	"regexp"
	"slices"
	"strings"
)

// This file holds file-discard's replaces: a command that empties or
// overwrites a file in place. A redirection that truncates its target
// behind a command that prints nothing, truncate, and dd's of= each drop
// one file, never a tree. Each writes through a symlink in the last part of
// the path, so the file judged is the one the link leads to.

// replaceCommands are the commands that set a file's size or bytes in
// place, by the name each runs as: GNU coreutils from Homebrew installs
// them with a g in front.
var replaceCommands = map[string]string{
	"truncate": "truncate", "gtruncate": "truncate", "dd": "dd", "gdd": "dd",
}

// replace judges each file a replace writes with the delete ladder (drop),
// as one file. A path under /dev is a device, not a file. The toss-bin form
// moves the file to the trash, then runs the command as written.
func (j *discardJudge) replace(ops []lossOperand, s walkStep) *fileLoss {
	command := writtenCommand(s)
	for _, op := range ops {
		if v, ok := op.word.literal(); ok && (v == "/dev" || strings.HasPrefix(v, "/dev/")) {
			continue
		}
		if loss := j.operand(replaceCall(op.word.raw, command), op, s); loss != nil {
			return loss
		}
	}
	return nil
}

// replaceCall is the call for a replace of the file as written, whose
// toss-bin form moves the file to the trash, then runs command.
func replaceCall(written, command string) *lossCall {
	return &lossCall{replace: true, toss: "move it to the trash first: `" + TossBin +
		" --safe-mode " + written + " && " + command + "`"}
}

// redirects judges each redirection that empties its target before the
// command runs (truncates) when what writes there prints nothing: the
// command (quiet), or the ( ) subshell, { } group or compound command it
// closes (quietCmds): `> f`, `: > f`, `: 2> f`, `cat /dev/null > f`,
// `exec > f`, `( : ) > f`, `{ :; } > f`. A command that prints puts new
// content in the file, which file-discard leaves alone, and a process
// substitution is no file. The } that ends a function's body opens nothing
// where it stands: it tells whether a call of the function prints
// (discardJudge.funcs).
func (j *discardJudge) redirects(words []shWord, s walkStep) *fileLoss {
	switch g := s.group; {
	case g != nil && g.function != "":
		j.funcs[g.function] = j.quietCmds(g.text, g.cmds)
		return nil
	case g != nil:
		if !j.quietCmds(g.text, g.cmds) {
			return nil
		}
	case !j.quiet(words, nullInput(s.redirs, s.targetWords)):
		return nil
	}
	var ops []lossOperand
	for i, r := range s.redirs {
		if i < len(s.targetWords) && truncates(r, s.targetWords[i].shWord) && !procSubst(s.targetWords[i].shWord) {
			ops = append(ops, lossOperand{word: s.targetWords[i].shWord, follows: true})
		}
	}
	return j.replace(ops, s)
}

// procSubst reports whether a word is a >( ) or <( ) process
// substitution, a pipe the shell opens and no file.
func procSubst(w shWord) bool {
	return strings.HasPrefix(w.raw, ">(") || strings.HasPrefix(w.raw, "<(")
}

// quietCmds reports whether every command of a group's body, read from
// text, prints nothing (quietCmd).
func (j *discardJudge) quietCmds(text string, cmds []shellCmd) bool {
	return !slices.ContainsFunc(cmds, func(c shellCmd) bool { return !j.quietCmd(text, c) })
}

// quietCmd reports whether one command of a group's body prints nothing.
// Past the reserved words it starts with (groupWords), it is quiet when it
// is a for loop's head, when it holds ( ) bodies alone and each is quiet,
// or when its words past assignments are (quiet). A select's head prints a
// menu, and a case's is read as no command, so neither is quiet.
func (j *discardJudge) quietCmd(text string, c shellCmd) bool {
	words := plainWords(c.words)
	for len(words) > 0 && groupWords[words[0].raw] {
		words = words[1:]
	}
	switch {
	case len(words) > 0 && words[0].raw == "for":
		return true
	case len(c.subshells) == 0:
		return j.quiet(skipAssignments(words), nullInput(c.redirs, c.targets))
	case len(words) > 0:
		return false
	}
	for _, s := range c.subshells {
		body := text[s.from:s.to]
		cmds, ok := readShell(body)
		if !ok || !j.quietCmds(body, cmds) {
			return false
		}
	}
	return true
}

// groupWords are the reserved words that open, join or close a group, which
// a command in a group's body may start with.
var groupWords = wordSet("! { } if then elif else fi while until do done")

// quiet is printsNothing for a command that may call a function the same
// text defines: such a call prints nothing when the function's body does
// (discardJudge.funcs).
func (j *discardJudge) quiet(words []shWord, nullIn bool) bool {
	if len(words) > 0 {
		if v, ok := words[0].literal(); ok {
			if quiet, defined := j.funcs[v]; defined {
				return quiet
			}
		}
	}
	return printsNothing(words, nullIn)
}

// nullInput reports whether a command's standard input is /dev/null: the
// last redirection of descriptor 0 is a < from it.
func nullInput(redirs []redirOp, targets []placedWord) bool {
	null := false
	for i, r := range redirs {
		if i < len(targets) && strings.HasPrefix(r.op, "<") && (r.fd == "" || r.fd == "0") {
			v, ok := targets[i].literal()
			null = r.op == "<" && ok && v == "/dev/null"
		}
	}
	return null
}

// echoOptions is an echo word that sets options and prints nothing: -n,
// -e, -E and their bundles.
var echoOptions = regexp.MustCompile(`^-[neE]+$`)

// printfEmpty is a printf format that prints nothing for empty values: %s
// and %b alone.
var printfEmpty = regexp.MustCompile(`^(%[sb])*$`)

// printsNothing reports whether a command, read from its words past
// assignments, prints nothing but a newline: no command at all, `:`, true
// or false, an exec that runs no command and so opens its redirections in
// the shell itself, a cat of /dev/null, or of its standard input when that
// is /dev/null (nullIn), an echo of options and empty words (echoesNothing),
// or a printf of empty words, after a format of %s and %b alone or none.
// None of them prints to standard error either.
func printsNothing(words []shWord, nullIn bool) bool {
	if len(words) == 0 {
		return true
	}
	rest := words[1:]
	switch runName(words[0]) {
	case ":", "true", "false":
		return true
	case "exec":
		return len(rest) == 0
	case "cat":
		return len(rest) == 0 && nullIn || len(rest) > 0 && !slices.ContainsFunc(rest, func(w shWord) bool {
			v, ok := w.literal()
			return !ok || v != "/dev/null"
		})
	case "echo":
		return echoesNothing(rest)
	case "printf":
		if v, ok := literalAt(rest, 0); ok && v == "--" {
			rest = rest[1:]
		}
		if v, ok := literalAt(rest, 0); ok && printfEmpty.MatchString(v) {
			rest = rest[1:]
		}
		return !slices.ContainsFunc(rest, func(w shWord) bool { return !emptyWord(w) })
	}
	return false
}

// literalAt is the literal text of args[i], ok false past the end or for a
// word with an expansion.
func literalAt(args []shWord, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	return args[i].literal()
}

// echoesNothing reports whether an echo of args prints nothing but a
// newline: options (echoOptions), then empty words alone. echo reads
// options only up to its first other word, so an option word after an
// empty one is text it prints.
func echoesNothing(args []shWord) bool {
	options := true
	for _, w := range args {
		v, ok := w.literal()
		switch {
		case !ok:
			return false
		case options && echoOptions.MatchString(v):
		case v == "":
			options = false
		default:
			return false
		}
	}
	return true
}

// truncates reports whether a redirection empties its target before the
// command runs: > or >| on any descriptor (`2> f`, `3> f`), &>, and >&
// with a file name on any descriptor (`>& f`, `1>& f`), which bash and zsh
// open as a file; zsh does so for `2>& f` too. >& with digits or -
// duplicates or closes a descriptor. >> and &>> append.
func truncates(r redirOp, target shWord) bool {
	switch r.op {
	case ">", ">|", "&>":
		return true
	case ">&":
		v, ok := target.literal()
		return ok && v != "-" && !isDigits(v)
	}
	return false
}

// writtenCommand is a step's command as written, near enough for the
// toss-bin form: its words, then each redirection but a heredoc. A group's
// step gives its redirections alone, which empty the file as the group
// does, since a quiet body prints nothing.
func writtenCommand(s walkStep) string {
	parts := make([]string, 0, len(s.words)+len(s.redirs))
	for _, w := range s.words {
		if s.group != nil {
			break
		}
		parts = append(parts, w.raw)
	}
	for i, r := range s.redirs {
		if i < len(s.targetWords) && r.op != "<<" && r.op != "<<-" {
			parts = append(parts, r.fd+r.op+" "+s.targetWords[i].raw)
		}
	}
	return strings.Join(parts, " ")
}

// replaceOperands reads a truncate or dd's arguments into the files it
// overwrites.
func replaceOperands(name string, args []shWord) []lossOperand {
	if name == "dd" {
		return ddOperands(args)
	}
	return truncateOperands(args)
}

// truncateLongOptions are GNU truncate's long options, and
// truncateValueOptions the ones that take a value. BSD truncate shares -c,
// -r and -s.
var (
	truncateLongOptions  = []string{"help", "io-blocks", "no-create", "reference", "size", "version"}
	truncateValueOptions = wordSet("reference size")
)

// truncateOperands reads truncate's arguments into the files it sets the
// size of, none when it only grows them: a size of +N with no reference
// file, since -r counts +N from the reference's size. An option counts after
// an operand too, as GNU getopt reads it, and `--` ends them. A size belt
// cannot read is no growth.
func truncateOperands(args []shWord) []lossOperand {
	var ops []lossOperand
	size, ref, endOpts := "", false, false
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		switch {
		case endOpts || !lit || len(v) < 2 || v[0] != '-':
			ops = append(ops, lossOperand{word: args[i], follows: true})
			continue
		case v == "--":
			endOpts = true
			continue
		}
		opt, value, glued := truncateOption(v)
		if opt != "" && !glued && i+1 < len(args) {
			i++
			value, _ = args[i].literal()
		}
		switch opt {
		case "reference":
			ref = true
		case "size":
			size = value
		}
	}
	if !ref && strings.HasPrefix(size, "+") {
		return nil
	}
	return ops
}

// truncateOption reads one option word of truncate: the value option it
// sets ("reference" or "size", "" for a flag) and the value glued to it.
// glued is false when the value is the next word.
func truncateOption(v string) (opt, value string, glued bool) {
	if long, ok := strings.CutPrefix(v, "--"); ok {
		name, text, hasText := strings.Cut(long, "=")
		if full := longOption(truncateLongOptions, name); truncateValueOptions[full] {
			return full, text, hasText
		}
		return "", "", true
	}
	for k := 1; k < len(v); k++ {
		switch v[k] {
		case 'r':
			return "reference", v[k+1:], k+1 < len(v)
		case 's':
			return "size", v[k+1:], k+1 < len(v)
		}
	}
	return "", "", true
}

// ddOperands reads dd's operands into the files it writes: the word after
// each of=. The other operands write nothing. A word that starts with an
// expansion may expand to of=, so it is judged as a path, which belt cannot
// place.
func ddOperands(args []shWord) []lossOperand {
	var ops []lossOperand
	for _, w := range args {
		head := literalHead(w)
		_, lit := w.literal()
		switch {
		case strings.HasPrefix(head, "of="):
			ops = append(ops, lossOperand{word: dropHead(w, len("of=")), follows: true})
		case !lit && !strings.Contains(head, "="):
			ops = append(ops, lossOperand{word: w, follows: true})
		}
	}
	return ops
}

// literalHead is the literal text a word starts with, up to its first
// expansion.
func literalHead(w shWord) string {
	var b strings.Builder
	for _, p := range w.parts {
		if p.kind != partLit {
			break
		}
		b.WriteString(p.text)
	}
	return b.String()
}

// dropHead is w without the first n bytes of its literal head
// (literalHead). Its raw text drops them too where it spells them plainly.
func dropHead(w shWord, n int) shWord {
	raw := strings.TrimPrefix(w.raw, literalHead(w)[:n])
	parts := slices.Clone(w.parts)
	for n > 0 && len(parts) > 0 {
		if len(parts[0].text) > n {
			parts[0].text = parts[0].text[n:]
			break
		}
		n -= len(parts[0].text)
		parts = parts[1:]
	}
	return shWord{raw: raw, parts: parts}
}

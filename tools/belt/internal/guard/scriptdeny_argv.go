package guard

import (
	"slices"
	"strconv"
	"strings"
)

// This file reads a deny entry (denyentry.go) the way the tool it names
// reads its arguments, so a spelling the text match misses still counts:
// rm's options in any shape (`rm -r -f` and `rm --recursive --force` for `rm
// -rf`), and the flags before and between any tool's verbs (`kubectl -n prod
// delete` for `kubectl delete`, `git -C x remote prune` for `git remote
// prune`).

// gcloudTracks are gcloud's release tracks. A track group runs the same
// command (`gcloud beta sql instances delete`), so it may be skipped.
var gcloudTracks = map[string]bool{"alpha": true, "beta": true, "preview": true}

// taskTools take a list of tasks and run each one, so a task entry matches
// at any position: `rake db:create db:drop` runs db:drop.
var taskTools = map[string]bool{"rake": true, "rails": true}

// commandBreaks end one command inside a line: list and pipe operators,
// subshell and substitution parentheses, and backticks.
const commandBreaks = ";|&()`"

// runsWords reports whether one command, its words with the command name
// first, runs what the deny entry names, read the way the tool reads its
// arguments. An entry the reader does not read (a `re:` entry, a shape
// readDenyEntry refuses) never matches here.
func (d deny) runsWords(words []string) bool {
	return d.entry != nil && d.entry.runs(words)
}

// runsIn reports whether any of a line's commands runs the deny entry,
// starting at any of its words, the way the text match finds an entry
// anywhere on a line: `sudo rm -r -f`, `xargs -0 rm -r -f` and `find -exec
// rm -r -f` all count.
func (d deny) runsIn(cmds [][]string) bool {
	if d.entry == nil {
		return false
	}
	for _, words := range cmds {
		for i := range words {
			if d.runsWords(words[i:]) {
				return true
			}
		}
	}
	return false
}

// lineCommands splits each form of a line into its commands, each as its
// words, for the argv match.
func lineCommands(forms ...string) [][]string {
	var out [][]string
	isBreak := func(r rune) bool { return strings.ContainsRune(commandBreaks, r) }
	for _, form := range forms {
		for _, part := range strings.FieldsFunc(form, isBreak) {
			if words := strings.Fields(part); len(words) > 0 {
				out = append(out, words)
			}
		}
	}
	return out
}

// ansiCWords returns words with each one written in ANSI-C quoting read as
// the shell passes it (ansiCWord), and words itself when none is.
func ansiCWords(words []string) []string {
	if !slices.ContainsFunc(words, func(w string) bool { return strings.Contains(w, "$'") }) {
		return words
	}
	out := make([]string, len(words))
	for i, w := range words {
		out[i] = ansiCWord(w)
	}
	return out
}

// ansiCWord returns a word that holds a $'...' string as the shell passes
// it, its escapes decoded and its other quotes dropped (`$'delete'` and
// `$'\x64elete'` read as delete). A word with any other expansion, or one
// the reader cannot follow, stays as written.
func ansiCWord(word string) string {
	if !strings.Contains(word, "$'") {
		return word
	}
	return shellValue(word)
}

// shellValue returns a word as the shell passes it when quoting is all it
// holds: its quotes and backslashes dropped and a $'...' string decoded, so
// `d'o'cker`, `$'docker'` and `\docker` all read as docker. A word with any
// other expansion, or one the reader cannot follow, stays as written.
func shellValue(word string) string {
	l := shLexer{src: word}
	w, ok := l.readWord()
	if !ok || l.i != len(word) {
		return word
	}
	if v, ok := w.literalANSIC(); ok {
		return v
	}
	return word
}

// ansiCText returns text with each $'...' string in it decoded as bash
// passes it (ansiCValue), for text a shell reads again as code: a -c
// string, eval's words, a script. A string after a backslash stays as
// written, and so does the rest of the text from a string no quote closes.
func ansiCText(text string) string {
	if !strings.Contains(text, "$'") {
		return text
	}
	var b strings.Builder
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '\\' && i+1 < len(text):
			b.WriteString(text[i : i+2])
			i++
		case strings.HasPrefix(text[i:], "$'"):
			end := ansiCEnd(text, i+2)
			if end < 0 {
				b.WriteString(text[i:])
				return b.String()
			}
			b.WriteString(ansiCValue(text[i+2 : end]))
			i = end
		default:
			b.WriteByte(text[i])
		}
	}
	return b.String()
}

// commandName returns a command word as the name a deny entry gives: the
// last path element, without the backslash that skips an alias (`\rm`,
// `/bin/rm`) and without the quotes around it, such as the one that opens a
// `bash -c 'rm …'` string on the line as written.
func commandName(word string) string {
	if i := strings.LastIndexByte(word, '/'); i >= 0 {
		word = word[i+1:]
	}
	return strings.Trim(word, `\'"`)
}

// rmLongOptions are GNU rm's long options, the one list rm-temp's
// rmLongFlags is built from too. rm takes any unique prefix of one (`--rec`,
// `--for`), and an ambiguous prefix (`--v`) makes it refuse to run.
var rmLongOptions = []string{
	"dir", "force", "help", "interactive", "no-preserve-root",
	"one-file-system", "preserve-root", "recursive", "verbose", "version",
}

// rmShortOptions gives rm's short options the name of their long form, so
// `-R` and `-r` both read as recursive. A letter not listed is its own name.
var rmShortOptions = map[rune]string{
	'd': "dir", 'f': "force", 'i': "interactive", 'R': "recursive", 'r': "recursive", 'v': "verbose",
}

// rmArgs is what an rm command line asks for: its options by name, and its
// operands.
type rmArgs struct {
	options  map[string]bool
	operands []string
}

// readRm reads rm's arguments. An option counts after an operand too, since
// GNU rm reads it there (BSD rm does not, and both may be on PATH), and `--`
// ends the options. known is false when a long option is not one of rm's.
func readRm(args []string) (r rmArgs, known bool) {
	r.options = map[string]bool{}
	known = true
	for i, arg := range args {
		if arg == "--" {
			r.operands = append(r.operands, args[i+1:]...)
			return r, known
		}
		a := readRmArg(arg)
		for _, name := range a.options {
			r.options[name] = true
		}
		if a.operand {
			r.operands = append(r.operands, arg)
		}
		known = known && a.known
	}
	return r, known
}

// rmArg is one of rm's arguments before `--` as readRm reads it: the options
// it gives by name, or that it is an operand. known is false for a long
// option rm does not know.
type rmArg struct {
	options        []string
	operand, known bool
}

// readRmArg reads one of rm's arguments before `--`.
func readRmArg(arg string) rmArg {
	switch {
	case strings.HasPrefix(arg, "--"):
		name, ok := rmLongOption(arg[2:])
		if !ok {
			return rmArg{}
		}
		return rmArg{options: []string{name}, known: true}
	case len(arg) > 1 && arg[0] == '-':
		a := rmArg{known: true}
		for _, c := range arg[1:] {
			name, ok := rmShortOptions[c]
			if !ok {
				name = string(c)
			}
			a.options = append(a.options, name)
		}
		return a
	}
	return rmArg{operand: true, known: true}
}

// rmLongOption names the long option a word gives without its dashes: an
// exact name or a unique prefix of one, with any `=value` dropped.
func rmLongOption(word string) (string, bool) {
	prefix, _, _ := strings.Cut(word, "=")
	if prefix == "" {
		return "", false
	}
	if slices.Contains(rmLongOptions, prefix) {
		return prefix, true
	}
	found := ""
	for _, name := range rmLongOptions {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		if found != "" {
			return "", false
		}
		found = name
	}
	return found, found != ""
}

// rmRuns reports whether rm's arguments ask for every option and operand the
// deny entry's own rm arguments name: `rm -rf` is rm with recursive and
// force, whatever the spelling.
func rmRuns(entry, args []string) bool {
	want, _ := readRm(entry)
	have, _ := readRm(args)
	for name := range want.options {
		if !have.options[name] {
			return false
		}
	}
	for _, operand := range want.operands {
		if !slices.Contains(have.operands, operand) {
			return false
		}
	}
	return true
}

// verbWord is a word that may name a verb. optional marks one the verbs may
// skip: a word right after a flag, which may be that flag's value instead,
// and a NAME=value word.
type verbWord struct {
	text     string
	optional bool
}

// verbsRun reports whether a tool's arguments reach the entry's verbs in
// order. A flag before or between the verbs is skipped, and any flag may
// take the next word as its value, so a global flag the guard does not know
// (`kubectl --context x delete`) still counts as a possible match.
// terraform's and tofu's `apply -destroy` counts as `destroy`, a gcloud
// release track may be skipped, and so may any word before a rake or rails
// task (taskTools).
func verbsRun(tool string, verbs, args []string) bool {
	words := verbWords(args)
	for i := range words {
		words[i].optional = words[i].optional || skippable(tool, words[i].text)
	}
	if destroyTool(tool) && destroys(args) {
		for i := range words {
			words[i].text = destroyText(words[i].text, true)
		}
	}
	return reachesVerbs(verbs, words)
}

// skippable reports whether the verbs may skip a word of a tool's arguments
// whatever comes before it: a gcloud release track, and any word before a
// rake or rails task (taskTools).
func skippable(tool, word string) bool {
	return (tool == "gcloud" && gcloudTracks[strings.ToLower(word)]) || taskTools[tool]
}

// destroyTool reports whether a tool reads `apply -destroy` as a destroy
// (destroys): terraform and tofu.
func destroyTool(tool string) bool {
	return tool == "terraform" || tool == "tofu"
}

// destroyText is a verb word as a destroy tool reads it: apply reads as
// destroy when -destroy is on.
func destroyText(word string, on bool) string {
	if on && word == "apply" {
		return "destroy"
	}
	return word
}

// verbWords reads the words that may name a verb: every word but the flags,
// up to `--`, and every word after it. A NAME=value word may be skipped:
// rake and rails take one as an environment setting before a task (`rake
// RAILS_ENV=production db:drop`), and make and others take variables so.
func verbWords(args []string) []verbWord {
	var out []verbWord
	afterFlag := false
	for i, arg := range args {
		if arg == "--" {
			for _, rest := range args[i+1:] {
				out = append(out, verbWord{text: rest})
			}
			return out
		}
		if mayBeFlag(arg) {
			afterFlag = flagTakesNext(arg)
			continue
		}
		out = append(out, verbWord{text: arg, optional: afterFlag || isAssignment(arg)})
		afterFlag = false
	}
	return out
}

// flagTakesNext reports whether a flag may take the next word as its value:
// `--context x` may and `--context=x` does not. The quote-collapsed form of
// `--context="x"` is `--context= x`.
func flagTakesNext(flag string) bool {
	return !strings.Contains(flag, "=") || strings.HasSuffix(flag, "=")
}

// mayBeFlag reports whether a word is a flag or may become one: a word the
// shell expands ($X, "$(…)") may turn into any flag.
func mayBeFlag(word string) bool {
	return (len(word) > 1 && word[0] == '-') || strings.Contains(word, "$")
}

// reachesVerbs reports whether the verbs match the first words in order,
// where an optional word may be skipped as the value of the flag before it.
// reach[j] says the first j verbs can be matched by the words read so far.
func reachesVerbs(verbs []string, words []verbWord) bool {
	reach := make([]bool, len(verbs)+1)
	reach[0] = true
	for _, w := range words {
		if reach[len(verbs)] {
			return true
		}
		next := make([]bool, len(verbs)+1)
		for j, verb := range verbs {
			if !reach[j] {
				continue
			}
			if strings.EqualFold(w.text, verb) {
				next[j+1] = true
			}
			if w.optional {
				next[j] = true
			}
		}
		reach = next
	}
	return reach[len(verbs)]
}

// destroys reports whether terraform's (or tofu's) arguments turn on
// -destroy, which makes apply a destroy. terraform reads flags with Go's flag package: one
// or two dashes, an optional boolean value, and the last one wins. A value
// it cannot read counts as on, once the quotes the line as written leaves
// around it are dropped.
func destroys(args []string) bool {
	on := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if v, ok := destroyFlag(arg); ok {
			on = v
		}
	}
	return on
}

// destroyFlag reads one argument as terraform's -destroy flag: ok when it is
// that flag, and on when it turns destroy on.
func destroyFlag(arg string) (on, ok bool) {
	if !strings.HasPrefix(arg, "-") {
		return false, false
	}
	name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
	if name != "destroy" {
		return false, false
	}
	parsed, err := strconv.ParseBool(strings.Trim(value, `'"`))
	return !hasValue || err != nil || parsed, true
}

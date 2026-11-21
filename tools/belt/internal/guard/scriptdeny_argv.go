package guard

import (
	"slices"
	"strconv"
	"strings"
)

// This file reads a literal deny entry the way the tool it names reads its
// arguments, so a spelling the text match misses still counts: rm's options
// in any shape (`rm -r -f` and `rm --recursive --force` for `rm -rf`), and
// the flags before a kubectl, gcloud or terraform verb (`kubectl -n prod
// delete` for `kubectl delete`). Every other entry matches as text only.

// verbTools are the tools whose deny entries name verbs, read past the flags
// before and between them.
var verbTools = map[string]bool{"gcloud": true, "kubectl": true, "terraform": true}

// gcloudTracks are gcloud's release tracks. A track group runs the same
// command (`gcloud beta sql instances delete`), so it may be skipped.
var gcloudTracks = map[string]bool{"alpha": true, "beta": true, "preview": true}

// commandBreaks end one command inside a line: list and pipe operators,
// subshell and substitution parentheses, and backticks.
const commandBreaks = ";|&()`"

// argvEntry returns a deny entry's words when the argv match reads it: an
// rm entry whose long options rm knows, or a verb-tool entry made of verbs
// only. Any other entry returns nil and keeps the text match alone.
func argvEntry(words []string) []string {
	switch tool := strings.ToLower(words[0]); {
	case tool == "rm":
		if _, known := readRm(words[1:]); known {
			return words
		}
	case verbTools[tool]:
		for _, w := range words[1:] {
			if strings.HasPrefix(w, "-") {
				return nil
			}
		}
		return words
	}
	return nil
}

// runsWords reports whether one command, its words with the command name
// first, runs what the deny entry names, read the way the tool reads its
// arguments. An entry the argv match does not read never matches here.
func (d deny) runsWords(words []string) bool {
	if len(d.argv) == 0 || len(words) == 0 || !strings.EqualFold(commandName(words[0]), d.argv[0]) {
		return false
	}
	tool := strings.ToLower(d.argv[0])
	if tool == "rm" {
		return rmRuns(d.argv[1:], words[1:])
	}
	return verbsRun(tool, d.argv[1:], words[1:])
}

// runsIn reports whether any of a line's commands runs the deny entry,
// starting at any of its words, the way the text match finds an entry
// anywhere on a line: `sudo rm -r -f`, `xargs -0 rm -r -f` and `find -exec
// rm -r -f` all count.
func (d deny) runsIn(cmds [][]string) bool {
	if len(d.argv) == 0 {
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
		switch {
		case arg == "--":
			r.operands = append(r.operands, args[i+1:]...)
			return r, known
		case strings.HasPrefix(arg, "--"):
			name, ok := rmLongOption(arg[2:])
			if ok {
				r.options[name] = true
			}
			known = known && ok
		case len(arg) > 1 && arg[0] == '-':
			for _, c := range arg[1:] {
				name, ok := rmShortOptions[c]
				if !ok {
					name = string(c)
				}
				r.options[name] = true
			}
		default:
			r.operands = append(r.operands, arg)
		}
	}
	return r, known
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

// verbWord is a word that may name a verb. optional marks one right after a
// flag, which may be that flag's value instead.
type verbWord struct {
	text     string
	optional bool
}

// verbsRun reports whether a verb tool's arguments reach the entry's verbs
// in order. A flag before or between the verbs is skipped, and any flag may
// take the next word as its value, so a global flag the guard does not know
// (`kubectl --context x delete`) still counts as a possible match.
// terraform's `apply -destroy` counts as `destroy`, and a gcloud release
// track may be skipped.
func verbsRun(tool string, verbs, args []string) bool {
	words := verbWords(args)
	if tool == "gcloud" {
		for i := range words {
			words[i].optional = words[i].optional || gcloudTracks[strings.ToLower(words[i].text)]
		}
	}
	if tool == "terraform" && destroys(args) {
		for i := range words {
			if words[i].text == "apply" {
				words[i].text = "destroy"
			}
		}
	}
	return reachesVerbs(verbs, words)
}

// verbWords reads the words that may name a verb: every word but the flags,
// up to `--`, and every word after it.
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
			// `--context x` may take the next word and `--context=x` does not.
			// The quote-collapsed form of `--context="x"` is `--context= x`.
			afterFlag = !strings.Contains(arg, "=") || strings.HasSuffix(arg, "=")
			continue
		}
		out = append(out, verbWord{text: arg, optional: afterFlag})
		afterFlag = false
	}
	return out
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

// destroys reports whether terraform's arguments turn on -destroy, which
// makes apply a destroy. terraform reads flags with Go's flag package: one
// or two dashes, an optional boolean value, and the last one wins. A value
// it cannot read counts as on, once the quotes the line as written leaves
// around it are dropped.
func destroys(args []string) bool {
	on := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if name != "destroy" {
			continue
		}
		parsed, err := strconv.ParseBool(strings.Trim(value, `'"`))
		on = !hasValue || err != nil || parsed
	}
	return on
}

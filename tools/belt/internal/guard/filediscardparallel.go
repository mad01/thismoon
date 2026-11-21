package guard

import (
	"cmp"
	"regexp"
	"slices"
	"strings"
)

// This file holds GNU parallel, a consumer like xargs: the command it runs
// takes each path an input source hands it, one per line of standard input
// or of an argument file, or one per ::: word. The option list is GNU
// parallel 20240222's manual.

// parallelCall is a parallel command read for file-discard: the command it
// runs, the ::: words it hands that command, the files :::: and -a read
// paths from (`-` for standard input), the text each path replaces in the
// command, whether -q keeps the command's words as they are, whether
// --dry-run only prints the jobs, the bytes at which it cuts its input into
// paths, and why belt cannot read it.
type parallelCall struct {
	command []shWord
	words   []shWord
	files   []shWord
	replace string
	quote   bool
	dryRun  bool
	split   string
	why     string
}

// The GNU parallel options file-discard reads besides those that change
// what it hands the command (-0, -q, -a, -d, -I, --trim, --dry-run): the
// long ones that take no value and the ones that take one, then the short
// ones of each. parallel reads short options bundled (-0j4) and stops at
// the first word that is no option. Any other option, an abbreviated long
// one included, is one belt does not read.
var (
	parallelFlags = wordSet("bar eta progress keep-order no-keep-order group ungroup line-buffer lb " +
		"verbose tag will-cite no-notice no-run-if-empty xargs exit shuf link xapply silent " +
		"interactive open-tty files files0 output-as-files outputasfiles color color-failed cf " +
		"latest-line ll")
	parallelValues = wordSet("jobs max-procs joblog jl halt halt-on-error delay timeout retries nice " +
		"load memfree memsuspend max-args max-replace-args max-chars tmpdir results res env " +
		"tagstring ctagstring process-slot-var term-seq total-jobs total limit")
)

const (
	parallelShortFlags  = "kutvrmXxpo"
	parallelShortValues = "jPnNsE"
)

// parallelSeps are the words that start an input source after the command:
// true for one of argument files.
var parallelSeps = map[string]bool{":::": false, ":::+": false, "::::": true, "::::+": true}

// parallelReplacement is a replacement string parallel fills in, such as
// {.}, {/}, {#}, {1} or {= perl =}; belt reads only the plain one.
var parallelReplacement = regexp.MustCompile(`\{[^{}]*\}`)

// readParallel reads parallel's options, then the command and its input
// sources.
func readParallel(args []shWord) parallelCall {
	x := parallelCall{replace: "{}", split: "\n"}
	i := 0
	for ; i < len(args) && x.why == ""; i++ {
		v, ok := args[i].literal()
		if !ok || len(v) < 2 || v[0] != '-' {
			break
		}
		if v == "--" {
			i++
			break
		}
		if strings.HasPrefix(v, "--") {
			x.readLong(v[2:], args, &i)
			continue
		}
		x.readShorts(v, args, &i)
	}
	if x.why == "" {
		x.readSources(args[min(i, len(args)):])
	}
	return x
}

// readLong reads one long option at args[*i], and its value.
func (x *parallelCall) readLong(spec string, args []shWord, i *int) {
	name, val, inline := strings.Cut(spec, "=")
	value := func() shWord {
		switch {
		case inline:
			return literalWord(val)
		case *i+1 < len(args):
			*i++
			return args[*i]
		}
		x.why = "parallel's --" + name + " takes a value the command does not give"
		return shWord{}
	}
	switch {
	case parallelFlags[name] && !inline:
	case name == "null" && !inline:
		x.split = ""
	case name == "quote" && !inline:
		x.quote = true
	case name == "dry-run" && !inline:
		x.dryRun = true
	case parallelValues[name]:
		value()
	case name == "arg-file":
		x.argFile(value())
	case name == "delimiter":
		x.delimiter(value())
	case name == "trim":
		x.trim(value())
	case name == "replace" && inline:
		x.replace = cmp.Or(val, "{}")
	default:
		x.why = "belt does not read parallel's --" + spec
	}
}

// readShorts reads one cluster of short options at args[*i]. A value
// option takes the rest of the cluster or the next word; -i and -e take a
// value only in the same word, and belt does not read them without one.
func (x *parallelCall) readShorts(v string, args []shWord, i *int) {
	for k := 1; k < len(v); k++ {
		c := v[k]
		switch {
		case c == '0':
			x.split = ""
		case c == 'q':
			x.quote = true
		case strings.IndexByte(parallelShortFlags, c) >= 0:
		case c == 'i' && k+1 < len(v):
			x.replace = v[k+1:]
			return
		case c == 'e' && k+1 < len(v):
			return
		case strings.IndexByte(parallelShortValues+"adI", c) >= 0:
			w := literalWord(v[k+1:])
			if k+1 == len(v) {
				if *i+1 >= len(args) {
					x.why = "parallel's -" + string(c) + " takes a value the command does not give"
					return
				}
				*i++
				w = args[*i]
			}
			x.shortValue(c, w)
			return
		default:
			x.why = "belt does not read parallel's -" + string(c)
			return
		}
	}
}

// shortValue takes the value of a short option that changes what parallel
// hands the command.
func (x *parallelCall) shortValue(c byte, w shWord) {
	switch c {
	case 'a':
		x.argFile(w)
	case 'd':
		x.delimiter(w)
	case 'I':
		v, ok := w.literal()
		if !ok || v == "" {
			x.why = "belt cannot read the parallel replacement string `" + w.raw + "`"
			return
		}
		x.replace = v
	}
}

// argFile records an -a file; a leading + links it to the one before,
// which hands the same paths.
func (x *parallelCall) argFile(w shWord) {
	if v, ok := w.literal(); ok && strings.HasPrefix(v, "+") {
		w = literalWord(v[1:])
	}
	x.files = append(x.files, w)
}

// delimiter reads -d: one byte, or a backslash escape (delimByte).
func (x *parallelCall) delimiter(w shWord) {
	v, known := w.literal()
	b, ok := delimByte(v)
	switch {
	case !known || !ok:
		x.why = "belt cannot read the parallel delimiter `" + w.raw + "`"
	case b == 0:
		x.split = ""
	default:
		x.split = string(b)
	}
}

// trim reads --trim: only n, the default, leaves each path as it is.
func (x *parallelCall) trim(w shWord) {
	if v, ok := w.literal(); !ok || v != "n" {
		x.why = "parallel --trim " + w.raw + " trims blanks off each path, so belt cannot tell which paths it names"
	}
}

// readSources splits the words after the options into the command and the
// ::: words and :::: files after it.
func (x *parallelCall) readSources(rest []shWord) {
	at := slices.IndexFunc(rest, isParallelSep)
	if at < 0 {
		x.command = rest
		return
	}
	x.command = rest[:at]
	files := false
	for _, w := range rest[at:] {
		if v, ok := w.literal(); ok && isParallelSep(w) {
			files = parallelSeps[v]
			continue
		}
		if files {
			x.files = append(x.files, w)
		} else {
			x.words = append(x.words, w)
		}
	}
}

func isParallelSep(w shWord) bool {
	v, ok := w.literal()
	_, sep := parallelSeps[v]
	return ok && sep
}

// commandWords returns the words of the command parallel runs, and their
// text. Without -q, parallel joins the words with blanks and a shell reads
// them again, so the quotes in them are that shell's to read; ok is false
// when that text is not one simple command.
func (x parallelCall) commandWords() (words []shWord, text string, ok bool) {
	parts := make([]string, len(x.command))
	for i, w := range x.command {
		parts[i] = w.raw
		if v, lit := w.literal(); lit && !x.quote {
			parts[i] = v
		}
	}
	text = strings.Join(parts, " ")
	if x.quote {
		return x.command, text, true
	}
	cmds, read := readShell(text)
	if !read || len(cmds) != 1 || len(cmds[0].subshells) > 0 || len(cmds[0].docs) > 0 {
		return nil, text, len(strings.TrimSpace(text)) == 0
	}
	return plainWords(cmds[0].words), text, true
}

// parallel judges a GNU parallel command, with pre the words in front of
// it: the command it runs (discardJudge.consumer) takes each ::: word and
// each line of each input it reads paths from (parallelCall.feeds), at the -I
// text or {} or after the command's own words, cut at each newline, at a
// NUL with -0, or at the -d byte. With no command, or one that starts with
// the replacement string, each input is a command of its own. --dry-run
// runs nothing.
func (j *discardJudge) parallel(pre, args []shWord, s walkStep) *fileLoss {
	for _, w := range pre {
		if name, _, ok := w.assignment(); ok && strings.HasPrefix(name, "PARALLEL") {
			return &fileLoss{why: "belt does not read a parallel run with " + name + " set, which may add options"}
		}
	}
	x := readParallel(args)
	switch {
	case x.why != "":
		return &fileLoss{why: x.why}
	case x.dryRun:
		return nil
	}
	cmd, text, ok := x.commandWords()
	switch {
	case !ok && namesLossText(text):
		return &fileLoss{why: "belt does not read the command text `" + text + "` parallel runs, and it names a delete"}
	case !ok:
		return nil
	}
	feeds := x.feeds(s)
	for _, fd := range feeds {
		fd.split = x.split
	}
	return j.consumer(consumerRun{
		who: "parallel", command: cmd, mark: x.replace, appends: true, feeds: feeds, words: x.words,
		marks: regexp.MustCompile(regexp.QuoteMeta(x.replace) + "|" + parallelReplacement.String()),
	}, s)
}

// feeds returns a feed for each input parallel reads paths from: standard
// input when no ::: or :::: names a source, or when one is `-`, and each
// :::: or -a file, which belt reads only as a <( ) of a feeder it reads
// (substFeed).
func (x parallelCall) feeds(s walkStep) []*pathFeed {
	if len(x.words) == 0 && len(x.files) == 0 {
		return []*pathFeed{stdinFeed(s, "parallel")}
	}
	var out []*pathFeed
	for _, f := range x.files {
		v, lit := f.literal()
		body, proc := procBody(f)
		switch {
		case lit && v == "-":
			out = append(out, stdinFeed(s, "parallel"))
		case proc:
			out = append(out, substFeed(f, body))
		default:
			out = append(out, &pathFeed{why: "parallel reads its paths from `" + f.raw + "`, a file belt cannot see"})
		}
	}
	return out
}

// procBody returns the command text of a word that is one whole <( ).
func procBody(w shWord) (string, bool) {
	if len(w.parts) != 1 || w.parts[0].kind != partUnknown || !strings.HasPrefix(w.raw, "<(") ||
		!strings.HasSuffix(w.raw, ")") {
		return "", false
	}
	return w.raw[2 : len(w.raw)-1], true
}

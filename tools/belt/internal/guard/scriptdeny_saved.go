package guard

import (
	"path"
	"path/filepath"
	"strings"
)

// This file denies running a file the same command writes a download or
// decoded text to: `curl -o i.sh URL && sh i.sh`, `curl URL > i.sh; bash
// i.sh`, `base64 -d x > i.sh; sh i.sh`. scanTarget reads the raw command
// for a file written and run in one command, which holds what an echo or a
// heredoc writes. What a fetcher or decoder writes is in no text belt can
// read, so running that file is denied as `curl | sh` is. A file written in
// one call and run in a later one is read from disk then.

// noteSaved records the files a command writes text belt cannot read to:
// its stdout redirections and a fetcher's or decoder's output files, when
// it fetches or decodes (stageSource), and tee's files, when a stage in
// front of it does.
func (w codeWalk) noteSaved(text string, cmds []shellCmd, i int, run []string) {
	files := savedFiles(text, cmds[i], run)
	if len(files) == 0 {
		return
	}
	src := w.stageSource(text, cmds[i])
	if src == "" && len(run) > 0 && filepath.Base(run[0]) == "tee" {
		if feed, ok := pipeFeed(cmds, i); ok {
			src = w.pipelineSource(text, cmds, feed)
		}
	}
	if src == "" {
		return
	}
	for _, file := range files {
		w.saved[resolvePath(file, w.s.in.Cwd)] = src
	}
}

// runsSaved denies a command that runs a file the walk saw saved
// (noteSaved).
func (w codeWalk) runsSaved(text string, c shellCmd, run []string) *Denial {
	for _, file := range runFiles(text, c, run) {
		if src := w.saved[resolvePath(file, w.s.in.Cwd)]; src != "" {
			return savedRunDenial(file, src)
		}
	}
	return nil
}

// printsSaved returns what a saved file holds when a command prints it with
// cat, head or tail, or "".
func (w codeWalk) printsSaved(tokens []string) string {
	words := stripWrappers(tokens)
	if len(words) == 0 || !fileProducers[filepath.Base(words[0])] {
		return ""
	}
	for _, arg := range words[1:] {
		if src := w.saved[resolvePath(arg, w.s.in.Cwd)]; src != "" {
			return src
		}
	}
	return ""
}

// savedRunDenial is the denial for running file, which the same command
// fills with src.
func savedRunDenial(file, src string) *Denial {
	return Reasonf(
		ScriptDenyListID,
		"runs %s, which the same command fills with %s, so nothing can read what would run. "+
			"Write the file in one call and read it, then run it in a later call, so the deny list (and the user) can check it before it executes.",
		file,
		src,
	)
}

// runFiles returns the files a command runs as code: the command itself
// when it is a path, its script file or sourced file (scriptOf), and the
// input redirection targets of one that runs its standard input.
func runFiles(text string, c shellCmd, run []string) []string {
	if len(run) == 0 {
		return nil
	}
	var out []string
	if strings.ContainsRune(run[0], '/') {
		out = append(out, run[0])
	}
	use := scriptOf(run)
	if use.at > 0 && !use.inline {
		out = append(out, run[use.at])
	}
	if use.stdin {
		for _, t := range inputTargets(text, c) {
			out = append(out, t.token())
		}
	}
	return out
}

// savedFiles returns the files a command writes when it fetches or decodes:
// its stdout redirection targets, curl's and wget's output files, a
// decoder's output option, and tee's files.
func savedFiles(text string, c shellCmd, run []string) []string {
	out := outputTargets(text, c)
	if len(run) == 0 {
		return out
	}
	name, args := commandName(run[0]), run[1:]
	switch {
	case name == "curl":
		out = append(out, curlOutputs(args)...)
	case name == "wget":
		out = append(out, wgetOutputs(args)...)
	case name == "tee":
		out = append(out, operands(args)...)
	case decodes(name, args):
		out = append(out, decoderOutputs(args)...)
	}
	return out
}

// outputTargets returns the files a command's stdout redirections name: the
// word after >, >>, >| or &>, not after 2> or another descriptor.
func outputTargets(text string, c shellCmd) []string {
	var out []string
	for _, t := range c.targets {
		i := t.at.from - 1
		for i >= 0 && (text[i] == ' ' || text[i] == '\t') {
			i--
		}
		if i > 0 && text[i] == '|' && text[i-1] == '>' {
			i--
		}
		if i < 0 || text[i] != '>' {
			continue
		}
		for i >= 0 && (text[i] == '>' || text[i] == '&') {
			i--
		}
		if i >= 0 && text[i] >= '2' && text[i] <= '9' {
			continue
		}
		out = append(out, t.token())
	}
	return out
}

// curlValued and wgetValued are the short options of curl and wget that
// take a value, in the same word or the next.
const (
	curlValued = "AbcCdDeEFHKmoPQrTtuUwxXyYz"
	wgetValued = "aABDeiIlOoPQRtTUwX"
)

// curlOutputs returns the files curl writes: each -o (--output) file, and
// with -O (--remote-name) each URL's last path element, inside
// --output-dir when it is set.
func curlOutputs(args []string) []string {
	var files, urls []string
	remote, dir := false, ""
	for i := 0; i < len(args); i++ {
		if v, ok := longValue(args, &i, "--output"); ok {
			files = append(files, v)
			continue
		}
		if v, ok := longValue(args, &i, "--output-dir"); ok {
			dir = v
			continue
		}
		switch a := args[i]; {
		case a == "--remote-name" || a == "--remote-name-all":
			remote = true
		case strings.HasPrefix(a, "--"):
		case len(a) > 1 && a[0] == '-':
			letters, letter, v := shortValue(args, &i, curlValued)
			remote = remote || strings.ContainsRune(letters, 'O')
			if letter == 'o' {
				files = append(files, v)
			}
		case strings.Contains(a, "://"):
			urls = append(urls, a)
		}
	}
	if remote {
		for _, u := range urls {
			files = append(files, urlBase(u))
		}
	}
	return inDir(dir, files)
}

// wgetOutputs returns the files wget writes: the -O (--output-document)
// file, or else each URL's last path element, inside -P
// (--directory-prefix) when it is set.
func wgetOutputs(args []string) []string {
	var urls []string
	doc, docSet, dir := "", false, ""
	for i := 0; i < len(args); i++ {
		if v, ok := longValue(args, &i, "--output-document"); ok {
			doc, docSet = v, true
			continue
		}
		if v, ok := longValue(args, &i, "--directory-prefix"); ok {
			dir = v
			continue
		}
		switch a := args[i]; {
		case strings.HasPrefix(a, "--"):
		case len(a) > 1 && a[0] == '-':
			switch _, letter, v := shortValue(args, &i, wgetValued); letter {
			case 'O':
				doc, docSet = v, true
			case 'P':
				dir = v
			}
		case strings.Contains(a, "://"):
			urls = append(urls, a)
		}
	}
	if docSet {
		return inDir("", []string{doc})
	}
	files := make([]string, 0, len(urls))
	for _, u := range urls {
		files = append(files, urlBase(u))
	}
	return inDir(dir, files)
}

// decoderOutputs returns a decoder's output file options: -o and --output
// (BSD base64), -out (openssl).
func decoderOutputs(args []string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		for _, name := range []string{"-o", "--output", "-out"} {
			if v, ok := longValue(args, &i, name); ok {
				out = append(out, v)
				break
			}
		}
	}
	return inDir("", out)
}

// longValue reads an option that takes a value, given as name=value or as
// name then the value, at args[*i]. It moves *i past a value in the next
// word.
func longValue(args []string, i *int, name string) (string, bool) {
	a := args[*i]
	if v, ok := strings.CutPrefix(a, name+"="); ok {
		return v, true
	}
	if a != name {
		return "", false
	}
	if *i+1 < len(args) {
		*i++
		return args[*i], true
	}
	return "", true
}

// shortValue reads a bundle of short options at args[*i] (`-fsSLo f`,
// `-qO-`): the letters up to the first one in valued, that letter (0 for
// none), and its value, from the rest of the word or the next word. It
// moves *i past a value in the next word.
func shortValue(args []string, i *int, valued string) (letters string, letter byte, value string) {
	a := args[*i]
	for j := 1; j < len(a); j++ {
		if strings.IndexByte(valued, a[j]) < 0 {
			continue
		}
		value = a[j+1:]
		if value == "" && *i+1 < len(args) {
			*i++
			value = args[*i]
		}
		return a[1:j], a[j], value
	}
	return a[1:], 0, ""
}

// urlBase returns the file name curl -O and wget give a URL's download: the
// last element of its path, or index.html for none, as wget names it.
func urlBase(u string) string {
	u, _, _ = strings.Cut(u, "#")
	u, _, _ = strings.Cut(u, "?")
	_, rest, _ := strings.Cut(u, "://")
	i := strings.IndexByte(rest, '/')
	if i < 0 || strings.HasSuffix(rest, "/") {
		return "index.html"
	}
	return path.Base(rest[i:])
}

// inDir returns the files that are not standard output (-), each inside dir
// when it is set and the file is relative.
func inDir(dir string, files []string) []string {
	var out []string
	for _, f := range files {
		switch {
		case f == "" || f == "-":
		case dir != "" && !filepath.IsAbs(f):
			out = append(out, filepath.Join(dir, f))
		default:
			out = append(out, f)
		}
	}
	return out
}

// operands returns the words that are not options.
func operands(args []string) []string {
	var out []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			out = append(out, a)
		}
	}
	return out
}

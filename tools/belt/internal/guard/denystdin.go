package guard

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// This file reads what a command takes on its standard input, for a command
// whose head an entry with a phrase names (`psql * DROP *`): the SQL in a
// heredoc, a here-string, a `<` file, or the text a printer pipes into it
// (echo, printf, or a cat, head, tail or tac of a file) counts as one more
// argument for the phrases (readingDeny), and so does what the group, the
// ( ), an earlier exec, the caller of a nested shell or the call of a
// function around it gives it (denystdin_shell.go), and what a substitution
// that prints a file or text and xargs hand it (denystdin_subst.go). Only
// such a head has a file read, and only a regular file. Both deny guards
// read it the same way: direct-deny-list for the commands a call runs
// itself, script-deny-list for nested code (a -c string, a substitution) and
// the script files it reads. Input belt cannot read fails closed: a file it
// cannot open or place, a descriptor or a device, a text past maxScriptSize,
// a heredoc whose body lies past the nested code it is in. A pipe from any
// other command (`pg_dump … | psql`), any other substitution (`$(date)`) and
// a variable (`"$SQL"`) add nothing.

// maxCallInput bounds the standard input text one call's reads take in all,
// so a call that names a large file many times cannot hold the hook past its
// timeout. Past it, the input reads as unread.
const maxCallInput = 8 * maxScriptSize

// phraseTools returns the tools an entry with a phrase names, in lower case.
func phraseTools(patterns []deny) map[string]bool {
	tools := map[string]bool{}
	for _, d := range patterns {
		if d.entry != nil && len(d.entry.phrases) > 0 {
			tools[d.entry.tool] = true
		}
	}
	return tools
}

// phraseHead returns the tool of tools (phraseTools) a command, its words
// with the name first, runs: its own name, or a word behind its launcher
// (launches), which hands the launched command its standard input. "" when
// none.
func phraseHead(run []string, tools map[string]bool) string {
	if len(run) == 0 || len(tools) == 0 {
		return ""
	}
	name := func(w string) string { return strings.ToLower(commandName(shellValue(w))) }
	if tool := name(run[0]); tools[tool] {
		return tool
	}
	if !launches(run) {
		return ""
	}
	for _, w := range run[1:] {
		if tool := name(w); tools[tool] {
			return tool
		}
	}
	return ""
}

// inputSource is one text a command reads on its standard input, or as an
// argument when arg is set (a substitution in its words, what xargs hands
// it): what it is, for a denial (`the heredoc`, `the file x.sql`), and its
// text, or why belt cannot read it.
type inputSource struct {
	what, text, unread string
	arg                bool
}

// runDir is where a command runs: dir ("" when unknown), whether that is
// sure (dirSure), and when it is not, alt, where the shell was before a
// move that may not have run (`cd x; psql < f`), "" for none.
type runDir struct {
	dir, alt string
	sure     bool
}

// inputAt is one command readShell read: the text and the commands it was
// read from, its index among them, and where it runs (runDir). nested marks
// a ( ) body or other nested code, where a heredoc with no end line has its
// body past the text. outer is where the command takes its standard input
// from around it, nil for none: the closer of the innermost { } group or
// compound command it is in, or the command whose ( ) body the text is
// (stdinWalk). stdin is what an exec with no command left on descriptor 0
// before it, nil for nothing belt reads. For the closer of a group, body
// counts the commands of the group in front of it, whose first one a pipe
// into the group reaches, and stdin is what the group starts with, for that
// pipe alone; body is 0 for any other command. fns are the functions the
// shell knows there (shellFuncs).
type inputAt struct {
	text string
	cmds []shellCmd
	i    int
	runDir
	nested bool
	outer  *inputAt
	stdin  *fdSource
	body   int
	fns    *shellFuncs
}

// inputReader reads standard input for the commands of one call. home
// places a ~ path; left is how many bytes the reads may still take
// (maxCallInput), and calls how many commands of function bodies
// (maxFunctionReads). root is the folder the reads take for the root of the
// file system, "" for the real one, so a test reads no file of the host.
type inputReader struct {
	home, root string
	left       int
	calls      int
}

// newInputReader starts the reads of one call, under root (inputReader).
func newInputReader(root string) *inputReader {
	home, _ := os.UserHomeDir()
	return &inputReader{home: home, root: root, left: maxCallInput, calls: maxFunctionReads}
}

// inputHit is what one command's standard input gives the deny list: the
// entry it runs once a source counts as one more argument (readingDeny),
// with that source, or a source belt cannot read, with the first entry
// with a phrase for its tool.
type inputHit struct {
	d    *deny
	src  inputSource
	tool string
}

// check reads the command at at, run being its words past its wrappers
// (unwrapCommand), with each source of its standard input, and what the
// substitutions in its words print (argSources), as one more argument. An
// xargs hands the command it runs what it reads (xargsHit), and a call of a
// function the shell knows hands the commands of its body what the call
// reads (callHit). Any other command reads no input at all.
func (r *inputReader) check(
	at inputAt,
	run []string,
	patterns []deny,
	tools map[string]bool,
) *inputHit {
	return r.checkCall(at, run, patterns, tools, 0)
}

// checkCall is check for a command depth function calls deep.
func (r *inputReader) checkCall(
	at inputAt,
	run []string,
	patterns []deny,
	tools map[string]bool,
	depth int,
) *inputHit {
	if len(tools) == 0 {
		return nil
	}
	if tool := phraseHead(run, tools); tool != "" {
		return firstHit(run, tool, append(r.sources(at), r.argSources(at, run)...), patterns)
	}
	if hit := r.xargsHit(at, run, patterns, tools); hit != nil {
		return hit
	}
	return r.callHit(at, run, patterns, tools, depth)
}

// firstHit returns the hit srcs give run, a command of tool: the entry run
// runs once their texts, one after the other as zsh's MULTIOS reads them,
// count as one more argument (readingDeny), else the first source belt
// cannot read, with the first entry with a phrase for the tool. nil for
// none. The texts are read in one pass, so a command with many sources and
// long words stays linear (blame).
func firstHit(run []string, tool string, srcs []inputSource, patterns []deny) *inputHit {
	var texts []string
	unread := -1
	for i, src := range srcs {
		switch {
		case src.unread != "" && unread < 0:
			unread = i
		case src.text != "":
			texts = append(texts, src.text)
		}
	}
	if len(texts) > 0 {
		if d := readingDeny(run, strings.Join(texts, "\n"), patterns); d != nil {
			return &inputHit{d: d, src: blame(tool, srcs, d), tool: tool}
		}
	}
	if unread >= 0 {
		return &inputHit{d: firstPhrase(patterns, tool), src: srcs[unread], tool: tool}
	}
	return nil
}

// blame returns the source a denial for d names: the first whose text alone
// holds d's phrases for a bare tool, else the first with a text.
func blame(tool string, srcs []inputSource, d *deny) inputSource {
	first := -1
	for i, src := range srcs {
		switch {
		case src.text == "":
		case d.entry != nil && d.entry.runsReading([]string{tool}, src.text):
			return src
		case first < 0:
			first = i
		}
	}
	return srcs[first]
}

// unreadHit is the hit for a phrase head in text readShell cannot follow
// whose words hold a `<`: belt cannot tell what it reads on standard input.
// nil for any other command.
func unreadHit(run []string, patterns []deny, tools map[string]bool) *inputHit {
	tool := phraseHead(run, tools)
	if tool == "" {
		return nil
	}
	for _, w := range run {
		if strings.Contains(w, "<") {
			src := inputSource{
				what:   "a redirection",
				unread: "belt cannot follow the shell text around it",
			}
			return &inputHit{d: firstPhrase(patterns, tool), src: src, tool: tool}
		}
	}
	return nil
}

// firstPhrase returns the first entry with a phrase that names tool.
func firstPhrase(patterns []deny, tool string) *deny {
	for i, d := range patterns {
		if d.entry != nil && len(d.entry.phrases) > 0 && d.entry.tool == tool {
			return &patterns[i]
		}
	}
	return nil
}

// sources returns what the command reads on its standard input: each
// heredoc, here-string and `<` file on descriptor 0, then what the pipeline
// in front of it prints (printed), then what an exec left on descriptor 0
// (stdin), then what the group or the ( ) it is in reads (outer). bash reads
// the last of its own; zsh, with its MULTIOS option on as it is by default,
// reads all of them, a pipe included. So every one counts.
func (r *inputReader) sources(at inputAt) []inputSource {
	c := at.cmds[at.i]
	var out []inputSource
	for k, op := range c.redirs {
		if redirFd(op) == "0" {
			out = append(out, r.redirSources(at, k)...)
		}
	}
	out = append(out, r.printed(at)...)
	if at.stdin != nil && at.body == 0 {
		out = append(out, r.fdSources(*at.stdin)...)
	}
	if at.outer != nil {
		out = append(out, r.sources(*at.outer)...)
	}
	return out
}

// redirSources returns what redirection k of the command at gives the
// descriptor it opens: a heredoc's body, a here-string's text, a file's
// text, or for a copy of another descriptor (`<&3`) a source belt does not
// read. A closed descriptor and an output file give nothing. What the
// substitutions in a here-string or an unquoted heredoc body print counts
// too (wordSubsts, docSubsts).
func (r *inputReader) redirSources(at inputAt, k int) []inputSource {
	c := at.cmds[at.i]
	target := c.targets[k]
	switch c.redirs[k].op {
	case "<<", "<<-":
		doc := 0
		for _, op := range c.redirs[:k] {
			if op.op == "<<" || op.op == "<<-" {
				doc++
			}
		}
		src := r.docSource(at, doc)
		if src.unread != "" {
			return []inputSource{src}
		}
		return append([]inputSource{src}, r.docSubsts(at, c.docs[doc])...)
	case "<<<":
		src := r.take(inputSource{what: "the here-string", text: hereText(target)})
		return append([]inputSource{src}, r.wordSubsts(at, target)...)
	case "<", "<>":
		return r.fileSource("the file "+target.raw, target, at)
	case "<&", ">&":
		if v, _ := target.literal(); v != "-" {
			return []inputSource{{
				what:   "descriptor " + target.raw,
				unread: "belt does not follow what another descriptor holds",
			}}
		}
	}
	return nil
}

// fdSources returns what an exec left on a descriptor (fdSource), or for
// the standard input a nested shell starts with, what its caller reads
// there.
func (r *inputReader) fdSources(s fdSource) []inputSource {
	if s.from != nil {
		return r.sources(*s.from)
	}
	if s.at == nil {
		return []inputSource{{what: s.what, unread: "belt does not follow what another descriptor holds"}}
	}
	return r.redirSources(*s.at, s.k)
}

// docSource returns the command's heredoc k, counted among all of its
// heredocs. In nested code a body that runs to the end of the text has no
// end line there: the shell reads it past the ( ) or the substitution it is
// in, where belt does not.
func (r *inputReader) docSource(at inputAt, k int) inputSource {
	c := at.cmds[at.i]
	src := inputSource{what: "the heredoc"}
	switch {
	case k >= len(c.docs):
		src.unread = "belt cannot tell where its body is"
	case at.nested && c.docs[k].body.to == len(at.text):
		src.unread = "its body lies past the nested code it is in"
	default:
		src.text = at.text[c.docs[k].body.from:c.docs[k].body.to]
	}
	return r.take(src)
}

// hereText returns the text a here-string word gives: its value when it
// holds no expansion but $'...', else the word as written, whose literal
// parts still hold any statement it names.
func hereText(w placedWord) string {
	if v, ok := w.literalANSIC(); ok {
		return v
	}
	return w.raw
}

// take charges a source's text to the call's reads: a text past
// maxScriptSize, or past what the call may still read, reads as unread.
func (r *inputReader) take(s inputSource) inputSource {
	switch {
	case s.unread != "":
	case len(s.text) > maxScriptSize:
		s.text, s.unread = "", "it is over 1 MiB, more than belt reads"
	case len(s.text) > r.left:
		s.text, s.unread = "", "the call takes more than 8 MiB on standard input, more than belt reads"
	default:
		r.left -= len(s.text)
	}
	return s
}

// fileSource reads the file a word names, once for each path it may be
// (place, readFile).
func (r *inputReader) fileSource(what string, w placedWord, at inputAt) []inputSource {
	src := inputSource{what: what}
	paths, why := r.place(w, at)
	if why != "" {
		src.unread = why
		return []inputSource{src}
	}
	out := make([]inputSource, 0, len(paths))
	for _, p := range paths {
		out = append(out, r.take(r.readFile(src, p)))
	}
	return out
}

// place returns the paths a file word may name, placed from the directory
// the command runs in, the way file-discard places an operand
// (rmScan.staticPath), or why belt cannot place it: an expansion other than
// a leading ~ or $HOME, such as a variable, a substitution or a glob, or a
// relative path where belt cannot tell the directory. After a move that may
// not have run (`cd x; psql < f`), a relative path is placed from both
// directories the command may run in (runDir.alt).
func (r *inputReader) place(w placedWord, at inputAt) ([]string, string) {
	p, glob, ok := (&rmScan{home: r.home}).staticPath(w.shWord)
	switch {
	case !ok || glob:
		return nil, "its path holds a variable, a substitution, a glob or another expansion only the shell can read"
	case path.IsAbs(p):
		return []string{path.Clean(p)}, ""
	case at.dir == "" || !at.sure && at.alt == "":
		return nil, "belt cannot tell which folder it is in: name the path in full, " +
			"or join the cd to the command with &&"
	case at.sure || at.alt == at.dir:
		return []string{path.Join(at.dir, p)}, ""
	}
	return []string{path.Join(at.dir, p), path.Join(at.alt, p)}, ""
}

// readFile reads the file at p into the source, up to one byte past what
// take keeps. /dev/null is empty. Anything else belt cannot read now is
// unread, and belt opens a regular file alone, since it opens the file in
// its own process:
//   - a missing file, since an earlier step of the call may make it without
//     naming it (`make && psql < out.sql`), and the shell runs nothing when
//     it stays missing;
//   - a path, as written or on the way through its links (follow), that
//     names a descriptor or a device (specialPath): /dev/stdin is belt's own
//     input, and /dev/zero or /dev/tty would never end;
//   - a folder, a FIFO, a socket or any other file that is not a regular
//     one, since opening a FIFO blocks;
//   - a file belt cannot open.
func (r *inputReader) readFile(src inputSource, p string) inputSource {
	if p == "/dev/null" {
		return src
	}
	real, why := r.follow(p)
	if why == "" && real == "/dev/null" {
		return src
	}
	var f *os.File
	if why == "" {
		f, why = r.open(real)
	}
	if why != "" {
		src.unread = why
		return src
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, int64(min(maxScriptSize, r.left))+1))
	if err != nil {
		src.unread = "belt cannot read it"
		return src
	}
	src.text = string(b)
	return src
}

// maxLinks bounds how many links follow takes, as the kernel bounds them.
const maxLinks = 40

// follow returns the clean absolute path p names once each link on the way
// is followed, part by part as the kernel follows them, read under r.root,
// or why belt does not read it. Each path on the way is checked before belt
// looks at it on disk (specialPath), so a link to /dev/stdin is never
// opened either.
func (r *inputReader) follow(p string) (string, string) {
	if why := specialPath(p); why != "" {
		return "", why
	}
	done, rest := "/", strings.Split(p, "/")
	for links := 0; len(rest) > 0; {
		part := rest[0]
		rest = rest[1:]
		switch part {
		case "", ".":
			continue
		case "..":
			done = path.Dir(done)
			continue
		}
		next := path.Join(done, part)
		if why := specialPath(next); why != "" {
			return "", why
		}
		if next == "/dev/null" {
			done = next
			continue
		}
		fi, err := os.Lstat(r.onDisk(next))
		if why := missing(err); why != "" {
			return "", why
		}
		if fi.Mode()&fs.ModeSymlink == 0 {
			done = next
			continue
		}
		links++
		target, err := os.Readlink(r.onDisk(next))
		if err != nil || links > maxLinks {
			return "", "belt cannot follow the links to it"
		}
		if path.IsAbs(target) {
			done = "/"
		}
		rest = append(strings.Split(target, "/"), rest...)
	}
	return done, ""
}

// open opens the regular file at p, read under r.root, or says why belt
// does not. It looks at the file before it opens it, opens it without
// following a link or waiting on a FIFO, and looks again at what it opened,
// so a FIFO or a device swapped in between is never read either.
func (r *inputReader) open(p string) (*os.File, string) {
	disk := r.onDisk(p)
	if why := notRegular(os.Lstat(disk)); why != "" {
		return nil, why
	}
	f, err := os.OpenFile(disk, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, "belt cannot open it"
	}
	if why := notRegular(f.Stat()); why != "" {
		_ = f.Close()
		return nil, why
	}
	return f, ""
}

// onDisk returns where the clean absolute path p lies under r.root.
func (r *inputReader) onDisk(p string) string {
	if r.root == "" {
		return p
	}
	return filepath.Join(r.root, filepath.FromSlash(p))
}

// specialPath says why belt does not open the clean absolute path p, "" when
// it may: one of the shell's descriptors (/dev/stdin, /dev/fd/N,
// /proc/self/fd/N), which belt would open as its own, or any other path
// under /dev or /proc but /dev/null, a device or a file of the process
// table, such as /dev/zero or /dev/tty, whose read may never end.
func specialPath(p string) string {
	parts := strings.Split(p, "/")
	switch {
	case p == "/dev/null":
		return ""
	case p == "/dev/stdin", p == "/dev/stdout", p == "/dev/stderr",
		strings.HasPrefix(p, "/dev/fd/"), len(parts) > 4 && parts[1] == "proc" && slices.Contains(parts[3:], "fd"):
		return "it is one of the shell's descriptors, which belt does not open"
	case strings.HasPrefix(p, "/dev/"), strings.HasPrefix(p, "/proc/"):
		return "it is a device or a file of the process table, which belt does not open"
	}
	return ""
}

// missing says why belt cannot look at a file whose lookup failed with err,
// "" for none (notRegular).
func missing(err error) string {
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return "it does not exist yet, and an earlier step may make it"
	case err != nil:
		return "belt cannot open it"
	}
	return ""
}

// notRegular says why belt does not read the file fi describes, "" for a
// regular file.
func notRegular(fi fs.FileInfo, err error) string {
	switch {
	case err != nil:
		return missing(err)
	case fi.IsDir():
		return "it is a folder"
	case !fi.Mode().IsRegular():
		return "it is not a regular file: belt does not open a FIFO, a socket or a device"
	}
	return ""
}

// printed returns what the pipeline in front of the command prints into
// it, as far as belt reads it (prints). Any other command in front is a
// source belt does not read, and adds nothing. For a group's closer, the
// pipeline is the one in front of the group (inputAt.body).
func (r *inputReader) printed(at inputAt) []inputSource {
	first := at.i - at.body
	if first == 0 || at.cmds[first].before != jointPipe {
		return nil
	}
	front := at
	front.i, front.body = first-1, 0
	return r.prints(front, "into it")
}

// prints returns what the command at prints, as far as belt reads it, with
// into naming where it prints, for a denial: a printer's text
// (shellWalk.produced: echo, printf, or cat, tee, head or tail printing a
// heredoc or a here-string) and what the substitutions in it print
// (printerSubsts), the files a cat, head, tail or tac names (fileArgs), and
// what one that names none, or a tee, hands on from its own standard input
// (sources). A printer whose words hold an expansion belt cannot read gives
// its words as written, and what its substitutions print; one whose text
// belt reads whole already holds what its substitutions print. Any other
// command gives nothing.
func (r *inputReader) prints(at inputAt, into string) []inputSource {
	p := at.cmds[at.i]
	tokens := p.tokens()
	i := runAt(tokens)
	if i >= len(tokens) {
		return nil
	}
	head := filepath.Base(tokens[i])
	if s, known := newShellWalk("", "").produced(at.text, p); known {
		src := inputSource{what: fmt.Sprintf("the text `%s` prints %s", head, into), text: s.text}
		if s.ok {
			return []inputSource{r.take(src)}
		}
		src.text += "\n" + commandText(at.text, p)
		return append([]inputSource{r.take(src)}, r.printerSubsts(at)...)
	}
	switch head {
	case "cat", "head", "tail", "tac":
		return r.fileArgs(at, head, p.words[i+1:], into)
	case "tee":
		return r.sources(at)
	}
	return nil
}

// fileValued are the options of head, tail and tac, by tool, marked as
// wrapperSpec marks them: GNU's from coreutils 9's --help and BSD's from
// the macOS man pages. cat takes no value. A word fileArgs cannot read as
// options of the tool, such as `-100`, takes no value either.
var fileValued = map[string]wrapperSpec{
	"head": {short: "c:n:qvz", long: "bytes: lines: quiet silent verbose zero-terminated"},
	"tail": {
		short: "b:c:fFn:qrs:vz",
		long: "bytes: follow:: lines: pid: quiet silent retry sleep-interval: verbose " +
			"zero-terminated max-unchanged-stats:",
	},
	"tac": {short: "brs:", long: "before regex separator:"},
}

// fileArgs returns what a cat, head, tail or tac, its words after the name
// args, prints: each file it names, whole, and its own standard input when
// it names none or names - (sources). An option's value is no file.
func (r *inputReader) fileArgs(at inputAt, head string, args []placedWord, into string) []inputSource {
	var out []inputSource
	stdin, files, options := false, false, true
	for k := 0; k < len(args); k++ {
		w := args[k]
		switch v := w.token(); {
		case options && v == "--":
			options = false
		case v == "-":
			stdin = true
		case options && strings.HasPrefix(v, "-"):
			if takesValue(fileValued[head], v) {
				k++
			}
		default:
			files = true
			what := fmt.Sprintf("the file %s that `%s` prints %s", w.raw, head, into)
			out = append(out, r.fileSource(what, w, at)...)
		}
	}
	if stdin || !files {
		out = append(out, r.sources(at)...)
	}
	return out
}

// takesValue reports whether an option word of spec ends in an option that
// takes its value from the next word (`-n`, `-qn`, `--lines`).
func takesValue(spec wrapperSpec, word string) bool {
	opts, ok := spec.options(word)
	if !ok || len(opts) == 0 {
		return false
	}
	last := opts[len(opts)-1]
	return last.arg == argNeeded && !last.attached
}

// commandText returns a command's words and redirections as written.
func commandText(text string, c shellCmd) string {
	from, to := len(text), 0
	for _, words := range [][]placedWord{c.words, c.targets} {
		for _, w := range words {
			from, to = min(from, w.at.from), max(to, w.at.to)
		}
	}
	if from >= to {
		return ""
	}
	return text[from:to]
}

// enter starts the next command of the text the walk follows for the
// directory alone (placeText): it takes the separator in front of it as
// walkCmds does and returns where the command runs (runDir), and whether
// the command before it moved the shell, for leave.
func (w *shellWalk) enter(c shellCmd) (runDir, bool) {
	w.join(c.before)
	w.joinChain(c.before)
	afterMove := w.justMoved
	w.justMoved = false
	at := runDir{dir: w.dir, sure: w.dirSure()}
	if !at.sure && w.moved {
		at.alt = w.moveFrom
	}
	return at, afterMove
}

// leave moves the walk past a command of text the way walkWords and
// walkCmds move it: a cd, pushd or popd moves it (shellWalk.move), an `||
// exit` after a move settles it (settleMove), any other command may move
// the shell unseen (keepsShellPlaced), and a heredoc leaves later moves
// unknown, as the walk reads them.
func (w *shellWalk) leave(text string, c shellCmd, afterMove bool) {
	if tokens := c.tokens(); len(tokens) > 0 {
		w.cdpath = w.cdpath || mentionsName(strings.Join(tokens, " "), cdpathNames)
		defines := definesFunction(text, c, tokens)
		if target, ok := parseCd(tokens); ok && !defines {
			raw := make([]string, 0, len(c.words))
			for _, word := range c.words {
				raw = append(raw, word.raw)
			}
			from := ""
			if w.dirSure() {
				from = w.dir
			}
			w.move(target, strings.Join(raw, " "), c.before, c.after, false)
			w.justMoved, w.moveFrom = true, from
		} else {
			w.cdHidden = w.cdHidden || !keepsShellPlaced(defines, tokens)
			if afterMove && leavesOnFail(c, tokens) {
				w.settleMove()
			}
		}
	}
	w.heredoc = w.heredoc || len(c.docs) > 0
}

// placeText starts the directory walk of one text, as walkRead does: a text
// that may set PWD, or cd or assign from inside a word, leaves the words
// that name where the shell is unknown.
func (w *shellWalk) placeText(text string) {
	w.pwdSet = w.pwdSet || rewritesPWD(text)
	w.cdHidden = w.cdHidden || expandsInShell(text)
}

// from names where the hit's source reaches the command, for a denial: its
// standard input, or an argument (inputSource.arg).
func (h *inputHit) from() string {
	if h.src.arg {
		return "with an argument from"
	}
	return "with its standard input from"
}

// on is from for a denial that names the entry the source runs.
func (h *inputHit) on() string {
	if h.src.arg {
		return "in its arguments"
	}
	return "on its standard input"
}

// directReason is direct-deny-list's denial for a hit.
func (h *inputHit) directReason() *Denial {
	if h.src.unread != "" {
		return Reasonf(
			DirectDenyListID,
			"this runs %s %s %s, and belt cannot read that: %s. "+
				"The Bash deny list holds %q for the SQL %s runs, so SQL belt cannot read is denied. %s",
			h.tool, h.from(), h.src.what, h.src.unread, h.d.pattern, h.tool, askUserToRun,
		)
	}
	return Reasonf(
		DirectDenyListID,
		"this runs %q from the Bash deny list, with the SQL from %s %s "+
			"(read past global flags and wrappers). It can delete or change state that may not come back, "+
			"so every form of it is denied: a script, a wrapper or an alias too. %s",
		h.d.pattern, h.src.what, h.on(), askUserToRun,
	)
}

// scriptReason is script-deny-list's denial for a hit in where, at line.
func (h *inputHit) scriptReason(where string, line int) *Denial {
	if h.src.unread != "" {
		return Reasonf(
			ScriptDenyListID,
			"%s runs %s (line %d) %s %s, and belt cannot read that: %s. "+
				"The Bash deny list holds %q for the SQL %s runs, so SQL belt cannot read is denied. %s",
			where,
			h.tool,
			line,
			h.from(),
			h.src.what,
			h.src.unread,
			h.d.pattern,
			h.tool,
			h.d.hint(directHint),
		)
	}
	return Reasonf(
		ScriptDenyListID,
		"%s runs %q (line %d) with the SQL from %s %s: "+
			"that command is on the Bash deny list and scripts don't get to bypass it. %s",
		where, h.d.pattern, line, h.src.what, h.on(), h.d.hint(directHint),
	)
}

// lineOf returns the line of text command c starts on, from 1.
func lineOf(text string, c shellCmd) int {
	from := len(text)
	for _, words := range [][]placedWord{c.words, c.targets} {
		for _, w := range words {
			from = min(from, w.at.from)
		}
	}
	return strings.Count(text[:min(from, len(text))], "\n") + 1
}

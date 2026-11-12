package guard

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"

	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// A heredoc that cat or tee writes to a document file is text: the shell
// runs none of its lines. splitSegments still reads every body line as a
// command of its own, so a markdown file whose code block shows `bash -c
// '...'` or `... | xargs rm -rf` was denied as if the command ran them.
// textHeredocBodies finds the bodies the guard reads as text, and check
// skips their segments. A body is text only when all of this holds:
//
//   - The heredoc's line is reached through plain lines only (pyHeadChars:
//     no quote, comment, substitution or continuation that could move where
//     the shell starts a line). It holds one bare cat or tee, optionally
//     after sudo, whose other words are output redirects, tee's -a and -i,
//     and file targets, and the body reaches a file.
//   - Every target, a stderr one included, is a literal path (no variable,
//     glob or ~user) with an extension from textDocExts. Below a temp
//     folder (textTempDirs or ~/.tmp), the payload cwd, the home directory
//     or / (whichever holds it first), no part of the path starts with a
//     dot, and no directory on the way is a symlink. Outside the temp
//     folders it is not under textSystemDirs or ~/Library. When it exists,
//     it is a regular file with one link and no execute bit.
//   - The delimiter is quoted, or the body holds no $, backtick or
//     backslash, and the body does not start with a shebang.
//   - Every cd is `cd PATH` with one literal path, followed by &&. A
//     relative target needs a known directory: the payload cwd, moved only
//     by the cds earlier in the target's own && chain. Once a chain with a
//     cd in it ends, the directory is unknown, as the cd may have failed. A
//     CDPATH in the guard's environment makes a relative cd unknown too.
//   - Outside the bodies there is no pipe, substitution, subshell,
//     background job or CDPATH. Every other step is a textSafeHeads
//     command, a textSafeGit subcommand or a textSafeGh action, with no
//     sudo and no redirect.
//
// So nothing in the command can run, source, copy or chmod the file it
// writes, and the file is no hook, config or startup file another tool runs.

// textSpan is a text heredoc's body and terminator line, as the byte range
// [from, to) of the command.
type textSpan struct{ from, to int }

// textSpans are the text heredoc bodies of one command, in order.
type textSpans []textSpan

// holds reports whether the byte offset falls inside a text heredoc body.
func (t textSpans) holds(off int) bool {
	for _, s := range t {
		if off >= s.from && off < s.to {
			return true
		}
	}
	return false
}

// cut returns the command with the text bodies removed. Each body becomes a
// newline, so the text on either side of it cannot join into a new token.
func (t textSpans) cut(cmd string) string {
	var b strings.Builder
	from := 0
	for _, s := range t {
		b.WriteString(cmd[from:s.from])
		b.WriteByte('\n')
		from = s.to
	}
	b.WriteString(cmd[from:])
	return b.String()
}

var (
	// textDocExts are the extensions a text target may carry. JSON, YAML and
	// TOML are left out: a git hook runner reads package.json or lefthook.yml
	// and runs the commands in it, and mise runs the hooks in mise.toml.
	textDocExts = map[string]bool{
		".md": true, ".markdown": true, ".txt": true, ".rst": true,
		".adoc": true, ".log": true, ".csv": true, ".tsv": true,
	}
	// textSystemDirs hold files the system reads as configuration. They are
	// compared case-folded, as the macOS file system matches names.
	textSystemDirs = []string{
		"/etc", "/private", "/Library", "/System", "/usr", "/opt", "/bin", "/sbin", "/dev",
	}
	// textTempDirs are the temp folders a note or a pull request body goes
	// to. Like ~/.tmp, the payload cwd and home, each is a trusted base, so
	// only the part below it is judged: on macOS /tmp and /var are symlinks
	// into /private, a system directory. $TMPDIR is not one, as the
	// environment sets it, and neither is /var/folders, which holds per-user
	// caches beside the temp folder.
	textTempDirs = []string{"/tmp", "/private/tmp", "/var/tmp", "/private/var/tmp"}
	// textSafeHeads print, list or create files and run none.
	textSafeHeads = map[string]bool{
		"echo": true, "printf": true, "cat": true, "ls": true, "wc": true,
		"pwd": true, "true": true, "mkdir": true, "touch": true,
	}
	// textSafeGit are the git subcommands a text heredoc may sit beside.
	// What they run comes from git config, dotfiles and hook files, and a
	// text target is none of those: its name has a document extension and
	// no part of its path starts with a dot.
	textSafeGit = map[string]bool{
		"add":    true, // stages the file as data; filters come from config and .gitattributes
		"commit": true, // runs hooks, whose names have no extension; a message file is data
		"status": true, // prints the tree state; fsmonitor comes from config
		"diff":   true, // prints changes; drivers, textconv and the pager come from config
		"log":    true, // prints history; the pager and signature checks come from config
		"show":   true, // prints objects; textconv and the pager come from config
	}
	// textGitWrites are the git options that write a file or a mode: --output
	// sends log or diff output to a file, which can put a commit message in
	// a hook, and --chmod sets the execute bit in the index.
	textGitWrites = []string{"--output", "--chmod"}
	// textSafeGh are the gh actions a text heredoc may sit beside. None runs
	// a local program but git, whose push runs hooks only.
	textSafeGh = map[string]map[string]bool{
		// pr sends the body through the API and may push the branch.
		"pr": {"create": true, "edit": true, "comment": true},
		// issue sends the body through the API.
		"issue": {"create": true, "edit": true, "comment": true},
		// release sends notes and assets through the API.
		"release": {"create": true, "edit": true},
		// gist uploads the file as text.
		"gist": {"create": true, "edit": true},
	}
	// textShellMarks open a parameter expansion, a command or process
	// substitution, a subshell or a pipe, or set CDPATH. Outside a text body
	// they keep every body checked.
	textShellMarks = []string{"${", "`", "(", ")", "|", "CDPATH"}
	// teeFlags are the tee options a text sink may carry.
	teeFlags = map[string]bool{
		"-a":                  true,
		"--append":            true,
		"-i":                  true,
		"--ignore-interrupts": true,
	}
	// textRedirect splits an output redirect into its operator and an
	// attached target (>doc.md, 2>>err.log, &>out).
	textRedirect = regexp.MustCompile(`^(&>>?|[12]?>>?)(.*)$`)
	// textPathWord is a path the shell takes as written: no variable, glob,
	// quote or ~user, and no leading dash.
	textPathWord = regexp.MustCompile(`^(~/|/)?[A-Za-z0-9_][A-Za-z0-9_./@+,-]*$`)
)

// textHeredocBodies returns the bodies of the command's text heredocs, or nil
// when the command holds none or anything in it could run a written file. cwd
// places relative targets.
func textHeredocBodies(cmd, cwd string) textSpans {
	home := config.ExpandHome("~")
	if !filepath.IsAbs(home) {
		home = ""
	}
	if !filepath.IsAbs(cwd) {
		cwd = ""
	}
	w := textWalk{
		bases: textBases(cwd, home), home: home, dir: cwd, known: cwd != "",
		cdPath: os.Getenv("CDPATH") != "",
	}
	var spans textSpans
	for pos := 0; pos < len(cmd); {
		end := len(cmd)
		if nl := strings.IndexByte(cmd[pos:], '\n'); nl >= 0 {
			end = pos + nl
		}
		span, next, ok := w.line(cmd, pos, end)
		if !ok {
			break
		}
		if span.to > span.from {
			spans = append(spans, span)
		}
		pos = next
	}
	if len(spans) == 0 || !textSafeCommand(cmd, spans, w.sinks) {
		return nil
	}
	return spans
}

// textWalk follows the plain lines at the start of a command and the
// directory their cd steps move to.
type textWalk struct {
	bases   []string // the directories a target's path is judged below, from textBases
	home    string   // the home directory; "" when unknown
	dir     string   // the shell's directory, when known
	known   bool     // false once a cd goes somewhere the guard cannot be sure of
	cdPath  bool     // CDPATH is set, so a relative cd may land elsewhere
	inChain bool     // a cd ran earlier in the current && chain
	sinks   []int    // the offsets of the text heredocs' << operators
}

// textHomeTemp is the temp folder in the home directory, ~/.tmp.
const textHomeTemp = ".tmp"

// textBases lists the trusted bases in the order trustedBase tries them:
// ~/.tmp and textTempDirs, then the payload cwd and the home directory. A
// temp folder comes first, so ~/.tmp is judged from there even when the cwd
// is home, and ~/.tmp leads, as home may sit in /tmp.
func textBases(cwd, home string) []string {
	var bases []string
	if home != "" {
		bases = append(bases, filepath.Join(home, textHomeTemp))
	}
	bases = append(bases, textTempDirs...)
	for _, b := range []string{cwd, home} {
		if b != "" {
			bases = append(bases, b)
		}
	}
	return bases
}

// line reads the shell line cmd[pos:end]. A plain line moves on to the next
// line; a text heredoc line returns its body span and moves past the
// terminator. Any other line ends the walk.
func (w *textWalk) line(cmd string, pos, end int) (span textSpan, next int, ok bool) {
	line := cmd[pos:end]
	opAt := strings.Index(line, "<<")
	if opAt < 0 {
		if !plainShell(line) {
			return textSpan{}, 0, false
		}
		w.steps(line, -1)
		return textSpan{}, end + 1, true
	}
	op, ok := parseHeredocOp(line[opAt:])
	if !ok || end == len(cmd) {
		return textSpan{}, 0, false
	}
	rest := line[:opAt] + " " + line[opAt+op.width:]
	if strings.Contains(rest, "<<") || !plainShell(rest) {
		return textSpan{}, 0, false
	}
	body, after, ok := heredocBody(cmd[end+1:], op)
	if !ok || !textBody(body, op) || !w.steps(rest, opAt) {
		return textSpan{}, 0, false
	}
	w.sinks = append(w.sinks, pos+opAt)
	to := len(cmd) - len(after)
	return textSpan{from: end + 1, to: to}, to, true
}

// textBody reports whether the shell hands the body over unchanged and the
// body does not declare itself a script.
func textBody(body string, op heredocOp) bool {
	if !op.quoted && strings.ContainsAny(body, "$`\\") {
		return false
	}
	return !strings.HasPrefix(body, "#!")
}

// steps follows the pieces of one shell line in order: a cd moves the
// directory, and the end of an && chain forgets a cd made in it. The piece
// holding the heredoc operator (at opAt, or -1 on a plain line) must write
// the body only to text targets.
func (w *textWalk) steps(line string, opAt int) bool {
	pieces, starts := splitHead(line)
	sink := false
	for i, p := range pieces {
		fields := strings.Fields(p)
		if opAt >= starts[i] && opAt < starts[i]+len(p) {
			if !w.textSink(fields) {
				return false
			}
			sink = true
		} else {
			w.cd(fields)
		}
		if !strings.HasPrefix(line[starts[i]+len(p):], "&&") {
			w.endChain()
		}
	}
	return opAt < 0 || sink
}

// textSink reports whether `[sudo] cat|tee ...` with the heredoc operator
// cut out writes only to text targets.
func (w *textWalk) textSink(fields []string) bool {
	targets, ok := sinkTargets(fields)
	if !ok {
		return false
	}
	for _, target := range targets {
		if !w.textTarget(target) {
			return false
		}
	}
	return true
}

// sinkTargets reads the words of `[sudo] cat|tee ...` with the heredoc
// operator cut out: output redirects, tee's flags and its file arguments.
// It returns every file the words write, stderr targets included. ok is
// false for any other word, and when the body reaches no file.
func sinkTargets(fields []string) (targets []string, ok bool) {
	if len(fields) > 1 && fields[0] == "sudo" {
		fields = fields[1:]
	}
	if len(fields) == 0 || (fields[0] != "cat" && fields[0] != "tee") {
		return nil, false
	}
	tee, toFile := fields[0] == "tee", false
	for i := 1; i < len(fields); i++ {
		f := fields[i]
		op, target := "tee", f
		m := textRedirect.FindStringSubmatch(f)
		switch {
		case f == "2>&1", tee && teeFlags[f]:
			continue
		case m != nil && m[2] == "" && i+1 < len(fields):
			i++
			op, target = m[1], fields[i]
		case m != nil && m[2] != "":
			op, target = m[1], m[2]
		case m != nil || !tee || strings.HasPrefix(f, "-"):
			return nil, false
		}
		if target == "/dev/null" {
			continue
		}
		targets = append(targets, target)
		toFile = toFile || !strings.HasPrefix(op, "2")
	}
	return targets, toFile
}

// textTarget reports whether a heredoc may write to target as text: a
// literal document path outside dot and system directories, that is new or
// a plain file the write cannot pass on to another one.
func (w *textWalk) textTarget(target string) bool {
	if !textTargetWord(target) || !textDocExts[strings.ToLower(filepath.Ext(target))] {
		return false
	}
	path := config.ExpandHome(target)
	switch {
	case !filepath.IsAbs(path) && (strings.HasPrefix(path, "~") || !w.known):
		return false
	case !filepath.IsAbs(path):
		path = filepath.Join(w.dir, path)
	}
	path = filepath.Clean(path)
	base, rel := w.trustedBase(path)
	// A temp base sits under /private on macOS, so it skips the system
	// directories. ~/Library is refused under any base: HOME itself can sit
	// inside a temp folder.
	if w.homeLibrary(path) || (!slices.Contains(textTempDirs, base) && w.systemPath(path)) {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if slices.ContainsFunc(parts, func(p string) bool { return strings.HasPrefix(p, ".") }) {
		return false
	}
	return textFileAt(base, parts)
}

// textTargetWord reports whether a target is a literal path. ~/.tmp is the
// one dot directory a target may name after ~/, as it is a temp folder.
func textTargetWord(word string) bool {
	if rest, ok := strings.CutPrefix(word, "~/"+textHomeTemp+"/"); ok {
		word = "~/" + rest
	}
	return textPathOK(word)
}

// textPathOK reports whether a word is a literal path with no . or ..
// part, so cleaning it cannot hide where it points.
func textPathOK(word string) bool {
	if !textPathWord.MatchString(word) {
		return false
	}
	return !slices.ContainsFunc(strings.Split(word, "/"), func(p string) bool {
		return p == "." || p == ".."
	})
}

// systemPath reports whether an absolute path is under a system directory
// or ~/Library.
func (w *textWalk) systemPath(path string) bool {
	if w.homeLibrary(path) {
		return true
	}
	return slices.ContainsFunc(textSystemDirs, func(dir string) bool { return pathUnder(path, dir) })
}

// homeLibrary reports whether an absolute path is under ~/Library.
func (w *textWalk) homeLibrary(path string) bool {
	return w.home != "" && pathUnder(path, filepath.Join(w.home, "Library"))
}

// pathUnder reports whether path is dir or below it, ignoring case.
func pathUnder(path, dir string) bool {
	p, d := strings.ToLower(path), strings.ToLower(dir)
	return p == d || strings.HasPrefix(p, d+"/")
}

// trustedBase splits an absolute path into the first of w.bases that holds
// it, else /, and the part below it. A base is where the session works,
// lives or keeps scratch files, so only the part below it is judged.
func (w *textWalk) trustedBase(path string) (base, rel string) {
	for _, b := range w.bases {
		r, err := filepath.Rel(b, path)
		if err == nil && r != "." && r != ".." && !strings.HasPrefix(r, "../") {
			return b, r
		}
	}
	return "/", strings.TrimPrefix(path, "/")
}

// textFileAt walks parts below base: every directory on the way must be a
// real directory, not a symlink, and an existing target a regular file with
// one link and no execute bit. A part that does not exist ends the walk,
// since the write or a mkdir creates it.
func textFileAt(base string, parts []string) bool {
	path := base
	for i, part := range parts {
		path = filepath.Join(path, part)
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return true
		case err != nil, i < len(parts)-1 && !info.IsDir():
			return false
		case i == len(parts)-1:
			return textFileMode(info)
		}
	}
	return false
}

// textFileMode reports whether an existing target is a regular file with no
// execute bit and no second link, which could be a startup file's.
func textFileMode(info fs.FileInfo) bool {
	if !info.Mode().IsRegular() || info.Mode()&0o111 != 0 {
		return false
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Nlink == 1
}

// cd follows a `cd DIR` piece. An absolute or ~/ path moves the directory;
// a relative one moves it only when the directory is known and CDPATH is
// unset. Anything else leaves the directory unknown.
func (w *textWalk) cd(fields []string) {
	if len(fields) == 0 || fields[0] != "cd" {
		return
	}
	w.inChain = true
	if len(fields) != 2 || !textCdWord(fields[1]) {
		w.known = false
		return
	}
	path := config.ExpandHome(fields[1])
	switch {
	case filepath.IsAbs(path):
		w.dir, w.known = filepath.Clean(path), true
	case strings.HasPrefix(path, "~") || w.cdPath:
		w.known = false
	case w.known:
		w.dir = filepath.Join(w.dir, path)
	}
}

// endChain marks the end of an && chain. A cd in it may have failed and
// skipped the rest of the chain, so the directory is unknown after it.
func (w *textWalk) endChain() {
	if w.inChain {
		w.known = false
	}
	w.inChain = false
}

// textCdWord reports whether a cd argument names one directory as written.
func textCdWord(word string) bool {
	return word == "~" || textPathOK(word)
}

// textSafeCommand reports whether nothing outside the text bodies can run a
// file: no pipe, substitution, subshell, background job or CDPATH, and every
// step but the heredoc sinks (at the sinks offsets) is textSafeStep.
func textSafeCommand(cmd string, spans textSpans, sinks []int) bool {
	outside := spans.cut(cmd)
	for _, mark := range textShellMarks {
		if strings.Contains(outside, mark) {
			return false
		}
	}
	// A lone & starts a background job, which splitSegments does not split.
	bare := outside
	for _, op := range []string{"&&", ">&", "&>"} {
		bare = strings.ReplaceAll(bare, op, "")
	}
	if strings.Contains(bare, "&") {
		return false
	}
	for _, seg := range splitSegments(cmd) {
		sink := slices.ContainsFunc(sinks, func(at int) bool {
			return at >= seg.off && at < seg.off+len(seg.text)
		})
		if !sink && !spans.holds(seg.off) && !textSafeStep(cmd, seg) {
			return false
		}
	}
	return true
}

// textSafeStep reports whether one step beside a text heredoc cannot run,
// copy or chmod a file: no redirect, and a textSafeHeads command, a
// textSafeGit subcommand without textGitWrites, a textSafeGh action, or a
// literal cd followed by &&.
func textSafeStep(cmd string, seg segment) bool {
	fields := strings.Fields(seg.text)
	switch {
	case strings.ContainsAny(seg.text, "<>"):
		return false
	case len(fields) == 0:
		return true
	case fields[0] == "cd":
		return len(fields) == 2 && textCdWord(fields[1]) &&
			strings.HasPrefix(cmd[seg.off+len(seg.text):], "&&")
	case fields[0] == "git":
		return len(fields) > 1 && textSafeGit[fields[1]] &&
			!slices.ContainsFunc(fields[2:], gitWritesFile)
	case fields[0] == "gh":
		return len(fields) > 2 && textSafeGh[fields[1]][fields[2]]
	}
	return textSafeHeads[fields[0]]
}

// gitWritesFile reports whether a git argument is one of textGitWrites or an
// abbreviation git would expand to one (--outp, --chm).
func gitWritesFile(arg string) bool {
	name, _, _ := strings.Cut(arg, "=")
	if len(name) < 3 || !strings.HasPrefix(name, "--") {
		return false
	}
	return slices.ContainsFunc(textGitWrites, func(opt string) bool {
		return strings.HasPrefix(opt, name) || strings.HasPrefix(name, opt)
	})
}

// plainShell reports whether a line holds only the bytes pyHeadChars allows,
// so no quote, substitution, comment or continuation can hide in it.
func plainShell(line string) bool {
	for i := 0; i < len(line); i++ {
		if strings.IndexByte(pyHeadChars, line[i]) < 0 {
			return false
		}
	}
	return true
}

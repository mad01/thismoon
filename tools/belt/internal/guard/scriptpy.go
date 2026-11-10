package guard

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// The script guard matches every inline script as plain text, so a heredoc
// that writes a file mentioning a deny-listed command is denied like one that
// runs it. inertPythonHeredoc carves out one narrow case where the guard can
// prove the text cannot run: a Python heredoc that is the whole command, run
// in isolated mode, using only modules that cannot start a process or run
// code from a string. Anything it is not sure about keeps the strict match.
//
// Why isolated mode (-I) is required: without it Python puts the working
// directory on sys.path, so a script can write heapq.py or traceback.py and
// have the interpreter import it later in the same run (a lazy stdlib import,
// or the traceback printer on an uncaught exception). That runs whatever the
// written file says, so a script that "only writes a file" is not inert.
// -I keeps the working directory and the user site directory off sys.path,
// which closes that path. It does not close a write into a directory still on
// sys.path: where the user owns the interpreter's stdlib or site-packages
// (mise and Homebrew installs), a script can write a module there and trigger
// its import in the same run. The text match never stopped that write either
// (printf or any other writer makes it), so this rule opens nothing new.
//
// The interpreter must be a bare name the shell finds on PATH. A path such as
// ./python3 or ~/bin/python3 may be a link to a shell, which would run the
// body as shell commands, so any form other than the bare word keeps the
// strict match.

// pyHeredocHint is appended to a denied Python heredoc: the shape that is
// read as code instead of text.
const pyHeredocHint = "If the script only reads, writes or prints text, run it as the whole command with " +
	"`python3 -I - <<'EOF'`, importing only json, re, pathlib, textwrap, datetime or collections, " +
	"with no os, sys, exec, eval, getattr or _-prefixed names anywhere in it: that heredoc is read as code, not text."

// pyAllowedModules are the modules an inert script may import. None of them
// starts a process or runs code from a string, and once pyBlockedNames is
// applied none of their public attributes reaches a module that can.
var pyAllowedModules = map[string]bool{
	"json": true, "re": true, "pathlib": true, "textwrap": true, "datetime": true, "collections": true,
}

// pyBlockedNames may not appear anywhere in an inert script, strings and
// comments included. They are the modules a public attribute chain reaches
// from the allowed modules (pathlib.os, json.codecs.sys, re.enum.bltns), the
// modules and callables that start a process or run code, and the dynamic
// access forms that would let a script spell any of these at run time.
var pyBlockedNames = map[string]bool{
	// Reachable from the allowed modules, or a process or code runner.
	"os": true, "sys": true, "posix": true, "nt": true, "builtins": true, "bltns": true,
	"subprocess": true, "pty": true, "shutil": true, "importlib": true, "ctypes": true,
	"runpy": true, "multiprocessing": true, "socket": true, "asyncio": true, "concurrent": true,
	"pickle": true, "marshal": true, "types": true, "inspect": true, "gc": true,
	"FunctionType": true, "CodeType": true, "LambdaType": true,
	// Process calls, should a module above be reached some other way.
	"system": true, "popen": true, "fork": true, "forkpty": true, "startfile": true,
	"execl": true, "execle": true, "execlp": true, "execlpe": true,
	"execv": true, "execve": true, "execvp": true, "execvpe": true,
	"spawnl": true, "spawnle": true, "spawnlp": true, "spawnlpe": true,
	"spawnv": true, "spawnve": true, "spawnvp": true, "spawnvpe": true,
	"posix_spawn": true, "posix_spawnp": true,
	// Code from a string, and dynamic access.
	"exec": true, "eval": true, "getattr": true, "setattr": true, "delattr": true,
	"vars": true, "globals": true, "locals": true, "breakpoint": true, "help": true,
	"attrgetter": true, "methodcaller": true, "import_module": true, "exec_module": true,
	"run_path": true, "run_module": true,
}

var (
	// pyFrameAttr names frame, code, generator and traceback attributes
	// (gi_frame, f_globals, f_builtins, co_code): they reach a namespace, and
	// through it every builtin, without a dunder.
	pyFrameAttr = regexp.MustCompile(`^(f|gi|cr|ag|tb|co)_`)
	// pyCodingDecl is a source encoding declaration: Python would decode the
	// script differently from the text the guard reads (utf-7 hides names).
	pyCodingDecl = regexp.MustCompile(`coding[:=]`)
	pyWord       = regexp.MustCompile(`[A-Za-z0-9_]+`)
	pyDelim      = regexp.MustCompile(`^[A-Za-z0-9_]+`)
	// pyFromImport is the `from M import` form, ending where `import` starts.
	pyFromImport = regexp.MustCompile(`(?:^|[\n\r;:])[ \t]*from[ \t]+([A-Za-z0-9_]+)$`)
)

// pyPlainWord is a shell word with nothing the shell would expand or treat
// as an operator: a cd target or a redirect target.
var pyPlainWord = regexp.MustCompile(`^[A-Za-z0-9_./~@%+:,=-]+$`)

var (
	// pyFlag is a cluster of Python options that take no argument and change
	// nothing about which code runs. -c, -m, -W, -X, -x and -i are not here.
	pyFlag = regexp.MustCompile(`^-[BEIOPSbqsu]+$`)
	// pyRedirectDup copies one output descriptor onto the other (2>&1).
	pyRedirectDup = regexp.MustCompile(`^[12]?>&[12]$`)
	// pyRedirectOp is an output redirect whose target is the next word;
	// pyRedirectTo has the target attached (>out.txt, 2>>log).
	pyRedirectOp = regexp.MustCompile(`^(&>>?|[12]?>>?)$`)
	pyRedirectTo = regexp.MustCompile(`^(&>>?|[12]?>>?)[A-Za-z0-9_./~@%+:,=-]+$`)
)

// pyHeadChars are the only bytes the command may hold outside the heredoc
// body, once the << operator and its delimiter are cut out: no quotes,
// substitutions, globs, pipes or input redirects can hide there.
const pyHeadChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_./~@%+:,=- \t\n&;>"

// heredocOp is a parsed << operator.
type heredocOp struct {
	delim  string
	quoted bool // the body is literal: no expansion or backslash handling
	strip  bool // <<- strips leading tabs from body lines and the terminator
	width  int  // bytes from the first < to the end of the delimiter word
}

// inertPythonHeredoc reports whether the command is a single Python heredoc
// the guard can prove runs no command, and where that Python command starts.
// Everything from start to the end of the command is the Python command, its
// body and its terminator line, so no segment there needs a check.
//
// The command must be, in full: optional `cd DIR` steps joined by && or ;,
// then `python3 -I [-] <<'EOF'` with only option flags and output redirects
// beside it, the body, the terminator line, and nothing after it. A second
// heredoc, an input redirect, a pipe, a substitution, a background job, an
// unquoted body that expands, or any other command before or after keeps the
// strict match.
func inertPythonHeredoc(cmd string) (start int, ok bool) {
	opAt := strings.Index(cmd, "<<")
	if opAt < 0 {
		return 0, false
	}
	nl := strings.IndexByte(cmd[opAt:], '\n')
	if nl < 0 {
		return 0, false
	}
	lineEnd := opAt + nl
	op, ok := parseHeredocOp(cmd[opAt:lineEnd])
	if !ok {
		return 0, false
	}
	head := cmd[:opAt] + " " + cmd[opAt+op.width:lineEnd]
	start, ok = pythonCommandStart(head)
	if !ok || start > opAt {
		return 0, false
	}
	body, rest, ok := heredocBody(cmd[lineEnd+1:], op)
	if !ok || strings.TrimSpace(rest) != "" {
		return 0, false
	}
	if !op.quoted && strings.ContainsAny(body, "$`\\") {
		return 0, false
	}
	return start, pythonInert(body)
}

// parseHeredocOp reads `<<` or `<<-`, optional blanks, and a delimiter word
// that is plain, single- or double-quoted, or backslash-quoted.
func parseHeredocOp(s string) (heredocOp, bool) {
	var op heredocOp
	i := 2
	if strings.HasPrefix(s, "<<<") {
		return op, false
	}
	if i < len(s) && s[i] == '-' {
		op.strip = true
		i++
	}
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	var quote byte
	if i < len(s) && (s[i] == '\'' || s[i] == '"' || s[i] == '\\') {
		op.quoted, quote = true, s[i]
		i++
	}
	op.delim = pyDelim.FindString(s[i:])
	if op.delim == "" {
		return op, false
	}
	i += len(op.delim)
	if quote == '\'' || quote == '"' {
		if i >= len(s) || s[i] != quote {
			return op, false
		}
		i++
	}
	// The shell's delimiter word runs to the next metacharacter: in
	// <<EOF-I the word is EOF-I, and -I is no flag for the interpreter.
	if i < len(s) && strings.IndexByte(" \t;&|<>()", s[i]) < 0 {
		return op, false
	}
	op.width = i
	return op, true
}

// pythonCommandStart checks the command text around the heredoc operator and
// returns the offset where the Python command starts.
func pythonCommandStart(head string) (int, bool) {
	for i := 0; i < len(head); i++ {
		if strings.IndexByte(pyHeadChars, head[i]) < 0 {
			return 0, false
		}
	}
	pieces, starts := splitHead(head)
	last := -1
	for i, p := range pieces {
		if strings.TrimSpace(p) != "" {
			last = i
		}
	}
	if last < 0 {
		return 0, false
	}
	for _, p := range pieces[:last] {
		if !plainCd(strings.Fields(p)) {
			return 0, false
		}
	}
	if !isolatedPython(strings.Fields(pieces[last])) {
		return 0, false
	}
	return starts[last], true
}

// splitHead splits the command text at &&, ; and newlines, the separators
// splitSegments also splits at, so each piece's start is a segment offset.
func splitHead(head string) (pieces []string, starts []int) {
	from := 0
	for i := 0; i < len(head); i++ {
		width := 0
		switch {
		case head[i] == ';' || head[i] == '\n':
			width = 1
		case strings.HasPrefix(head[i:], "&&"):
			width = 2
		}
		if width == 0 {
			continue
		}
		pieces, starts = append(pieces, head[from:i]), append(starts, from)
		from = i + width
		i += width - 1
	}
	return append(pieces, head[from:]), append(starts, from)
}

// plainCd reports whether a piece before the Python command is empty or a
// cd to a plain directory word.
func plainCd(fields []string) bool {
	switch {
	case len(fields) == 0:
		return true
	case fields[0] != "cd" || len(fields) > 2:
		return false
	}
	return len(fields) == 1 || pyPlainWord.MatchString(fields[1])
}

// isolatedPython reports whether the fields are a Python interpreter run with
// -I, reading its script from stdin: option flags, an optional `-`, then
// output redirects only. The interpreter word is matched whole, so a path
// (./python3, /opt/bin/python3, ~/bin/python3) is not a Python interpreter
// here; quotes, backslashes and $ never reach this point (pyHeadChars).
func isolatedPython(fields []string) bool {
	if len(fields) == 0 || !pythonName.MatchString(fields[0]) {
		return false
	}
	isolated, i := false, 1
	for ; i < len(fields) && pyFlag.MatchString(fields[i]); i++ {
		isolated = isolated || strings.ContainsRune(fields[i], 'I')
	}
	if i < len(fields) && fields[i] == "-" {
		i++
	}
	for ; i < len(fields); i++ {
		switch f := fields[i]; {
		case pyRedirectDup.MatchString(f), pyRedirectTo.MatchString(f):
		case pyRedirectOp.MatchString(f) && i+1 < len(fields) && pyPlainWord.MatchString(fields[i+1]):
			i++
		default:
			return false
		}
	}
	return isolated
}

// heredocBody returns the body up to the terminator line and the text after
// that line. ok is false when no line terminates the body. A <<- body comes
// back with its leading tabs stripped, as the interpreter reads it.
func heredocBody(after string, op heredocOp) (body, rest string, ok bool) {
	var lines []string
	for line := 0; line <= len(after); {
		end := strings.IndexByte(after[line:], '\n')
		next := line + end + 1
		if end < 0 {
			end, next = len(after)-line, len(after)+1
		}
		text := after[line : line+end]
		if op.strip {
			text = strings.TrimLeft(text, "\t")
		}
		if text == op.delim {
			return strings.Join(lines, "\n"), after[min(next, len(after)):], true
		}
		lines = append(lines, text)
		line = next
	}
	return "", "", false
}

// pythonInert reports whether Python source provably cannot start a process
// or run code it builds at run time. The checks read the whole text, strings
// and comments included, so no parsing mistake can hide a name: every import
// is of an allowed module, and no word is a blocked name, a private or dunder
// name, or a frame attribute. A blocked word inside a string costs only a
// strict match.
func pythonInert(src string) bool {
	if !utf8.ValidString(src) || strings.IndexByte(src, 0) >= 0 {
		return false
	}
	// Python folds identifiers to NFKC, so a fullwidth ｅｘｅｃ is exec.
	text := norm.NFKC.String(src)
	if pyCodingDecl.MatchString(text) {
		return false
	}
	for _, loc := range pyWord.FindAllStringIndex(text, -1) {
		if !pyWordAllowed(text, loc[0], loc[1]) {
			return false
		}
	}
	return true
}

// pyWordAllowed judges one word of the script.
func pyWordAllowed(text string, from, to int) bool {
	w := text[from:to]
	switch {
	case w[0] >= '0' && w[0] <= '9', w == "_":
		return true // a number, or the throwaway name
	case w[0] == '_', pyBlockedNames[w], pyFrameAttr.MatchString(w):
		return false
	case w == "compile":
		// re.compile is an attribute; the builtin compile is a bare name.
		before := strings.TrimRight(text[:from], " \t")
		return strings.HasSuffix(before, ".")
	case w == "import":
		return pyImportAllowed(text, from, to)
	}
	return true
}

// pyImportAllowed reports whether the `import` at text[from:to] starts or
// ends an import statement of allowed modules only. Any other use of the
// word, in code or in a string, fails.
func pyImportAllowed(text string, from, to int) bool {
	before := strings.TrimRight(text[:from], " \t")
	if m := pyFromImport.FindStringSubmatch(before); m != nil {
		return pyAllowedModules[m[1]]
	}
	if before != "" && strings.IndexByte("\n\r;:", before[len(before)-1]) < 0 {
		return false
	}
	rest := text[to:]
	if rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
		return false
	}
	if end := strings.IndexAny(rest, "\n\r;#"); end >= 0 {
		rest = rest[:end]
	}
	for _, part := range strings.Split(rest, ",") {
		f := strings.Fields(part)
		switch {
		case len(f) == 1 && pyAllowedModules[f[0]]:
		case len(f) == 3 && pyAllowedModules[f[0]] && f[1] == "as":
		default:
			return false
		}
	}
	return true
}

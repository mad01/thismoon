package guard

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/mad01/thismoon/kit/notify"
	"github.com/mad01/thismoon/tools/belt/internal/config"
)

// ScriptDenyListID identifies the deep deny inspection guard: it applies the
// Bash deny list inside scripts, not just to the top-level command.
const ScriptDenyListID = "script-deny-list"

// maxScriptSize caps how much of a script file gets scanned.
const maxScriptSize = 1 << 20

// shellInterpreters and inlineEInterpreters are command names whose file
// argument is a script worth scanning, split by which flag marks an inline
// script: -c for shells and python, -e/-E/--eval for the rest. Python names
// are matched by prefix (python3.12) in isInterpreter.
var (
	shellInterpreters = map[string]bool{
		"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "fish": true,
	}
	inlineEInterpreters = map[string]bool{
		"perl": true, "ruby": true, "node": true, "osascript": true,
	}
	pythonName = regexp.MustCompile(`^python[0-9.]*$`)
)

// inlineC and inlineE are the flag sets marking an inline script per
// interpreter family.
var (
	inlineC = map[string]bool{"-c": true}
	inlineE = map[string]bool{"-e": true, "-E": true, "--eval": true}
)

// isInterpreter classifies a segment head, returning the flag set that marks
// an inline script for it.
func isInterpreter(head string) (inlineFlags map[string]bool, ok bool) {
	switch {
	case shellInterpreters[head] || pythonName.MatchString(head):
		return inlineC, true
	case inlineEInterpreters[head]:
		return inlineE, true
	}
	return nil, false
}

// scriptExts mark directly-executed files (./x.sh) as scannable scripts.
var scriptExts = map[string]bool{
	".sh": true, ".bash": true, ".zsh": true, ".fish": true, ".py": true,
	".rb": true, ".pl": true, ".js": true, ".mjs": true, ".cjs": true, ".ts": true,
}

// wrapperOptions are the wrappers that run the command after them as
// written, so they are stripped from the front of a segment before the head
// is classified, each with the options it reads in front of its command
// (wrapperSpec). It is the one table of them: isWrapper and every reader
// that skips a wrapper look a name up here. The wrappers that hand their
// words to a shell instead are shellRunners. coproc runs its command as a
// coprocess, and zsh's noglob and nocorrect run theirs with globbing or
// spelling correction off. The options come from sudo -h, env --help,
// gtimeout --help and the macOS man pages, and for the tools macOS does not
// ship (doas, setsid, flock, ionice, chronic) from their OpenBSD,
// util-linux and moreutils manuals. An option a wrapper's entry does not
// name fails closed: the words after the wrapper are read as shell text
// (wrapperSpec.read).
var wrapperOptions = map[string]wrapperSpec{
	"sudo": {
		short: "Aa:BbC:c:D:Eeg:Hh::iKklNnPp:R:r:SsT:t:U:u:Vv",
		long: "askpass auth-type: background bell close-from: login-class: chdir: preserve-env:: " +
			"edit group: set-home help host: login remove-timestamp reset-timestamp list " +
			"non-interactive preserve-groups prompt: chroot: role: stdin shell type: " +
			"command-timeout: other-user: user: version validate",
		asText: "s i shell login",
	},
	"env": {
		short: "0ia:C:P:S:u:v",
		long: "ignore-environment null argv0: chdir: split-string: unset: debug " +
			"block-signal:: default-signal:: ignore-signal:: list-signal-handling help version",
		asText:   "S split-string",
		loneDash: true,
	},
	"doas":      {short: "a:C:Lnsu:", asText: "s", noRun: "C L"},
	"command":   {short: "pVv", noRun: "v V"},
	"builtin":   {},
	"coproc":    {async: true},
	"noglob":    {},
	"nocorrect": {},
	"exec":      {short: "a:cl"},
	"nohup":     {long: "help version"},
	"setsid":    {short: "cfwhV", long: "ctty fork wait help version", async: true},
	"time":      {short: "af:hlo:pqVv", long: "append format: output: portability quiet verbose help version"},
	"timeout":   timeoutSpec,
	"gtimeout":  timeoutSpec,
	"nice":      {short: "n:0123456789", long: "adjustment: help version"},
	"ionice": {
		short: "c:n:p:P:tu:hV",
		long:  "class: classdata: pid: pgid: ignore uid: help version",
		noRun: "p P u pid pgid uid",
	},
	"stdbuf":     {short: "e:i:o:", long: "error: input: output: help version"},
	"caffeinate": {short: "dimst:uw:"},
	"chronic":    {short: "ev"},
	"arch": {
		words: "32 64 arch: c d: e: h i386 x86_64 x86_64h arm64 arm64e",
	},
	"flock": {
		short: "sexnoFuw:E:hVc:",
		long: "shared exclusive unlock nonblock nb close no-fork timeout: wait: " +
			"conflict-exit-code: fcntl verbose help version command:",
		asText:   "c command",
		operand:  true,
		textNext: "-c --command",
	},
}

// timeoutSpec is GNU timeout's, which Homebrew also installs as gtimeout:
// options, then the duration, then the command.
var timeoutSpec = wrapperSpec{
	short:   "k:s:fpv",
	long:    "kill-after: signal: foreground preserve-status verbose help version",
	operand: true,
}

// isWrapper reports whether a command name is a wrapper wrapperOptions
// reads.
func isWrapper(name string) bool {
	_, ok := wrapperOptions[name]
	return ok
}

// fileProducers and fetchProducers classify the segment piped into a bare
// interpreter: a file producer's file arguments get scanned, a fetcher is
// denied outright — its content cannot be inspected before it runs.
var (
	fileProducers  = map[string]bool{"cat": true, "head": true, "tail": true}
	fetchProducers = map[string]bool{"curl": true, "wget": true}
)

// ScriptDenyList denies bash tool calls that route deny-listed commands
// around the permission system's view of the top-level command line: an
// executed or sourced script file, a `-c`/`-e` string, a heredoc, a file
// written and run inside the same command, or an indirection head (eval,
// xargs, find -exec) whose real command a prefix matcher never sees. The
// permission system only judges the literal command — `bash cleanup.sh`
// looks harmless even when the script runs `rm -rf`. This guard reads what
// is about to run and applies the same deny list to it. Patterns come from
// the Claude settings deny lists (permissions.deny `Bash(...)` entries) plus
// `extra_patterns` in the belt config; `mode: soft` downgrades denials to
// warn events.
type ScriptDenyList struct {
	cfg config.Config
	// emit sends the soft-mode warning to the events service. Injectable for
	// tests.
	emit func(source, level, title, message string, tags map[string]string)
}

// NewScriptDenyList builds the guard with the real events backend.
func NewScriptDenyList(cfg config.Config) *ScriptDenyList {
	return &ScriptDenyList{cfg: cfg, emit: notify.EmitEventSync}
}

func (g *ScriptDenyList) ID() string    { return ScriptDenyListID }
func (g *ScriptDenyList) Event() string { return EventBash }

// Check scans every script the command would execute and denies on the first
// deny-list match. In soft mode the denial becomes a warn event and the
// command proceeds.
func (g *ScriptDenyList) Check(in Input) *Denial {
	d := g.check(in)
	if d == nil {
		return nil
	}
	if g.cfg.Guards[ScriptDenyListID].Soft() {
		g.emit("belt", "warn", "script-deny-list (soft)",
			d.Reason+" Command proceeding (soft mode).",
			map[string]string{"guard": ScriptDenyListID})
		return nil
	}
	return d
}

// scan carries one check invocation's shared state across segments. cmd is
// the command with its data pieces masked (maskData): the segments and inline
// scripts are read from it, so a denied command named only in an echo string
// or a commit message is not read as code. scanTarget and scanMadeExecutable
// keep reading in.Command, the raw text a written file's content lives in.
// names are the variables, functions and aliases cmd defines (shellNames).
type scan struct {
	in       Input
	cmd      string
	segs     []segment
	written  map[string]bool
	patterns []deny
	names    shellNames
}

// check runs every scan over the command with its data masked (maskData).
// A denial that goes away once belt trusts every `$(cat <<'EOF' ... EOF)`
// message as bash 5 and zsh read it (maskDataBash5) comes from a message
// bash 3.2 may run: it says so and names the forms that stay data
// (bash32Hint). So does one in the text after a message bash 3.2 may end
// in another place (scanBash32Tail).
func (g *ScriptDenyList) check(in Input) *Denial {
	patterns := g.compiled()
	if len(patterns) == 0 || in.Command == "" {
		return nil
	}
	cmd := maskData(in.Command)
	d := g.scanMasked(in, cmd, patterns)
	if d == nil {
		if d = scanBash32Tail(in.Command, patterns); d != nil {
			return bash32Denial(d)
		}
		return nil
	}
	if bash5 := maskDataBash5(in.Command); bash5 != cmd && g.scanMasked(in, bash5, patterns) == nil {
		return bash32Denial(d)
	}
	return d
}

// scanMasked runs every scan over in.Command, read from cmd, the command
// with its data blanked.
func (g *ScriptDenyList) scanMasked(in Input, cmd string, patterns []deny) *Denial {
	segs := splitSegments(cmd)
	s := scan{
		in:       in,
		cmd:      cmd,
		segs:     segs,
		written:  redirectTargets(segs, in.Cwd),
		patterns: patterns,
		names:    readShellNames(cmd),
	}
	// A Python heredoc proven inert ends the command: its own segment, its
	// body lines and its terminator need no check. A text heredoc's body
	// lines are not commands either (scripttext.go). Both read the raw
	// command; masking keeps offsets, so their spans still line up.
	inertFrom, inert := inertPythonHeredoc(in.Command)
	text := textHeredocBodies(in.Command, in.Cwd)
	for i := range segs {
		if inert && segs[i].off >= inertFrom {
			break
		}
		if text.holds(segs[i].off) {
			continue
		}
		if d := g.checkSegment(s, i); d != nil {
			return d
		}
	}
	// A substitution runs where it stands, whatever command it is an argument
	// of, so its body is code even inside a masked echo. A print piped into an
	// interpreter is the script that interpreter runs. A download or decoded
	// text run as code is denied, as nothing can read it (codeWalk).
	if d := scanSubstitutions(cmd, patterns); d != nil {
		return d
	}
	if d := newCodeWalk(g, s).run(cmd, 0); d != nil {
		return d
	}
	return scanPipedPrint(in.Command, patterns)
}

// checkSegment classifies one segment's head and scans whatever script text
// or file it would run. The head is read past wrappers first, and a command
// hidden behind one or behind a name is checked (checkHidden). A wrapper
// named by a path (./nice, /usr/bin/env) is read both ways: as the script
// of that name it may be, and as the wrapper. A head in ANSI-C quoting
// reads as the shell passes it (ansiCWord), so `$'bash'` names bash. A head
// that names an interpreter through a variable is read as that interpreter
// (shellNames.runnerNamed).
func (g *ScriptDenyList) checkSegment(s scan, i int) *Denial {
	seg := s.segs[i]
	fields := strings.Fields(seg.text)
	tokens, kind := unwrapCommand(fields)
	for _, word := range fields[:len(fields)-len(tokens)] {
		if strings.ContainsRune(word, '/') && !strings.Contains(word, "=") && wrapperName(filepath.Base(word)) {
			if d := g.scanRunFile(s, word); d != nil {
				return d
			}
		}
	}
	if d := s.checkHidden(seg, firstCommandWord(skipOpeners(fields)), tokens, kind); d != nil {
		return d
	}
	if len(tokens) == 0 {
		return nil
	}
	head := filepath.Base(ansiCWord(tokens[0]))
	if runner, ok := s.names.runnerNamed(tokens[0]); ok {
		head = runner // "$SHELL" -c …, S=sh; … | $S
	}
	if flags, ok := isInterpreter(head); ok {
		return g.checkInterpreter(s, i, head, tokens[1:], flags)
	}
	switch {
	case head == "uv" && len(tokens) > 1 && tokens[1] == "run":
		_, file := parseInterpreterArgs(tokens[2:], inlineC)
		return g.scanTarget(s, file)
	case head == "eval":
		// The eval argument may span separator splits inside quotes; scan
		// from this segment to the end of the command, like inline scripts.
		return scanText(s.cmd[seg.off:], "the eval argument", s.patterns)
	case head == "xargs":
		return scanText(strings.Join(tokens[1:], " "), "the xargs command", s.patterns)
	case head == "find":
		return scanFindExec(tokens, s.patterns)
	case head == "source" || tokens[0] == ".":
		if len(tokens) > 1 {
			return g.scanTarget(s, tokens[1])
		}
	case head == "chmod":
		return g.scanMadeExecutable(s, tokens[1:])
	case strings.ContainsRune(tokens[0], '/'):
		return g.scanRunFile(s, tokens[0])
	}
	return nil
}

// scanRunFile scans a file a segment runs by its path (./deploy, bin/x.sh)
// when it is a script: one the same command writes, or scannableScript.
func (g *ScriptDenyList) scanRunFile(s scan, path string) *Denial {
	resolved := resolvePath(path, s.in.Cwd)
	if s.written[resolved] || scannableScript(resolved) {
		return g.scanTarget(s, path)
	}
	return nil
}

// checkInterpreter handles a segment whose head is an interpreter: an inline
// script (-c/-e string or heredoc), a script file argument, a `< file` stdin
// redirect, or a pipe feeding it from a producer segment. A shell's -c may
// sit in a bundle (`bash -lc`, `sh -ec`; inlineScriptAt).
func (g *ScriptDenyList) checkInterpreter(
	s scan, i int, head string, args []string, inlineFlags map[string]bool,
) *Denial {
	seg := s.segs[i]
	inline, file := parseInterpreterArgs(args, inlineFlags)
	if shellInterpreters[head] && inlineScriptAt(slices.Concat([]string{head}, args)) >= 0 {
		inline, file = true, ""
	}
	heredoc := strings.Contains(seg.text, "<<")
	if inline || heredoc {
		// Inline script: scan from this segment to the end of the command —
		// the script text follows the interpreter and may cross separator
		// splits inside quotes, but text before the segment belongs to
		// other commands.
		if d := scanText(s.cmd[seg.off:], "the inline script", s.patterns); d != nil {
			if heredoc && pythonName.MatchString(head) {
				d.Reason += " " + pyHeredocHint
			}
			return d
		}
	}
	if file != "" {
		return g.scanTarget(s, file)
	}
	if !inline && !heredoc {
		return g.checkProducer(s, i)
	}
	return nil
}

// checkProducer looks at the segment piped into a bare interpreter: file
// producers (cat x.sh | bash) get their file arguments scanned, a heredoc
// they pass on (cat <<EOF | bash) gets its body scanned, and fetchers
// (curl ... | bash) are denied because nothing can read what would run.
func (g *ScriptDenyList) checkProducer(s scan, i int) *Denial {
	if i == 0 || !pipeBefore(s.in.Command, s.segs[i].off) {
		return nil
	}
	producer := s.segs[i-1]
	prev := stripWrappers(strings.Fields(producer.text))
	if len(prev) == 0 {
		return nil
	}
	head := filepath.Base(prev[0])
	if (fileProducers[head] || head == "tee") && strings.Contains(producer.text, "<<") {
		// A heredoc or here-string passed on unchanged (cat <<EOF | sh) is
		// the script the interpreter reads; its body follows the producer.
		where := "the heredoc piped into an interpreter"
		if d := scanText(s.in.Command[producer.off:], where, s.patterns); d != nil {
			return d
		}
	}
	switch {
	case fileProducers[head]:
		for _, arg := range prev[1:] {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			if d := g.scanTarget(s, arg); d != nil {
				return d
			}
		}
	case fetchProducers[head]:
		return Reasonf(
			ScriptDenyListID,
			"pipes a %s download straight into an interpreter, so nothing can inspect what would run. "+
				"Download to a file in one call, then run the file in a later call, so the deny list (and the user) can read it before it executes.",
			head,
		)
	}
	return nil
}

// pipeBefore reports whether the segment starting at off follows a single |
// (a pipe), not a || or another separator.
func pipeBefore(command string, off int) bool {
	return off > 0 && command[off-1] == '|' && (off < 2 || command[off-2] != '|')
}

// patterns merges the Claude settings deny prefixes with the guard's
// configured extra patterns.
func (g *ScriptDenyList) patterns() []string {
	return uniquePatterns(g.cfg.ClaudeDeny, g.cfg.Guards[ScriptDenyListID].ExtraPatterns)
}

// compiled compiles the guard's patterns, each one the settings list gives
// marked as such (deny.settings), so its denial asks the user to run the
// command, since direct-deny-list denies the direct form too.
func (g *ScriptDenyList) compiled() []deny {
	patterns := compilePatterns(g.patterns())
	fromSettings := map[string]bool{}
	for _, p := range uniquePatterns(g.cfg.ClaudeDeny) {
		fromSettings[p] = true
	}
	for i := range patterns {
		patterns[i].settings = fromSettings[patterns[i].pattern]
	}
	return patterns
}

// uniquePatterns joins pattern lists in order, each pattern trimmed, with
// the empty ones and repeats dropped.
func uniquePatterns(lists ...[]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range lists {
		for _, p := range list {
			p = strings.TrimSpace(p)
			if p == "" || seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// scanTarget scans one script path the command would run: the file's current
// content, and — when the same command also writes that path — the raw
// command text, which is where the future content lives at check time.
func (g *ScriptDenyList) scanTarget(s scan, path string) *Denial {
	if path == "" {
		return nil
	}
	resolved := resolvePath(path, s.in.Cwd)
	if g.cfg.Guards[ScriptDenyListID].ExcludesPath(resolved) {
		return nil
	}
	if s.written[resolved] {
		where := fmt.Sprintf("the command (which writes and then runs %s)", path)
		if d := scanText(s.in.Command, where, s.patterns); d != nil {
			return d
		}
	}
	return g.scanFile(resolved, s.patterns)
}

// scanMadeExecutable handles chmod: a file the same command writes and then
// gives an execute bit is a script about to exist, so the raw command text,
// where its content lives, is scanned as for a write-then-run. A mode that
// adds no execute bit (600, -x) and a file the command does not write are
// left alone.
func (g *ScriptDenyList) scanMadeExecutable(s scan, args []string) *Denial {
	if !chmodAddsExec(args) {
		return nil
	}
	for _, arg := range args {
		resolved := resolvePath(arg, s.in.Cwd)
		if !s.written[resolved] || g.cfg.Guards[ScriptDenyListID].ExcludesPath(resolved) {
			continue
		}
		where := fmt.Sprintf("the command (which writes %s and changes its mode)", arg)
		if d := scanText(s.in.Command, where, s.patterns); d != nil {
			return d
		}
	}
	return nil
}

var (
	// chmodClause is one symbolic mode clause: who, then operators each with
	// the permissions they set or clear (u+x, go=r, +rwx, a-x, g=u).
	chmodClause = regexp.MustCompile(`^[ugoa]*([-+=][rwxXstugo]*)+$`)
	// chmodSets is an operator in a clause that can set bits, with its
	// permissions.
	chmodSets  = regexp.MustCompile(`[+=][rwxXstugo]*`)
	chmodOctal = regexp.MustCompile(`^[0-7]{1,4}$`)
)

// chmodAddsExec reports whether chmod's arguments may add an execute bit.
// Flags such as -R are skipped, and the first other word is the mode. A long
// option (--reference copies another file's mode) or a mode it cannot read
// counts as adding one.
func chmodAddsExec(args []string) bool {
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, "--"):
			return true
		case strings.HasPrefix(arg, "-") && !chmodClause.MatchString(arg):
			continue
		}
		return modeAddsExec(arg)
	}
	return false
}

// modeAddsExec reports whether a chmod mode may set an execute bit: an octal
// mode with one, or a clause that sets x or X, or copies another class's
// bits (g=u).
func modeAddsExec(mode string) bool {
	if chmodOctal.MatchString(mode) {
		bits, err := strconv.ParseUint(mode, 8, 16)
		return err != nil || bits&0o111 != 0
	}
	for _, clause := range strings.Split(mode, ",") {
		if !chmodClause.MatchString(clause) {
			return true
		}
		for _, set := range chmodSets.FindAllString(clause, -1) {
			if strings.ContainsAny(set[1:], "xXugo") {
				return true
			}
		}
	}
	return false
}

// scanFile reads the script (capped at maxScriptSize) and scans it.
// Unreadable files are allowed — the shell will fail on them anyway, and
// belt must not block on its own limitations; the write-then-run pass in
// scanTarget covers the file that does not exist yet.
func (g *ScriptDenyList) scanFile(resolved string, patterns []deny) *Denial {
	f, err := os.Open(resolved)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxScriptSize))
	if err != nil {
		return nil
	}
	return scanText(string(data), fmt.Sprintf("script %s", resolved), patterns)
}

// resolvePath expands ~ and joins relative paths against cwd, mirroring how
// the shell will resolve the path.
func resolvePath(path, cwd string) string {
	resolved := config.ExpandHome(path)
	if !filepath.IsAbs(resolved) && cwd != "" {
		resolved = filepath.Join(cwd, resolved)
	}
	return resolved
}

// redirectTargets collects the files the command writes via >/>> redirects
// or tee, resolved like executed scripts, so scanTarget can spot a script
// written and run inside one command — at check time that file does not
// exist yet and scanFile alone would fail open.
func redirectTargets(segs []segment, cwd string) map[string]bool {
	targets := map[string]bool{}
	add := func(path string) {
		if path != "" && !strings.HasPrefix(path, "&") {
			targets[resolvePath(path, cwd)] = true
		}
	}
	for _, seg := range segs {
		tokens := strings.Fields(seg.text)
		for i := 0; i < len(tokens); i++ {
			if filepath.Base(tokens[i]) == "tee" {
				for _, arg := range tokens[i+1:] {
					if !strings.HasPrefix(arg, "-") {
						add(arg)
					}
				}
				break
			}
			j := strings.IndexByte(tokens[i], '>')
			if j < 0 {
				continue
			}
			rest := strings.TrimLeft(tokens[i][j:], ">")
			if rest == "" {
				if i+1 < len(tokens) {
					add(tokens[i+1])
					i++
				}
				continue
			}
			add(rest)
		}
	}
	return targets
}

// scannableScript reports whether a directly-executed path is script-like: a
// known script extension, or an existing file that starts with a shebang, so
// ./deploy with #!/bin/bash gets scanned without an allowlisted extension.
func scannableScript(resolved string) bool {
	if scriptExts[filepath.Ext(resolved)] {
		return true
	}
	f, err := os.Open(resolved)
	if err != nil {
		return false
	}
	defer f.Close()
	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return false
	}
	return magic[0] == '#' && magic[1] == '!'
}

// scanFindExec scans the command after find's -exec/-execdir/-ok/-okdir —
// the one part of a find invocation that runs something.
func scanFindExec(tokens []string, patterns []deny) *Denial {
	for i, tok := range tokens {
		switch tok {
		case "-exec", "-execdir", "-ok", "-okdir":
			return scanText(strings.Join(tokens[i+1:], " "), "the find "+tok+" command", patterns)
		}
	}
	return nil
}

// deny pairs a deny-list pattern with its compiled matchers: re for the
// text match, nil for an entry with a `*` group, which argv alone reads, and
// entry for the argv match (denyentry.go), nil for an entry the reader does
// not read. settings marks an entry from the Claude settings deny list, as
// opposed to extra_patterns.
type deny struct {
	pattern  string
	re       *regexp.Regexp
	entry    *denyEntry
	settings bool
}

// hint returns the advice a script-deny-list denial of this entry ends
// with: direct, the advice to run the command where the permission system
// can judge it, for an extra_patterns entry. An entry from the settings
// list gets askUserToRun instead, since direct-deny-list denies the direct
// form too.
func (d deny) hint(direct string) string {
	if d.settings {
		return askUserToRun
	}
	return direct
}

// compilePatterns builds the matchers per pattern. A `re:` prefix compiles
// the rest as a case-insensitive regex: the escape hatch for shapes the
// entry format cannot express. Anchor it yourself when word boundaries
// matter. Every other pattern is read as a deny entry on argv
// (readDenyEntry). One without a `*` group also matches its words as text
// in order, separated by whitespace, not embedded in a longer word: `kubectl
// delete` matches `  kubectl   delete pod x` but not `kubectl deleted`. A
// pattern the reader refuses keeps that text match alone.
func compilePatterns(patterns []string) []deny {
	var out []deny
	for _, p := range patterns {
		if raw, ok := strings.CutPrefix(p, "re:"); ok {
			if re, err := regexp.Compile("(?i)" + raw); err == nil {
				out = append(out, deny{pattern: p, re: re})
			}
			continue
		}
		d, toks := deny{pattern: p}, strings.Fields(p)
		if e, ok := readDenyEntry(p); ok {
			d.entry, toks = &e, entryWords(p)
		}
		if d.entry == nil || !slices.Contains(toks, "*") {
			d.re = wordsRegexp(toks, textGap)
		}
		if d.re != nil || d.entry != nil {
			out = append(out, d)
		}
	}
	return out
}

// inText reports whether the text match finds the pattern in a line.
func (d deny) inText(line string) bool {
	return d.re != nil && d.re.MatchString(line)
}

// textGap is what stands between an entry's words in the text match.
const textGap = `\s+`

// wordsRegexp matches words in order as text, case-insensitively, separated
// by gap (textGap, or phraseGap for a phrase) and not inside a longer word.
// It is nil for no words.
func wordsRegexp(words []string, gap string) *regexp.Regexp {
	if len(words) == 0 {
		return nil
	}
	quoted := make([]string, len(words))
	for i, t := range words {
		quoted[i] = regexp.QuoteMeta(t)
	}
	return regexp.MustCompile(`(?i)(?:^|[^\w-])` + strings.Join(quoted, gap) + `(?:[^\w-]|$)`)
}

// lineNoise collapses the separators that hide a command from the
// word-boundary matcher — quotes, commas, brackets, backticks — so
// subprocess.run(["rm", "-rf", ...]) and kubectl "delete" match the same
// patterns as their plain forms.
var lineNoise = strings.NewReplacer(
	`"`, " ", `'`, " ", ",", " ", "[", " ", "]", " ", "`", " ",
)

// logicalLine is one script statement after backslash-continued lines are
// joined, tagged with the physical line it starts on.
type logicalLine struct {
	text   string
	number int
}

// logicalLines splits text into statements, joining lines that end in a
// backslash so a pattern split across a continuation still matches.
func logicalLines(text string) []logicalLine {
	raw := strings.Split(text, "\n")
	out := make([]logicalLine, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		start := i + 1
		line := raw[i]
		for strings.HasSuffix(line, `\`) && i+1 < len(raw) {
			line = strings.TrimSuffix(line, `\`) + " " + raw[i+1]
			i++
		}
		out = append(out, logicalLine{text: line, number: start})
	}
	return out
}

// directHint ends a deny-list denial: what to do instead.
const directHint = "Run the blocked operation as a direct command so the permission system can evaluate it, " +
	"or ask the user first."

// bash32Hint ends a denial whose match is in a `$(cat <<'EOF' ... EOF)`
// message belt reads as code because bash 3.2 may run part of it
// (bash32Closes), or in the text after one it may end later (bash32Tail):
// it takes the place of the advice a deny-list denial ends with
// (deny.hint), and the git guards add it to their own reason
// (bash32Hinted).
const bash32Hint = "belt found it in or after a $(cat <<'EOF' ... EOF) message: bash 3.2, macOS's /bin/bash, " +
	"can end that substitution early or late and run part of the message or the text after it, " +
	"so belt reads them as code. " +
	"Pass the message as data instead: git commit -F - (or gh ... --body-file -) with a quoted heredoc " +
	"such as <<'EOF', or a message file (git commit -F <file>, gh ... --body-file <file>)."

// bash32Denial is d with bash32Hint in place of the advice its reason ends
// with (directHint, or askUserToRun for a settings entry), or after the
// reason when it has neither.
func bash32Denial(d *Denial) *Denial {
	reason, ok := strings.CutSuffix(d.Reason, directHint)
	if !ok {
		reason, ok = strings.CutSuffix(d.Reason, askUserToRun)
	}
	if !ok {
		reason += " "
	}
	return &Denial{Guard: d.Guard, Reason: reason + bash32Hint}
}

// bash32Hinted is d with bash32Hint added when bash32 says it comes from a
// step of a message read as code for bash 3.2 alone (walkStep.bash32).
func bash32Hinted(d *Denial, bash32 bool) *Denial {
	if d == nil || !bash32 {
		return d
	}
	return bash32Denial(d)
}

// scanBash32Tail scans the text after the first `$(cat <<'EOF' ... EOF)`
// that bash 3.2 may end in another place than belt (bash32Tail) as code. A
// quote of the body can stay open past belt's ), so any word, comment or
// heredoc body after it may run there: every # reads as a blank, so no line
// is skipped as a comment, and the names the whole command defines count.
func scanBash32Tail(cmd string, patterns []deny) *Denial {
	at := bash32Tail(cmd)
	if at == len(cmd) {
		return nil
	}
	tail := strings.ReplaceAll(cmd[at:], "#", " ")
	return scanLines(tail, "the text after a $(cat <<'EOF' ... EOF) message", patterns, readShellNames(cmd), 0)
}

// scanText checks every non-comment logical line against every pattern —
// first as written, then with quoting noise collapsed, then each form's
// commands on argv — and denies on the first hit. A command whose name the
// text's own variables, aliases or functions give is read through them
// (shellNames.hiddenRun). Text with a $'...' string in it is scanned again
// with each one decoded (ansiCText), and its names read again from the
// decoded text, since a shell that reads the text as code runs the decoded
// value: `bash -c $'kube\x63tl delete pod x'`, `bash -c $'K=kube\x63tl\n$K
// delete pod x'`.
func scanText(text, where string, patterns []deny) *Denial {
	if d := scanLines(text, where, patterns, readShellNames(text), 0); d != nil {
		return d
	}
	if decoded := ansiCText(text); decoded != text {
		return scanLines(decoded, where, patterns, readShellNames(decoded), 0)
	}
	return nil
}

// scanLines is scanText with the names to read commands through, and how
// many expansions deep the text already is.
func scanLines(text, where string, patterns []deny, names shellNames, depth int) *Denial {
	for _, line := range logicalLines(text) {
		trimmed := strings.TrimSpace(line.text)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		normalized := lineNoise.Replace(line.text)
		cmds := lineCommands(line.text, normalized)
		for _, d := range patterns {
			if d.inText(line.text) || d.inText(normalized) || d.runsIn(cmds) {
				return Reasonf(
					ScriptDenyListID,
					"%s runs %q (line %d) — that command is on the Bash deny list and scripts don't get to bypass it. %s",
					where,
					d.pattern,
					line.number,
					d.hint(directHint),
				)
			}
		}
		for _, words := range cmds {
			run, _ := unwrapCommand(words)
			if len(run) == 0 || !names.hides(commandName(run[0])) {
				continue
			}
			at := fmt.Sprintf("%s (line %d)", where, line.number)
			if d := names.hiddenRun(run, at, patterns, depth); d != nil {
				return d
			}
		}
	}
	return nil
}

// parseInterpreterArgs walks the tokens after an interpreter and reports
// whether it runs an inline script (a flag from inlineFlags), or which file
// it runs — a positional argument or a `< file` stdin redirect. An inline
// flag counts only before the file argument — in `python x.py -c foo` the -c
// belongs to the script, and x.py must still be scanned.
func parseInterpreterArgs(tokens []string, inlineFlags map[string]bool) (inline bool, file string) {
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case inlineFlags[tok]:
			return true, ""
		case tok == "--":
			if i+1 < len(tokens) {
				return false, tokens[i+1]
			}
			return false, ""
		case tok == "-m":
			return false, "" // python -m runs a module, not a file
		case tok == "<<" || tok == "<<-":
			i++ // heredoc: skip the delimiter token, the body is scanned via the segment text
		case strings.HasPrefix(tok, "<<"):
			continue // <<EOF / <<<word: the body is scanned via the segment text
		case tok == "<":
			if i+1 < len(tokens) {
				return false, tokens[i+1]
			}
			return false, ""
		case strings.HasPrefix(tok, "<"):
			return false, tok[1:] // attached stdin redirect: <x.sh
		case strings.HasPrefix(tok, "-"):
			continue
		default:
			return false, tok
		}
	}
	return false, ""
}

// stripWrappers drops leading env assignments and wrapper commands (sudo,
// env, exec, ...), with their options (wrapperOptions), so the real head of
// the segment gets classified. It returns a suffix of tokens.
func stripWrappers(tokens []string) []string {
	rest, _ := peelWrappers(tokens)
	return rest
}

// peelWrappers is stripWrappers that also says how the words left run:
// runsDirect behind assignments alone, runsWrapped behind a wrapper that
// runs them as written, and runsAsText behind one that hands them to a
// shell (sudo -s, env -S) or whose options belt cannot read, from the words
// after that wrapper on (wrapperSpec.read).
func peelWrappers(tokens []string) ([]string, wrapKind) {
	kind := runsDirect
	for len(tokens) > 0 {
		if strings.Contains(tokens[0], "=") {
			tokens = tokens[1:]
			continue
		}
		head := filepath.Base(tokens[0])
		spec, ok := wrapperOptions[head]
		if !ok {
			return tokens, kind
		}
		rest, how := spec.read(tokens[1:])
		if how == runsAsText {
			return rest, how
		}
		if head == "coproc" && len(rest) > 1 && (rest[1] == "{" || rest[1] == "(") {
			rest = rest[1:] // coproc NAME { …; }: NAME names the coprocess
		}
		if spec.operand {
			if len(rest) == 0 {
				return tokens, kind
			}
			rest = rest[1:]
			if text, ok := spec.afterOperand(rest); ok {
				return text, runsAsText
			}
		}
		tokens, kind = rest, runsWrapped
	}
	return tokens, kind
}

// skipOptions returns args from the first word that is not an option: a
// word in valued takes the next word as its value, any other word starting
// with - is a flag or a bundle (`-oL`, `-10`), and -- ends the options.
func skipOptions(args []string, valued map[string]bool) []string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--":
			return args[i+1:]
		case valued[a]:
			i++
		case len(a) > 1 && a[0] == '-':
		default:
			return args[i:]
		}
	}
	return nil
}

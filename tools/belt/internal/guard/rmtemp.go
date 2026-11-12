package guard

import (
	"os"
	"os/exec"
	"path"
	"strings"
)

// RmTempID identifies the guard that sends temp-folder deletes to toss-bin.
const RmTempID = "rm-temp"

// TossBin is the command the rm-temp deny hands over. It moves paths to the
// trash and, with --temp-only, refuses anything outside the temp folders.
// Exported for belt doctor, which checks that PATH resolves it.
const TossBin = "toss-bin"

// RmTemp refuses a Bash call whose every rm deletes only paths inside a temp
// folder, and hands over the toss-bin command that does the same job. A
// settings ask rule on rm shows a permission dialog for any rm, temp paths
// included, and a PreToolUse deny runs before that rule: the agent reroutes
// and no dialog shows. toss-bin also moves the paths to the trash, so the
// delete stays recoverable. An rm with any target the guard cannot place
// inside a temp folder gets no answer, so the ask rule still decides it.
type RmTemp struct {
	// home returns the home directory, "" when unknown. Injectable for tests.
	home func() string
	// lookPath finds toss-bin. Without it the deny would hand over a command
	// that cannot run, so the guard stays silent. Injectable for tests.
	lookPath func(file string) (string, error)
}

// NewRmTemp builds the guard with the real home directory and PATH lookup.
// It reads no config beyond the enable toggle, which ForEvent applies.
func NewRmTemp() *RmTemp {
	return &RmTemp{home: homeDir, lookPath: exec.LookPath}
}

func (g *RmTemp) ID() string    { return RmTempID }
func (g *RmTemp) Event() string { return EventBash }

// Check denies when the command holds at least one rm and every rm in it
// deletes only temp paths. One rm the guard cannot vouch for leaves the
// whole call to the ask rule: its dialog shows either way, so a deny would
// only cost a round trip.
func (g *RmTemp) Check(in Input) *Denial {
	calls, ok := tempRmCalls(in.Command, g.home())
	if !ok {
		return nil
	}
	if _, err := g.lookPath(TossBin); err != nil {
		return nil
	}
	return Reasonf(RmTempID, "%s", rmTempReason(calls))
}

// homeDir returns the cleaned absolute home directory, or "" when there is
// none, which turns ~ and $HOME targets and the ~/.tmp root off.
func homeDir() string {
	home, err := os.UserHomeDir()
	if err != nil || !path.IsAbs(home) {
		return ""
	}
	return path.Clean(home)
}

// rmCall is one rm whose targets are all temp paths: the targets as
// written, and whether one hangs off a mktemp result in $TMPDIR.
type rmCall struct {
	targets []string
	tmpdir  bool
}

// tempVar is what the guard knows about a variable set from mktemp: the
// template path when mktemp was told where to create it, or tmpdir when it
// creates in $TMPDIR, a folder the guard cannot see.
type tempVar struct {
	path   string
	tmpdir bool
}

// rmScan walks a command's simple commands in order, tracking variables set
// from mktemp so a later rm of "$D" can be judged.
type rmScan struct {
	home string
	vars map[string]tempVar
}

// tempRmCalls returns every rm in the command when each one deletes only
// temp paths. ok is false when the command holds no rm, when one rm has a
// target outside the temp folders or one the guard cannot resolve, and when
// the command does not lex.
func tempRmCalls(command, home string) ([]rmCall, bool) {
	cmds, ok := lexCommands(command)
	if !ok {
		return nil, false
	}
	s := rmScan{home: home, vars: map[string]tempVar{}}
	var calls []rmCall
	for _, words := range cmds {
		words = skipReserved(words)
		if s.assign(words) {
			continue
		}
		words = skipAssignments(words)
		if len(words) == 0 || !isCommandWord(words[0], "rm") {
			continue
		}
		call, ok := s.rm(words[1:])
		if !ok {
			return nil, false
		}
		calls = append(calls, call)
	}
	return calls, len(calls) > 0
}

// shellReserved are the words that may come before a command's name.
var shellReserved = map[string]bool{
	"!": true, "{": true, "if": true, "then": true, "elif": true,
	"else": true, "while": true, "until": true, "do": true,
}

func skipReserved(words []shWord) []shWord {
	for len(words) > 0 && shellReserved[words[0].raw] {
		words = words[1:]
	}
	return words
}

func skipAssignments(words []shWord) []shWord {
	for len(words) > 0 {
		if _, _, ok := words[0].assignment(); !ok {
			break
		}
		words = words[1:]
	}
	return words
}

// assignment splits a NAME=value word. A NAME+=value word reports the name
// with a nil value, which tracking reads as unknown.
func (w shWord) assignment() (string, []wordPart, bool) {
	if len(w.parts) == 0 || w.parts[0].kind != partLit || w.parts[0].quoted {
		return "", nil, false
	}
	text := w.parts[0].text
	eq := strings.IndexByte(text, '=')
	if eq <= 0 {
		return "", nil, false
	}
	name, appends := strings.CutSuffix(text[:eq], "+")
	if !isShellName(name) {
		return "", nil, false
	}
	if appends {
		return name, nil, true
	}
	value := w.parts[1:]
	if rest := text[eq+1:]; rest != "" {
		value = append([]wordPart{{kind: partLit, text: rest}}, value...)
	}
	return name, value, true
}

// assign applies a command made only of assignments (optionally behind
// export or local) to the tracked variables, and reports whether the
// command was one. A variable set from anything but a mktemp the guard
// understands is forgotten.
func (s *rmScan) assign(words []shWord) bool {
	if len(words) > 0 && (words[0].raw == "export" || words[0].raw == "local") {
		words = words[1:]
	} else if len(skipAssignments(words)) > 0 {
		return false
	}
	for _, w := range words {
		name, value, ok := w.assignment()
		if !ok {
			continue // `export NAME` or a flag
		}
		if v, ok := s.mktempVar(value); ok {
			s.vars[name] = v
		} else {
			delete(s.vars, name)
		}
	}
	return true
}

// isCommandWord reports whether a command word runs the named command: the
// bare name, however quoted or escaped, or an absolute path to it.
func isCommandWord(w shWord, name string) bool {
	v, ok := w.literal()
	return ok && (v == name || path.IsAbs(v) && path.Base(v) == name)
}

// rmFlags are the short option letters BSD and GNU rm take.
const rmFlags = "dfiIPRrvWx"

// rmLongFlags are the GNU long options that delete the operands as given.
var rmLongFlags = map[string]bool{
	"--force": true, "--recursive": true, "--dir": true, "--verbose": true,
	"--one-file-system": true, "--no-preserve-root": true, "--preserve-root": true,
	"--preserve-root=all": true, "--interactive": true, "--interactive=never": true,
	"--interactive=once": true, "--interactive=always": true,
}

// rm judges one rm's arguments: every operand must be a temp path. An
// option after an operand is not ok, since BSD rm reads it as a file name
// where GNU rm reads it as an option. So is an option the guard does not
// know, and an rm with no operand.
func (s *rmScan) rm(args []shWord) (rmCall, bool) {
	var call rmCall
	endOpts := false
	for _, w := range args {
		v, lit := w.literal()
		switch {
		case !endOpts && lit && v == "--":
			endOpts = true
			continue
		case !endOpts && lit && len(v) > 1 && v[0] == '-':
			if len(call.targets) > 0 || !isRmOption(v) {
				return rmCall{}, false
			}
			continue
		}
		tmpdir, ok := s.target(w)
		if !ok {
			return rmCall{}, false
		}
		call.targets = append(call.targets, w.raw)
		call.tmpdir = call.tmpdir || tmpdir
	}
	return call, len(call.targets) > 0
}

func isRmOption(v string) bool {
	if strings.HasPrefix(v, "--") {
		return rmLongFlags[v]
	}
	for i := 1; i < len(v); i++ {
		if strings.IndexByte(rmFlags, v[i]) < 0 {
			return false
		}
	}
	return true
}

// tmpdirBase stands in for a mktemp result in $TMPDIR, whose real path the
// guard cannot know. A target built on it must clean to it or below it.
const tmpdirBase = "/mktemp-in-tmpdir"

// target reports whether one rm operand resolves strictly inside a temp
// folder, and whether it hangs off a $TMPDIR mktemp result.
func (s *rmScan) target(w shWord) (tmpdir, ok bool) {
	parts := w.parts
	if len(parts) == 0 || parts[0].kind != partVar || parts[0].text == "HOME" {
		p, _, ok := s.staticPath(w)
		return false, ok && underTempRoot(p, s.home)
	}
	v, known := s.vars[parts[0].text]
	if !known {
		return false, false
	}
	// The rest must be empty or start a sub-path: `$D/x` names something in
	// $D, where `${D}x` names a sibling of it.
	suffix, _, ok := literalPath(parts[1:])
	if !ok || suffix != "" && suffix[0] != '/' {
		return false, false
	}
	if v.tmpdir {
		c := path.Clean(tmpdirBase + suffix)
		return true, c == tmpdirBase || strings.HasPrefix(c, tmpdirBase+"/")
	}
	return false, underTempRoot(v.path+suffix, s.home)
}

// staticPath returns the path a word names when the guard can expand it
// without a shell: a literal, or one behind a leading ~, ~/, $HOME or
// ${HOME}. glob reports an unquoted glob character in it, which
// literalPath has already checked cannot leave its folder.
func (s *rmScan) staticPath(w shWord) (p string, glob, ok bool) {
	parts := w.parts
	if len(parts) == 0 {
		return "", false, false
	}
	base := ""
	switch first := parts[0]; {
	case first.kind == partVar && first.text == "HOME":
		base, parts = s.home, parts[1:]
	case first.kind == partLit && !first.quoted && strings.HasPrefix(first.text, "~"):
		rest := first.text[1:]
		// ~user, and a tilde prefix that runs into quoted text, are not
		// expanded here.
		if rest != "" && rest[0] != '/' || rest == "" && len(parts) > 1 {
			return "", false, false
		}
		base = s.home
		parts = append([]wordPart{{kind: partLit, text: rest}}, parts[1:]...)
	default:
		rest, glob, ok := literalPath(parts)
		return rest, glob, ok
	}
	rest, glob, ok := literalPath(parts)
	if base == "" || !ok || rest != "" && rest[0] != '/' {
		return "", false, false
	}
	return base + rest, glob, true
}

// pathSafe are the unquoted bytes a path may hold besides letters, digits
// and glob characters. Anything else (braces, !, ^, #, =, a later ~) is a
// shell expansion in bash or zsh, or close enough to one that the guard
// does not guess.
const pathSafe = "/._-+,:@%]"

// literalPath joins literal pieces into a path. A piece that is not
// literal is not ok, and so is an unquoted byte outside pathSafe and the
// glob characters. A glob is ok only when no expansion of it can leave
// the folder it is written in: no component holds `..`, and no globbing
// component starts with a dot (bash before 5.2 matches `.*` to `..`).
func literalPath(parts []wordPart) (p string, glob, ok bool) {
	var b strings.Builder
	var active []bool
	for _, part := range parts {
		if part.kind != partLit {
			return "", false, false
		}
		for i := 0; i < len(part.text); i++ {
			c := part.text[i]
			isGlob := !part.quoted && strings.IndexByte("*?[", c) >= 0
			if !part.quoted && !isGlob && !isPathByte(c) {
				return "", false, false
			}
			b.WriteByte(c)
			active = append(active, isGlob)
			glob = glob || isGlob
		}
	}
	p = b.String()
	return p, glob, !glob || globStaysInside(p, active)
}

func isPathByte(c byte) bool {
	return isNameByte(c) || c >= 0x80 || strings.IndexByte(pathSafe, c) >= 0
}

// globStaysInside reports whether a path's active glob characters (marked
// per byte) can only match names inside the folders the path spells out.
func globStaysInside(p string, glob []bool) bool {
	start := 0
	for start <= len(p) {
		end := strings.IndexByte(p[start:], '/')
		if end < 0 {
			end = len(p) - start
		}
		comp := p[start : start+end]
		if comp == ".." {
			return false
		}
		globbing := false
		for _, g := range glob[start : start+end] {
			globbing = globbing || g
		}
		if globbing && comp[0] == '.' {
			return false
		}
		start += end + 1
	}
	return true
}

// underTempRoot reports whether p, cleaned lexically, sits strictly inside
// one of the temp folders toss-bin --temp-only accepts. A root itself is not
// inside, and neither is a relative path.
func underTempRoot(p, home string) bool {
	if !path.IsAbs(p) {
		return false
	}
	c := path.Clean(p)
	for _, root := range tempRoots(home) {
		if strings.HasPrefix(c, root+"/") {
			return true
		}
	}
	return false
}

// tempRoots are the folders toss-bin --temp-only accepts; ~/.tmp only when
// the home directory is known.
func tempRoots(home string) []string {
	roots := []string{"/tmp", "/var/tmp", "/private/tmp", "/private/var/tmp"}
	if home != "" {
		roots = append(roots, path.Join(home, ".tmp"))
	}
	return roots
}

// mktempVar reads an assignment value of the form $(mktemp ...) or
// `mktemp ...`, quoted or not, and says where mktemp creates its path.
func (s *rmScan) mktempVar(value []wordPart) (tempVar, bool) {
	if len(value) != 1 || value[0].kind != partSubst {
		return tempVar{}, false
	}
	cmds, ok := lexCommands(value[0].text)
	if !ok || len(cmds) != 1 || len(cmds[0]) == 0 {
		return tempVar{}, false
	}
	if !isCommandWord(cmds[0][0], "mktemp") {
		return tempVar{}, false
	}
	return s.parseMktemp(cmds[0][1:])
}

// parseMktemp places a mktemp call from its arguments. With no template, or
// with -t, it creates in $TMPDIR (or /tmp). With -p DIR or --tmpdir=DIR it
// creates in DIR, and otherwise at the template as given. Flags outside
// -d -q -u -t -p, a relative template and more than one template are not
// understood, so the variable stays unknown.
func (s *rmScan) parseMktemp(args []shWord) (tempVar, bool) {
	var dir, template string
	tmpdirMode, haveTemplate := false, false
	for i := 0; i < len(args); i++ {
		v, lit := args[i].literal()
		var ok bool
		switch {
		case !lit || !strings.HasPrefix(v, "-"):
			if haveTemplate {
				return tempVar{}, false
			}
			template, ok = s.mktempPath(args[i])
			haveTemplate = true
		case v == "--directory" || v == "--quiet" || v == "--dry-run":
			ok = true
		case v == "-p" && i+1 < len(args):
			i++
			dir, ok = s.mktempPath(args[i])
		case strings.HasPrefix(v, "--tmpdir="):
			dir, ok = strings.TrimPrefix(v, "--tmpdir="), true
		default:
			ok = len(v) > 1 && v[1] != '-' && strings.Trim(v[1:], "dqut") == ""
			tmpdirMode = tmpdirMode || strings.ContainsRune(v, 't')
		}
		if !ok {
			return tempVar{}, false
		}
	}
	switch {
	case dir != "":
		if strings.Contains(template, "/") {
			return tempVar{}, false
		}
		if template == "" {
			template = "tmp.XXXXXXXXXX"
		}
		p := dir + "/" + template
		return tempVar{path: p}, underTempRoot(p, s.home)
	case tmpdirMode || !haveTemplate:
		return tempVar{tmpdir: true}, !strings.Contains(template, "/")
	}
	return tempVar{path: template}, underTempRoot(template, s.home)
}

// mktempPath expands a mktemp path argument. A glob in one is not
// understood: the shell would expand it before mktemp sees it.
func (s *rmScan) mktempPath(w shWord) (string, bool) {
	p, glob, ok := s.staticPath(w)
	return p, ok && !glob
}

// rmTempReason is what the model reads: why, then the toss-bin command for
// each rm with its targets as written, then, when a target comes from a
// mktemp in $TMPDIR, how to create it where toss-bin accepts it.
func rmTempReason(calls []rmCall) string {
	var b strings.Builder
	b.WriteString(
		"every rm target here is inside a temp folder, so delete with toss-bin instead: " +
			"it needs no permission prompt and moves the paths to the trash. ",
	)
	tmpdir := false
	cmds := make([]string, len(calls))
	for i, c := range calls {
		cmds[i] = "`" + TossBin + " --temp-only " + strings.Join(c.targets, " ") + "`"
		tmpdir = tmpdir || c.tmpdir
	}
	if len(cmds) == 1 {
		b.WriteString("Replace the rm with " + cmds[0] + ".")
	} else {
		b.WriteString("Replace the rm commands, in order, with " + strings.Join(cmds, ", ") + ".")
	}
	if tmpdir {
		b.WriteString(" toss-bin takes only /tmp, /var/tmp and ~/.tmp, not $TMPDIR, " +
			"so give mktemp a template there: `mktemp -d /tmp/<name>.XXXXXX` (no -d for a file).")
	}
	return b.String()
}

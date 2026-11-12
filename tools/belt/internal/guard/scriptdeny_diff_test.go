package guard

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// originCheck is the script-deny-list decision as it stood on origin/main
// before the Python heredoc rule: a test-only copy of check, checkSegment,
// checkInterpreter and checkProducer. The helpers it calls (splitSegments,
// parseInterpreterArgs, scanText, scanTarget and the rest) are unchanged.
func originCheck(g *ScriptDenyList, in Input) *Denial {
	patterns := compilePatterns(g.patterns())
	if len(patterns) == 0 || in.Command == "" {
		return nil
	}
	segs := splitSegments(in.Command)
	s := scan{in: in, segs: segs, written: redirectTargets(segs, in.Cwd), patterns: patterns}
	for i := range segs {
		if d := originSegment(g, s, i); d != nil {
			return d
		}
	}
	return nil
}

func originSegment(g *ScriptDenyList, s scan, i int) *Denial {
	seg := s.segs[i]
	tokens := stripWrappers(strings.Fields(seg.text))
	if len(tokens) == 0 {
		return nil
	}
	head := filepath.Base(tokens[0])
	if flags, ok := isInterpreter(head); ok {
		return originInterpreter(g, s, i, tokens[1:], flags)
	}
	switch {
	case head == "uv" && len(tokens) > 1 && tokens[1] == "run":
		_, file := parseInterpreterArgs(tokens[2:], inlineC)
		return g.scanTarget(s, file)
	case head == "eval":
		return scanText(s.in.Command[seg.off:], "the eval argument", s.patterns)
	case head == "xargs":
		return scanText(strings.Join(tokens[1:], " "), "the xargs command", s.patterns)
	case head == "find":
		return scanFindExec(tokens, s.patterns)
	case head == "source" || tokens[0] == ".":
		if len(tokens) > 1 {
			return g.scanTarget(s, tokens[1])
		}
	case strings.ContainsRune(tokens[0], '/'):
		resolved := resolvePath(tokens[0], s.in.Cwd)
		if s.written[resolved] || scannableScript(resolved) {
			return g.scanTarget(s, tokens[0])
		}
	}
	return nil
}

func originInterpreter(
	g *ScriptDenyList,
	s scan,
	i int,
	args []string,
	inlineFlags map[string]bool,
) *Denial {
	seg := s.segs[i]
	inline, file := parseInterpreterArgs(args, inlineFlags)
	heredoc := strings.Contains(seg.text, "<<")
	if inline || heredoc {
		if d := scanText(s.in.Command[seg.off:], "the inline script", s.patterns); d != nil {
			return d
		}
	}
	if file != "" {
		return g.scanTarget(s, file)
	}
	if !inline && !heredoc {
		return originProducer(g, s, i)
	}
	return nil
}

func originProducer(g *ScriptDenyList, s scan, i int) *Denial {
	if i == 0 || !pipeBefore(s.in.Command, s.segs[i].off) {
		return nil
	}
	prev := stripWrappers(strings.Fields(s.segs[i-1].text))
	if len(prev) == 0 {
		return nil
	}
	switch head := filepath.Base(prev[0]); {
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
		return Reasonf(ScriptDenyListID, "pipes a %s download straight into an interpreter", head)
	}
	return nil
}

// corpusInput is one generated command and whether it is a benign Python
// script in the relaxed shape, which the rule is meant to let through.
type corpusInput struct {
	command string
	relaxed bool
}

// TestScriptDenyListNarrowsOnlyIntendedPasses is the safety net for the
// Python heredoc and text heredoc rules: across a generated corpus, every
// input the origin/main decision denies is still denied, unless it is one of
// heredocIntendedPasses or in a relaxed shape. Both rules only remove checks.
// The chmod and heredoc-pipe checks add denials, and textTightened lists the
// inputs that may be denied now although origin allowed them.
func TestScriptDenyListNarrowsOnlyIntendedPasses(t *testing.T) {
	// A relative cd is followed only when CDPATH is unset (scripttext.go).
	t.Setenv("CDPATH", "")
	cwd, bypasses := heredocBypassCases(t)
	writeScript(t, cwd, "evil.sh", "rm -rf /tmp/x\n")
	writeScript(t, cwd, "notes.md", "Run git config to check the name.\n")
	writeScript(t, cwd, "tool", "#!/bin/sh\necho hi\n")
	generalDeny, generalExtra, general := scriptDenyListCases(t)
	g := NewScriptDenyList(scriptDenyConfig(
		append(slices.Clone(heredocDeny), generalDeny...),
		append(slices.Clone(heredocExtra), generalExtra...),
	))

	type entry struct {
		command, cwd string
		relaxed      bool
	}
	var corpus []entry
	for _, c := range bypasses {
		corpus = append(corpus, entry{c.command, cwd, false})
	}
	for _, c := range general {
		corpus = append(corpus, entry{c.command, c.cwd, false})
	}
	for _, c := range generatedCorpus() {
		corpus = append(corpus, entry{c.command, cwd, c.relaxed})
	}
	for _, c := range textHeredocCorpus() {
		corpus = append(corpus, entry{c.command, cwd, c.relaxed})
	}
	intended := map[string]bool{}
	for _, c := range heredocIntendedPasses {
		intended[c] = true
		corpus = append(corpus, entry{c, cwd, false})
	}
	tightened := map[string]bool{}
	for _, c := range textTightened {
		tightened[c] = true
		corpus = append(corpus, entry{c, cwd, false})
	}

	var oldDenies, passes int
	for _, e := range corpus {
		in := Input{Event: EventBash, Command: e.command, Cwd: e.cwd}
		old, now := originCheck(g, in) != nil, g.Check(in) != nil
		switch {
		case now && !old && !tightened[e.command]:
			t.Errorf("denies what origin/main allowed:\n%s", e.command)
		case tightened[e.command] && (old || !now):
			t.Errorf(
				"tightened input must be allowed by origin/main and denied now (old %v, now %v):\n%s",
				old,
				now,
				e.command,
			)
		case old && !now && !intended[e.command] && !e.relaxed:
			t.Errorf(
				"lets through what origin/main denied, and it is not an intended pass:\n%s",
				e.command,
			)
		case e.relaxed && now:
			t.Errorf("denies an input in a relaxed shape:\n%s", e.command)
		case intended[e.command] && (!old || now):
			t.Errorf(
				"intended pass must be denied by origin/main and allowed now (old %v, now %v):\n%s",
				old,
				now,
				e.command,
			)
		}
		if old {
			oldDenies++
		}
		if old && !now {
			passes++
		}
	}
	t.Logf(
		"corpus %d inputs, origin/main denies %d, the rule lets through %d",
		len(corpus),
		oldDenies,
		passes,
	)
}

// generatedCorpus crosses interpreters, script bodies, heredoc quoting,
// flags, and the shell around the command (a prefix, operator-line suffix,
// and a command after the terminator). Benign Python bodies in the shape the
// rule relaxes are marked relaxed: the hand-written predicate below is the
// test's own statement of that shape, kept narrower than the code's.
func generatedCorpus() []corpusInput {
	var out []corpusInput
	for _, f := range scriptForms() {
		for _, p := range corpusPrefixes {
			for _, sfx := range corpusSuffixes {
				out = append(out, f.render(p, sfx, "", corpusRelaxed(f, p, sfx, "")))
			}
		}
		for _, tail := range corpusTails {
			out = append(out, f.render("", "", tail, corpusRelaxed(f, "", "", tail)))
		}
	}
	return out
}

var (
	corpusPrefixes = []string{
		"", "cd /tmp && ", "cd /tmp; ", "exec > >(sh); ", "sh /tmp/zz & ", "X=1 ", "sudo ",
		"echo hi | ", "PYTHONPATH=. ",
	}
	corpusSuffixes = []string{
		"", " > out.txt", " 2>&1", " | cat", " | sh", " | cat | sh", " > >(sh)", " | tee >(sh)", " &",
		" < evil.py", " notes.md", " <<'Z'", " $((1<<2))",
	}
	corpusTails = []string{
		"", "\ncat /tmp/zz | bash", "\nsh -c \"$(cat /tmp/zz)\"", "\nbash out.sh", "\necho done",
		"\nbash -c 'rm -rf /tmp/x'", "\n)",
	}
)

// scriptForm is one interpreter invocation with its script, before the shell
// around it is added.
type scriptForm struct {
	interp, flags, dash, delim string // delim "" means an inline -c/-e string
	body                       string
	benign                     bool // a Python body that only prints or writes text
}

func (f scriptForm) render(prefix, suffix, tail string, relaxed bool) corpusInput {
	if f.delim == "" {
		flag := "-c"
		if !strings.Contains(f.interp, "python") && !shellInterpreters[f.interp] {
			flag = "-e"
		}
		cmd := prefix + f.interp + f.flags + " " + flag + " " + shellQuote(f.body) + suffix + tail
		return corpusInput{command: cmd, relaxed: relaxed}
	}
	body, term := f.body, delimWord(f.delim)
	if strings.HasPrefix(f.delim, "-") {
		body = "\t" + strings.ReplaceAll(body, "\n", "\n\t")
		term = "\t" + term
	}
	cmd := prefix + f.interp + f.flags + " " + f.dash + "<<" + f.delim + suffix + "\n" + body + "\n" + term
	if strings.Contains(suffix, "<<'Z'") {
		cmd += "\nimport os\nos.system('rm -rf /tmp/x')\nZ"
	}
	return corpusInput{command: cmd + tail, relaxed: relaxed}
}

// delimWord is the terminator line for a delimiter as written after <<.
func delimWord(delim string) string {
	return strings.Trim(strings.TrimPrefix(delim, "-"), `'"\`)
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// corpusRelaxed is the test's statement of the relaxed shape: a benign
// Python heredoc run with -I by the bare interpreter name, alone or after a
// cd, with at most a file or descriptor redirect, and nothing after it.
func corpusRelaxed(f scriptForm, prefix, suffix, tail string) bool {
	return f.benign && f.delim != "" && strings.Contains(f.flags, "I") &&
		(f.interp == "python3" || f.interp == "python3.12") &&
		(prefix == "" || prefix == "cd /tmp && " || prefix == "cd /tmp; ") &&
		(suffix == "" || suffix == " > out.txt" || suffix == " 2>&1") && tail == ""
}

var (
	pythonBenign = []string{
		"print('rm -rf /tmp/x')",
		"import pathlib\npathlib.Path('notes.md').write_text('git config --global user.name x')",
		"import json, re\nprint(json.dumps({'cmd': 'rm -rf /tmp/x'}))",
	}
	pythonDanger = []string{
		"import os\nos.system('rm -rf /tmp/x')",
		"import subprocess\nsubprocess.run(['rm', '-rf', '/tmp/x'])",
		"import posix\nposix.system('rm -rf /tmp/x')",
		"import sys\nsys.modules['os'].system('git config --global user.name x')",
		`exec("imp" + "ort o" + "s; o" + "s.sy" + "stem('rm -rf /tmp/x')")`,
		"__import__('o' + 's').system('rm -rf /tmp/x')",
		"import pathlib\npathlib.Path('.').parser.os.system('rm -rf /tmp/x')",
		"g = (x for x in [1])\ng.gi_frame.f_builtins['ev' + 'al']('print(1)')\nprint('rm -rf /tmp/x')",
		"import re\nre.enum.bltns.exec('print(1)')\nprint('rm -rf /tmp/x')",
		"import plumbum\nplumbum.local['rm']('-rf', '/tmp/x')",
		"from shutil import which\nprint('rm -rf /tmp/x')",
		"import re\nr = compile('1', 'x', 'eval')\nprint('rm -rf /tmp/x')",
	}
	otherBodies = map[string][]string{
		"node": {
			"require('fs').writeFileSync('n.md', 'rm -rf /tmp/x')",
			"require('child_process').execSync('rm -rf /tmp/x')",
		},
		"ruby":      {"File.write('n.md', 'rm -rf /tmp/x')", "system('rm -rf /tmp/x')"},
		"perl":      {`print "rm -rf /tmp/x\n"`, `system("rm -rf /tmp/x")`},
		"osascript": {`return "rm -rf /tmp/x"`, `do shell script "rm -rf /tmp/x"`},
		"bash":      {"echo 'rm -rf /tmp/x'", "rm -rf /tmp/x"},
		"sh":        {"echo 'git config --global user.name x'", "git config --global user.name x"},
		"zsh":       {"print 'rm -rf /tmp/x'", "rm -rf /tmp/x"},
	}
	heredocDelims = []string{"'EOF'", `"EOF"`, `\EOF`, "EOF", "-'EOF'"}
)

func scriptForms() []scriptForm {
	var forms []scriptForm
	add := func(interp string, flagSets, dashes []string, body string, benign bool) {
		for _, flags := range flagSets {
			forms = append(
				forms,
				scriptForm{interp: interp, flags: flags, body: body, benign: benign},
			)
			for _, dash := range dashes {
				for _, delim := range heredocDelims {
					forms = append(forms, scriptForm{
						interp: interp, flags: flags, dash: dash, delim: delim, body: body, benign: benign,
					})
				}
			}
		}
	}
	pyFlags, pyDashes := []string{"", " -I", " -IB", " -E -P"}, []string{"", "- "}
	for _, b := range pythonBenign {
		add("python3", pyFlags, pyDashes, b, true)
	}
	for _, b := range pythonDanger {
		add("python3", pyFlags, pyDashes, b, false)
	}
	add("python3.12", []string{" -I"}, []string{"- "}, pythonBenign[0], true)
	add("python3.12", []string{" -I"}, []string{"- "}, pythonDanger[0], false)
	// A non-bare interpreter word may be a link to a shell, so a body that
	// is benign Python or a bare shell command must keep the strict match.
	for _, interp := range heredocNonBareInterpreters {
		add(interp, []string{" -I"}, pyDashes, pythonBenign[0], true)
		add(interp, []string{" -I"}, pyDashes, pythonBenign[1], true)
		add(interp, []string{" -I"}, pyDashes, "rm -rf /tmp/x", false)
	}
	for interp, bodies := range otherBodies {
		for _, b := range bodies {
			add(interp, []string{""}, []string{"", "- "}, b, false)
		}
	}
	return forms
}

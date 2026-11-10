package guard

import (
	"strings"
	"testing"
)

// heredocDeny and heredocExtra are the patterns the heredoc tests run with:
// a Claude settings entry and the belt config's rm entries.
var (
	heredocDeny  = []string{"git config"}
	heredocExtra = []string{"rm -rf", "rm -fr"}
)

// heredocBypassCases are inputs that really run a deny-listed command, which
// the guard on origin/main denied and an earlier relaxed guard let through.
// Every one must deny. cwd holds evil.py, a script that runs the command.
func heredocBypassCases(t *testing.T) (cwd string, cases []scriptDenyCase) {
	t.Helper()
	cwd = t.TempDir()
	writeScript(t, cwd, "evil.py", "import os\nos.system('rm -rf /tmp/x')\n")
	const rm, gc = "rm -rf /tmp/x", "git config --global user.name x"
	// built spells os.system('rm -rf /tmp/x') without the words os, system
	// or import, the way a script would hide it from a word list.
	const built = `"imp" + "ort o" + "s; o" + "s.sy" + "stem('` + rm + `')"`
	return cwd, []scriptDenyCase{
		{
			"stdin dash then a written script",
			"echo '" + rm + "' > /tmp/z.sh; bash - < /tmp/z.sh",
			cwd,
			true,
		},
		{"python dash reads a script file", "python3 - < evil.py", cwd, true},
		{
			"heredoc replaced by a later stdin redirect",
			"python3 - <<'A' < evil.py\nprint(1)\nA",
			cwd,
			true,
		},
		{
			"posix module",
			"python3 -I - <<'A'\nimport posix\nposix.system('" + rm + "')\nA",
			cwd,
			true,
		},
		{
			"os under another name",
			"python3 -I - <<'A'\nimport os\no = os\no.system('" + rm + "')\nA",
			cwd,
			true,
		},
		{
			"sys.modules",
			"python3 -I - <<'A'\nimport sys\nsys.modules['os'].system('" + gc + "')\nA",
			cwd,
			true,
		},
		{
			"vars of a module",
			"python3 -I - <<'A'\nimport os\nvars(os)['system']('" + rm + "')\nA",
			cwd,
			true,
		},
		{
			"node built module name",
			`node -e "require('child_'+'process').execSync('` + rm + `')"`,
			cwd,
			true,
		},
		{
			"node execve",
			`node -e "process.execve('/bin/sh', ['sh', '-c', '` + rm + `'])"`,
			cwd,
			true,
		},
		{"ruby pipe open", `ruby -e 'open "|` + rm + `"'`, cwd, true},
		{"perl readpipe", `perl -e 'readpipe("` + rm + `")'`, cwd, true},
		{"applescript raw event", `osascript -e '«event sysoexec» "` + rm + `"'`, cwd, true},
		{
			"jxa heredoc",
			"osascript -lJavaScript <<EOF\nconst app = Application.currentApplication()\n" +
				"app.includeStandardAdditions = true\napp.doShellScript('" + rm + "')\nEOF",
			cwd,
			true,
		},
		{"output through cat into sh", `python3 -c 'print("` + rm + `")' | cat | sh`, cwd, true},
		{
			"tee into a process substitution",
			"python3 -I - <<'A' | tee >(sh)\nprint('" + rm + "')\nA",
			cwd,
			true,
		},
		{
			"written file piped to bash",
			"python3 -I - <<'A'\nopen('/tmp/zz', 'w').write('" + rm + "')\nA\n" +
				"cat /tmp/zz | bash",
			cwd,
			true,
		},
		{
			"written file run by sh -c",
			"python3 -I - <<'A'\nopen('/tmp/zz', 'w').write('" + rm + "')\nA\n" +
				`sh -c "$(cat /tmp/zz)"`,
			cwd,
			true,
		},
		{
			"second heredoc is the script", "python3 -I - <<'A' <<'B'\nprint(1)\nA\nimport os\nos.system('" + rm + "')\nB",
			cwd, true,
		},
		{
			"arithmetic read as a heredoc",
			"python3 -c 'print(1)' $((1<<2))\nbash -c '" + rm + "'",
			cwd,
			true,
		},
		{"grep run through a shell", "python3 -I - <<'A'\nimport subprocess\n" +
			"subprocess.run(['grep', '-c', '" + rm + "'], executable='/bin/sh')\nA", cwd, true},
		{"exec in a keyword argument", "python3 -I - <<'A'\nimport subprocess\nc = '" + rm + "'\n" +
			"subprocess.run(['true'], cwd=exec(c))\nA", cwd, true},
		{
			"bash 5.3 command substitution",
			"python3 -I - <<EOF\nprint(\"${ " + rm + "; }\")\nEOF",
			cwd,
			true,
		},

		// Found while narrowing the rule.
		{
			"module written then imported without -I",
			"python3 - <<'A'\nopen('traceback.py', 'w').write(" +
				built + ")\n1/0\nA",
			cwd,
			true,
		},
		{
			"-I glued to the delimiter",
			"python3 - <<EOF-I\nopen('traceback.py', 'w').write(" + built +
				")\n1/0\nEOF\n",
			cwd,
			true,
		},
		{
			"stdout sent to sh beforehand",
			"exec > >(sh); python3 -I - <<'A'\nprint('" + rm + "')\nA",
			cwd,
			true,
		},
		{
			"background reader started first",
			"sh /tmp/zz & python3 -I - <<'A'\nopen('/tmp/zz', 'w').write('" +
				rm + "')\nA",
			cwd,
			true,
		},
		{"fullwidth exec", "python3 -I - <<'A'\nｅｘｅｃ(" + built + ")\nA", cwd, true},
		{"builtins through a frame", "python3 -I - <<'A'\ng = (x for x in [1])\n" +
			"g.gi_frame.f_builtins['ev' + 'al'](" + built + ")\nA", cwd, true},
		{"utf-7 source", "python3 -I - <<'A'\n# coding: utf-7\nprint('" + rm + "')\nA", cwd, true},
		{"f-string expression", "python3 -I - <<'A'\nimport pathlib\n" +
			`x = f"{" " + vars(pathlib)['o' + 's'].system('` + rm + `') + " "}"` + "\nA", cwd, true},
		{
			"dash -x shifts the source", "python3 -I -x - <<'A'\n'''\nimport os; os.system('" + rm + "')\n'''\nA",
			cwd, true,
		},
		{
			"unquoted heredoc expands a variable",
			"python3 -I - <<A\nprint('$X " + rm + "')\nA",
			cwd,
			true,
		},
		// A path may be a link to a shell (ln -s /bin/sh python3), which runs
		// the body as shell commands.
		{"relative interpreter path", "./python3 -I - <<'A'\n" + rm + "\nA", cwd, true},
		{"absolute interpreter path", "/opt/py/bin/python3 -I - <<'A'\n" + rm + "\nA", cwd, true},
		{"home interpreter path", "~/bin/python3 -I - <<'A'\n" + rm + "\nA", cwd, true},
		{"interpreter path after cd", "cd /tmp && ./python3 -I - <<'A'\n" + rm + "\nA", cwd, true},
	}
}

// heredocNonBareInterpreters spell the interpreter some way other than the
// bare word python3. The relaxed path must never apply to them.
var heredocNonBareInterpreters = []string{
	"./python3", "/opt/py/bin/python3", "~/bin/python3", "bin/python3",
	`"python3"`, `'python3'`, `\python3`, `pyth""on3`, `python3""`, `$PY`,
}

// TestInertPythonHeredocNeedsBareInterpreter checks that a benign body under
// any non-bare interpreter word gets no relaxed reading. The quoted forms are
// not denied by the guard either way: origin/main does not see a quoted word
// as an interpreter, so its decision for them is unchanged.
func TestInertPythonHeredocNeedsBareInterpreter(t *testing.T) {
	for _, interp := range heredocNonBareInterpreters {
		for _, body := range []string{"rm -rf /tmp/x", "print('rm -rf /tmp/x')"} {
			command := interp + " -I - <<'A'\n" + body + "\nA"
			if _, ok := inertPythonHeredoc(command); ok {
				t.Errorf("read as inert Python under a non-bare interpreter:\n%s", command)
			}
		}
	}
}

// heredocIntendedPasses are the hand-reviewed inputs the guard on origin/main
// denied and the narrowed rule lets through: Python heredocs that are the
// whole command, run with -I, and only read, write or print text that names
// a denied command. The differential test allows exactly these.
var heredocIntendedPasses = []string{
	"python3 -I - <<'EOF'\nimport pathlib\np = pathlib.Path('Makefile')\n" +
		"p.write_text(p.read_text().replace('\trm -rf $(WEB)/.out', ''))\nEOF",
	"cd /repo && python3 -I - <<'PYEOF'\ndoc = '''It reads git config alone: no network, no writes.'''\n" +
		"open('x.md', 'w').write(doc)\nPYEOF",
	"python3 -I <<\"EOF\"\ncases = {\n\t\"sdk-cli; rm -rf --bogus\": \"\",\n}\nprint(len(cases))\nEOF",
	"python3 -I - <<'EOF' > out.txt\nprint('rm -rf /tmp/x')\nEOF",
	"python3 -IB - <<'EOF' 2>&1\nimport json\nprint(json.dumps({'cmd': 'git config --get user.name'}))\nEOF",
	"python3 -I - <<'EOF'\nnote = '''\nbash -c 'rm -rf build'\n'''\nprint(note)\nEOF",
	"python3 -I - <<-'EOF'\n\tfrom pathlib import Path\n\tPath('notes.md').write_text('Run rm -rf build/ to clean.')\n\tEOF",
	"cd /tmp; python3 -I - <<\\EOF\nimport re, textwrap\nprint(textwrap.dedent('''\n    rm -rf /tmp/x\n'''))\nEOF",
	"python3 -I - <<EOF\nprint('rm -rf /tmp/x')\nEOF",
	// Writes a module file whose text runs the command. Safe only because
	// -I keeps the working directory off sys.path, so the interpreter never
	// imports the file it wrote; the same script without -I is denied.
	"python3 -I - <<'EOF'\nopen('traceback.py', 'w').write(\"imp\" + \"ort o\" + \"s; o\" + \"s.sy\" + " +
		"\"stem('rm -rf /tmp/x')\")\n1/0\nEOF",
}

func TestScriptDenyListHeredocBypasses(t *testing.T) {
	cwd, cases := heredocBypassCases(t)
	g := NewScriptDenyList(scriptDenyConfig(heredocDeny, heredocExtra))
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if d := g.Check(Input{Event: EventBash, Command: tt.command, Cwd: cwd}); d == nil {
				t.Fatalf("allowed a command that runs a denied command:\n%s", tt.command)
			}
		})
	}
}

func TestScriptDenyListHeredocIntendedPasses(t *testing.T) {
	g := NewScriptDenyList(scriptDenyConfig(heredocDeny, heredocExtra))
	for _, command := range heredocIntendedPasses {
		if d := g.Check(Input{Event: EventBash, Command: command, Cwd: t.TempDir()}); d != nil {
			t.Errorf("denied an inert Python heredoc:\n%s\n(denial: %s)", command, d.Reason)
		}
	}
}

// TestScriptDenyListPythonHeredocHint checks that a denied Python heredoc
// names the shape that is read as code.
func TestScriptDenyListPythonHeredocHint(t *testing.T) {
	g := NewScriptDenyList(scriptDenyConfig(heredocDeny, heredocExtra))
	d := g.Check(Input{Event: EventBash, Command: "python3 - <<'EOF'\nprint('rm -rf /tmp/x')\nEOF"})
	if d == nil || !strings.Contains(d.Reason, "python3 -I") {
		t.Fatalf("want a denial naming python3 -I, got %+v", d)
	}
	d = g.Check(Input{Event: EventBash, Command: "bash <<'EOF'\nrm -rf /tmp/x\nEOF"})
	if d == nil || strings.Contains(d.Reason, "python3 -I") {
		t.Fatalf("want a shell heredoc denial without the Python hint, got %+v", d)
	}
}

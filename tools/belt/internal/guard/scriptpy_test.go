package guard

import "testing"

func TestInertPythonHeredocShape(t *testing.T) {
	const body = "print('hi')\n"
	tests := []struct {
		name    string
		command string
		ok      bool
		start   int
	}{
		{"bare", "python3 -I - <<'EOF'\n" + body + "EOF", true, 0},
		{"no dash", "python3 -I <<'EOF'\n" + body + "EOF\n", true, 0},
		{"double-quoted delimiter", "python3 -I - <<\"EOF\"\n" + body + "EOF", true, 0},
		{"backslash delimiter", "python3 -I - <<\\EOF\n" + body + "EOF", true, 0},
		{"unquoted literal body", "python3 -I - <<EOF\n" + body + "EOF", true, 0},
		{"tab-stripped", "python3 -I - <<-'EOF'\n\t" + body + "\tEOF", true, 0},
		{"flag cluster", "python3.13 -IB - <<'EOF'\n" + body + "EOF", true, 0},
		{"cd prefix", "cd /tmp && python3 -I - <<'EOF'\n" + body + "EOF", true, 10},
		{"cd prefix with ;", "cd /tmp; python3 -I - <<'EOF'\n" + body + "EOF", true, 8},
		{"cd on its own line", "cd /tmp\npython3 -I - <<'EOF'\n" + body + "EOF", true, 8},
		{"output redirect", "python3 -I - <<'EOF' > out.txt\n" + body + "EOF", true, 0},
		{"attached redirect", "python3 -I - <<'EOF' >out.txt 2>&1\n" + body + "EOF", true, 0},
		{"redirect before heredoc", "python3 -I - > out.txt <<'EOF'\n" + body + "EOF", true, 0},

		{"no -I", "python3 - <<'EOF'\n" + body + "EOF", false, 0},
		{"path to interpreter", "/usr/bin/python3 -I - <<'EOF'\n" + body + "EOF", false, 0},
		{"relative interpreter", "./python3 -I - <<'EOF'\n" + body + "EOF", false, 0},
		{"-I glued to the delimiter", "python3 - <<EOF-I\n" + body + "EOF", false, 0},
		{"-x skips a line", "python3 -I -x - <<'EOF'\n" + body + "EOF", false, 0},
		{"-c", "python3 -I -c 'print(1)' <<'EOF'\n" + body + "EOF", false, 0},
		{"script argument", "python3 -I - notes.md <<'EOF'\n" + body + "EOF", false, 0},
		{"input redirect", "python3 -I - <<'EOF' < evil.py\n" + body + "EOF", false, 0},
		{"second heredoc", "python3 -I - <<'A' <<'B'\nprint(1)\nA\n" + body + "B", false, 0},
		{"here-string", "python3 -I - <<<'print(1)'\n" + body, false, 0},
		{"pipe after", "python3 -I - <<'EOF' | cat\n" + body + "EOF", false, 0},
		{"process substitution", "python3 -I - <<'EOF' > >(sh)\n" + body + "EOF", false, 0},
		{"background", "python3 -I - <<'EOF' &\n" + body + "EOF", false, 0},
		{"command after", "python3 -I - <<'EOF'\n" + body + "EOF\nbash out.sh", false, 0},
		{
			"command after on the operator line",
			"python3 -I - <<'EOF' && bash out.sh\n" + body + "EOF",
			false,
			0,
		},
		{"no terminator", "python3 -I - <<'EOF'\n" + body, false, 0},
		{"terminator with trailing blank", "python3 -I - <<'EOF'\n" + body + "EOF \n", false, 0},
		{"other command before", "echo hi; python3 -I - <<'EOF'\n" + body + "EOF", false, 0},
		{"exec redirect before", "exec > >(sh); python3 -I - <<'EOF'\n" + body + "EOF", false, 0},
		{"env assignment", "PYTHONPATH=. python3 -I - <<'EOF'\n" + body + "EOF", false, 0},
		{"wrapper", "sudo python3 -I - <<'EOF'\n" + body + "EOF", false, 0},
		{"substitution around", "x=$(python3 -I - <<'EOF'\n" + body + "EOF\n)", false, 0},
		{"arithmetic", "python3 -I - <<'EOF' $((1<<2))\n" + body + "EOF", false, 0},
		{"unquoted body that expands", "python3 -I - <<EOF\nprint('$HOME')\nEOF", false, 0},
		{"unquoted body with a backslash", "python3 -I - <<EOF\nprint('a\\\nb')\nEOF", false, 0},
		{"not python", "node - <<'EOF'\nconsole.log(1)\nEOF", false, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, ok := inertPythonHeredoc(tt.command)
			if ok != tt.ok || ok && start != tt.start {
				t.Fatalf(
					"inertPythonHeredoc(%q) = %d, %v; want %d, %v",
					tt.command,
					start,
					ok,
					tt.start,
					tt.ok,
				)
			}
		})
	}
}

func TestPythonInert(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{"print", "print('rm -rf /tmp/x')", true},
		{
			"allowed imports",
			"import json, re as regex\nfrom pathlib import Path\nimport textwrap; import datetime",
			true,
		},
		{"pathlib write", "from pathlib import Path\nPath('n.md').write_text('git config')", true},
		{"re.compile", "import re\nr = re.compile(r'x+')", true},
		{"number with underscores", "n = 1_000_000", true},
		{"throwaway name", "for _ in range(3):\n    print(_)", true},
		{"indented import", "if True:\n    import json", true},

		{"os", "import os", false},
		{"os from an allowed module", "import pathlib\npathlib.os.system('ls')", false},
		{"sys from codecs", "import json\njson.codecs.sys.modules", false},
		{"builtins through enum", "import re\nre.enum.bltns.print('x')", false},
		{"other module", "import plumbum", false},
		{"dotted import", "import json.decoder", false},
		{"from other module", "from shutil import rmtree", false},
		{"relative import", "from . import x", false},
		{"import in a string", "print('import json')", false},
		{"import word in prose", "doc = 'we import data'", false},
		{"bare compile", "compile('1', 'x', 'eval')", false},
		{"exec", "exec('print(1)')", false},
		{"eval", "eval('1')", false},
		{"getattr", "getattr(x, 'y')", false},
		{"vars", "vars()", false},
		{"dunder", "x.__class__", false},
		{"dunder import", "__import__('json')", false},
		{"private name", "import collections\ncollections._sys", false},
		{"frame attribute", "g = (x for x in [])\ng.gi_frame", false},
		{"frame builtins", "f.f_builtins['print']", false},
		{"fullwidth exec", "ｅｘｅｃ('1')", false},
		{"coding declaration", "# -*- coding: utf-7 -*-\nprint(1)", false},
		{"invalid utf-8", "print('\xff')", false},
		{"nul byte", "print(1)\x00", false},
		{"FunctionType", "import collections\ncollections.abc.FunctionType", false},
		{"functools through re", "import re\nre.functools.reduce", true},
		{"methodcaller", "methodcaller('x')", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pythonInert(tt.src); got != tt.want {
				t.Fatalf("pythonInert(%q) = %v, want %v", tt.src, got, tt.want)
			}
		})
	}
}

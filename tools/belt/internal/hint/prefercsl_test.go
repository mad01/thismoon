package hint

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/sourcegraph/zoekt/query"
)

func TestSegments(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []segment // compared with surrounding space trimmed
	}{
		{
			"a pipe stage sits beside its command",
			"make test | grep FAIL",
			[]segment{{"make test", "grep FAIL"}},
		},
		{
			"a command after && is its own",
			"cd /tmp && grep -r foo .",
			[]segment{{"cd /tmp", ""}, {"grep -r foo .", ""}},
		},
		{
			"|| starts a new command, not a pipe stage",
			"test -f x || grep -r foo .",
			[]segment{{"test -f x", ""}, {"grep -r foo .", ""}},
		},
		{
			"a pipe inside quotes is not a separator",
			`grep -r "foo|bar" .`,
			[]segment{{`grep -r "foo|bar" .`, ""}},
		},
		{
			"only the first pipe stage is kept",
			"find . -name '*.go' | xargs grep x | head -5",
			[]segment{{"find . -name '*.go'", "xargs grep x"}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got []segment
			for _, s := range segments(tc.cmd) {
				got = append(got, segment{strings.TrimSpace(s.cmd), strings.TrimSpace(s.pipe)})
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("segments(%q) = %q, want %q", tc.cmd, got, tc.want)
			}
		})
	}
}

func TestParseSweepClassification(t *testing.T) {
	const cwd = "/repo"
	tests := []struct {
		name  string
		seg   string
		sweep bool // true = multi-file sweep worth advising on
	}{
		// The expensive false-positive class: targeted single-file reads are
		// legitimate and must stay silent.
		{"single file grep", `grep -n "func Foo" internal/x.go`, false},
		{"single file with line flag", `grep -n pattern /repo/main.go`, false},
		{"stdin grep", `grep -n pattern`, false},
		{"two named files are filtering", `grep foo a.go b.go`, false},
		{"context flag value is not a path", `grep -n -C 3 foo a.go b.go`, false},
		{"bundled context value is not a path", `grep -nA 3 foo a.go b.go`, false},
		{"stdin redirect is not a path", `rg foo < a.go`, false},
		{"output redirect target is not a path", `grep -n foo a.go > out`, false},

		// Real sweeps.
		{"recursive grep", `grep -r foo /repo`, true},
		{"recursive no path", `grep -r foo`, true},
		{"bundled flags", `grep -rn foo /repo`, true},
		{"bundled with l", `grep -rln foo /repo`, true},
		{"long recursive flag", `grep --recursive foo /repo`, true},
		{"include filter", `grep --include=*.go foo /repo`, true},
		{"three files is multi", `grep foo a.go b.go c.go`, true},
		{"glob operand", `grep foo *.go`, true},
		{"find walks by definition", `find /repo -name "*.go"`, true},
		{"git grep walks by definition", `git grep foo`, true},
		{"git -C grep", `git -C /repo grep -n foo`, true},
		{"fd with a path", `fd '\.go$' /repo`, true},
		{"rg recurses into a directory without -r", `rg foo /repo`, true},
		{"rg over two directories", `rg foo /repo/a /repo/b`, true},
		{"rg type value is not the pattern", `rg --type go foo /repo`, true},
		{"rg with no path may read stdin", `rg foo`, false},
		{"redirects do not hide a sweep", `grep -rn foo . 2>/dev/null`, true},

		// Not searches at all.
		{"ls is not a search", `ls -la /repo`, false},
		{"cat is not a search", `cat /repo/main.go`, false},
		{"git log is not a search", `git -C /repo log --oneline`, false},
		{"bare find searches nothing on macOS", `find -name x`, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSweep(tc.seg, cwd)
			if tc.sweep && got == nil {
				t.Errorf("parseSweep(%q) = nil, want a sweep", tc.seg)
			}
			if !tc.sweep && got != nil {
				t.Errorf("parseSweep(%q) = %+v, want nil", tc.seg, got)
			}
		})
	}
}

func TestQuery(t *testing.T) {
	tests := []struct {
		sw   sweep
		want string
	}{
		{sweep{pattern: `foo`}, `foo`},
		{sweep{pattern: `foo|bar`}, `foo|bar`},
		{sweep{pattern: `foo`, include: "*.go"}, `foo f:\.go$`},
		{sweep{pattern: `foo`, include: "*.md"}, `foo f:\.md$`},
		// A glob that is not a plain extension is dropped, not mistranslated.
		{sweep{pattern: `foo`, include: "src/**/*.go"}, `foo`},
		{sweep{pattern: `foo`, include: "*.{go,md}"}, `foo`},
		{sweep{name: `\.go$`}, `f:\.go$`},
		{sweep{name: `a b`}, `f:"a b"`},
	}
	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			if got := tc.sw.query(); got != tc.want {
				t.Errorf("%+v.query() = %q, want %q", tc.sw, got, tc.want)
			}
		})
	}
}

func TestZoektTerm(t *testing.T) {
	tests := []struct {
		re   string
		want string
	}{
		{`foo`, `foo`},
		{`foo\(`, `foo\(`},
		// A space would make two AND terms; quoted, it is one phrase.
		{`func Foo`, `"func Foo"`},
		// zoekt unescapes one level of backslashes inside quotes.
		{`func Foo\(`, `"func Foo\\("`},
		// A leading - would negate the term.
		{`-x`, `"-x"`},
		{`say "hi"`, `"say \"hi\""`},
	}
	for _, tc := range tests {
		t.Run(tc.re, func(t *testing.T) {
			if got := zoektTerm(tc.re); got != tc.want {
				t.Errorf("zoektTerm(%q) = %q, want %q", tc.re, got, tc.want)
			}
		})
	}
}

func TestParseSweepDialects(t *testing.T) {
	tests := []struct {
		seg  string
		want string // the pattern as RE2
	}{
		// grep reads a BRE: a bare paren is literal, \| is alternation.
		{`grep -rn 'foo(' /r`, `foo\(`},
		{`grep -rn 'a\|b' /r`, `a|b`},
		{`grep -rn 'x\+' /r`, `x+`},
		{`grep -rn '\<word\>' /r`, `\bword\b`},
		{`grep -rn -e 'a(' -e b /r`, `a\(|b`},
		// -E reads an ERE, where \| is a literal pipe.
		{`grep -rnE 'a\|b' /r`, `a\|b`},
		{`grep -rE 'a|b' /r`, `a|b`},
		// -F reads a fixed string.
		{`grep -rnF 'foo(' /r`, `foo\(`},
		{`grep -rn --fixed-strings 'a.b' /r`, `a\.b`},
		{`git -C /r grep -F 'foo('`, `foo\(`},
		{`ag -Q 'foo(' /r`, `foo\(`},
		{`rg -F 'foo(' /r`, `foo\(`},
		// rg takes an extended regex as it is.
		{`rg 'a\|b' /r`, `a\|b`},
		{`rg 'foo\(' /r`, `foo\(`},
	}
	for _, tc := range tests {
		t.Run(tc.seg, func(t *testing.T) {
			got := parseSweep(tc.seg, "/cwd")
			if got == nil {
				t.Fatalf("parseSweep(%q) = nil, want a sweep", tc.seg)
			}
			if got.pattern != tc.want {
				t.Errorf("parseSweep(%q).pattern = %q, want %q", tc.seg, got.pattern, tc.want)
			}
		})
	}
}

// TestAdvisedQueriesParse holds the advice to its contract: the query it
// hands back must be one csl accepts, parsed by the same zoekt parser, and
// mean what the command searched for.
func TestAdvisedQueriesParse(t *testing.T) {
	tests := []struct {
		cmd  string
		want string
	}{
		{`grep -rn "func Foo" /abs`, `"func Foo"`},
		{`grep -rn 'func Foo(' /abs`, `"func Foo\\("`},
		{`grep -rnF 'foo(' /abs`, `foo\(`},
		{`grep -rn 'a\|b' /abs`, `a|b`},
		{`grep -rn --include '*.go' foo /abs`, `foo f:\.go$`},
		{`grep -rn --exclude-dir node_modules foo /abs`, `foo`},
		{`rg --glob '*.go' foo /abs`, `foo f:\.go$`},
		{`rg --type go foo /abs`, `foo`},
		{`grep -rn -e '-x' /abs`, `"-x"`},
		{`find /abs -name '*.go' | xargs grep x`, `x f:\.go$`},
		{`find /abs -name '*.go' -print0 | xargs -0 -n 1 grep -l 'a b'`, `"a b" f:\.go$`},
		{`fd -e go | xargs rg -F 'foo('`, `foo\( f:\.go$`},
	}
	for _, tc := range tests {
		t.Run(tc.cmd, func(t *testing.T) {
			seg := segments(tc.cmd)[0]
			sw := parseSweep(seg.cmd, "/cwd")
			if sw == nil {
				t.Fatalf("parseSweep(%q) = nil, want a sweep", seg.cmd)
			}
			sw.takePipe(seg.pipe)
			got := sw.query()
			if got != tc.want {
				t.Errorf("query for %q = %q, want %q", tc.cmd, got, tc.want)
			}
			if _, err := query.Parse(got); err != nil {
				t.Errorf("query for %q = %q, which zoekt rejects: %v", tc.cmd, got, err)
			}
		})
	}
}

func TestSplitFieldsKeepsQuotedSpans(t *testing.T) {
	got := splitFields(`grep -n "func Foo" x.go`)
	want := []string{"grep", "-n", `"func Foo"`, "x.go"}
	if !slices.Equal(got, want) {
		t.Errorf("splitFields = %q, want %q", got, want)
	}
}

func TestDropRedirects(t *testing.T) {
	tests := []struct {
		words []string
		want  []string
	}{
		{[]string{"foo", "go.mod", "2>/dev/null"}, []string{"foo", "go.mod"}},
		{[]string{"foo", "a.go", "2>&1"}, []string{"foo", "a.go"}},
		{[]string{"foo", "a.go", ">", "out"}, []string{"foo", "a.go"}},
		{[]string{"foo", "a.go", ">>", "log", "2>&1"}, []string{"foo", "a.go"}},
		{[]string{"foo", "<", "file"}, []string{"foo"}},
		{[]string{"foo", "&>", "log", "."}, []string{"foo", "."}},
		{[]string{"foo", "&>log", "."}, []string{"foo", "."}},
		{[]string{"foo", "2>", "err", "."}, []string{"foo", "."}},
		{[]string{"'>'", "."}, []string{"'>'", "."}},
	}
	for _, tc := range tests {
		if got := dropRedirects(tc.words); !slices.Equal(got, tc.want) {
			t.Errorf("dropRedirects(%q) = %q, want %q", tc.words, got, tc.want)
		}
	}
}

func TestCheckStaysQuietWithoutIndex(t *testing.T) {
	// indexedRepos reads the real csl index dir. Whatever it holds, a command
	// with no search in it must never produce advice.
	h := NewPreferCSL(testConfig())
	if got := h.Check(Input{Event: EventBash, Command: "go test ./...", Cwd: "/repo"}); got != nil {
		t.Errorf("Check on a non-search command = %+v, want nil", got)
	}
}

func TestParseSweepFields(t *testing.T) {
	tests := []struct {
		name    string
		seg     string
		tool    string
		pattern string
		include string
		targets []string
	}{
		{
			"value flag before pattern",
			`grep -rn -C 3 foo /abs/repo`,
			"grep", "foo", "",
			[]string{"/abs/repo"},
		},
		{
			"-e patterns join as alternation",
			`grep -r -e foo -e bar /abs/repo`,
			"grep", "foo|bar", "",
			[]string{"/abs/repo"},
		},
		{
			"--regexp is -e",
			`grep -r --regexp foo /abs/repo`,
			"grep", "foo", "",
			[]string{"/abs/repo"},
		},
		{
			"--include takes the next word",
			`grep -rn --include '*.go' foo /abs/repo`,
			"grep", "foo", "*.go",
			[]string{"/abs/repo"},
		},
		{
			"--include takes an attached value",
			`grep -rn --include=*.go foo /abs/repo`,
			"grep", "foo", "*.go",
			[]string{"/abs/repo"},
		},
		{
			"--exclude-dir takes the next word",
			`grep -rn --exclude-dir node_modules foo /abs/repo`,
			"grep", "foo", "",
			[]string{"/abs/repo"},
		},
		{
			"a bundled value flag takes the next word",
			`grep -rnA 3 foo /abs/repo`,
			"grep", "foo", "",
			[]string{"/abs/repo"},
		},
		{
			"rg glob becomes the include",
			`rg -g '*.go' foo /abs/repo`,
			"rg", "foo", "*.go",
			[]string{"/abs/repo"},
		},
		{
			"rg --glob takes the next word",
			`rg --glob '*.go' foo /abs/repo`,
			"rg", "foo", "*.go",
			[]string{"/abs/repo"},
		},
		{
			"rg --type takes the next word",
			`rg --type go foo /abs/repo`,
			"rg", "foo", "",
			[]string{"/abs/repo"},
		},
		{
			"find -name becomes the include",
			`find /abs/repo -type f -name '*.md'`,
			"find", "", "*.md",
			[]string{"/abs/repo"},
		},
		{
			"git -C names the directory",
			`git -C /abs/repo grep -n foo`,
			"git grep", "foo", "",
			[]string{"/abs/repo"},
		},
		{
			"git -C nests and takes pathspecs",
			`git -C /abs -c core.x=y grep foo -- repo`,
			"git grep", "foo", "",
			[]string{"/abs/repo"},
		},
		{"relative path joins the cwd", `grep -r foo sub`, "grep", "foo", "", []string{"/cwd/sub"}},
		{
			"redirect targets are not paths",
			`grep -rn foo /abs/repo > out 2>&1`,
			"grep", "foo", "",
			[]string{"/abs/repo"},
		},
		{
			"every path is a target",
			`grep -r x /tmp /abs/repo`,
			"grep", "x", "",
			[]string{"/tmp", "/abs/repo"},
		},
		{
			"a path only the shell knows is left out",
			`grep -r x $DIR /abs/repo`,
			"grep", "x", "",
			[]string{"/abs/repo"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSweep(tc.seg, "/cwd")
			if got == nil {
				t.Fatalf("parseSweep(%q) = nil, want a sweep", tc.seg)
			}
			if got.tool != tc.tool || got.pattern != tc.pattern || got.include != tc.include ||
				!slices.Equal(got.targets, tc.targets) {
				t.Errorf(
					"parseSweep(%q) = %+v, want tool %q pattern %q include %q targets %q",
					tc.seg, *got, tc.tool, tc.pattern, tc.include, tc.targets,
				)
			}
		})
	}
}

func TestParseSweepFd(t *testing.T) {
	tests := []struct {
		seg     string
		name    string
		include string
		targets []string
	}{
		{`fd '\.go$' /abs/repo`, `\.go$`, "", []string{"/abs/repo"}},
		{`fd -F 'a.b' /abs/repo`, `a\.b`, "", []string{"/abs/repo"}},
		{`fd -e go`, "", "*.go", []string{"/cwd"}},
		{`fd --extension go`, "", "*.go", []string{"/cwd"}},
	}
	for _, tc := range tests {
		got := parseSweep(tc.seg, "/cwd")
		if got == nil {
			t.Fatalf("parseSweep(%q) = nil, want a sweep", tc.seg)
		}
		if got.name != tc.name || got.pattern != "" || got.include != tc.include ||
			!slices.Equal(got.targets, tc.targets) {
			t.Errorf("parseSweep(%q) = %+v, want name %q include %q targets %q",
				tc.seg, *got, tc.name, tc.include, tc.targets)
		}
	}
}

func TestParseSweepUnknownPathStaysQuiet(t *testing.T) {
	// A path the shell would expand cannot be resolved without running it;
	// resolving it against the cwd would name the wrong repo.
	for _, seg := range []string{
		`grep -rn foo $DIR`,
		"grep -rn foo `pwd`/x",
		`grep -rn foo ~bob/src`,
		`git -C $REPO grep foo`,
	} {
		if got := parseSweep(seg, "/cwd"); got != nil {
			t.Errorf("parseSweep(%q) = %+v, want nil", seg, got)
		}
	}
}

func TestParseSweepRecursiveNamedFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		seg   string
		sweep bool
	}{
		// -r on a named file still reads that one file.
		{`grep -rn foo a.go`, false},
		{`grep -rn foo a.go b.go`, false},
		{`git grep foo -- a.go`, false},
		{`grep -rn foo a.go b.go c.go`, true},
		{`grep -rn foo .`, true},
		{`grep -rn foo missing.go`, true}, // not a file on disk: read as a directory
		{`rg foo a.go b.go`, false},
		{`rg foo a.go .`, true},
		// A redirection is neither a pattern nor a path.
		{`rg -n foo a.go 2>/dev/null`, false},
		{`grep -n foo a.go b.go 2>&1`, false},
		{`grep -n foo a.go > out`, false},
		{`rg foo < a.go`, false},
		{`grep -rn foo a.go >> log 2>&1`, false},
	}
	for _, tc := range tests {
		got := parseSweep(tc.seg, dir)
		if tc.sweep != (got != nil) {
			t.Errorf("parseSweep(%q) = %+v, want sweep=%v", tc.seg, got, tc.sweep)
		}
	}
}

func TestWorkDirsFollowCD(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		cmd  string
		want []string // the directory of each non-cd segment, in order
	}{
		{"cd then search", `cd /a && grep -r foo .`, []string{"/a"}},
		{"relative cd", `cd sub && grep -r foo .`, []string{"/cwd/sub"}},
		{"cd with a flag", `cd -P /a; grep -r foo .`, []string{"/a"}},
		{"cd with a redirect", `cd /a 2>/dev/null && grep -r foo .`, []string{"/a"}},
		{"bare cd goes home", `cd && grep -r foo .`, []string{home}},
		{"cd - is unknown", `cd - && grep -r foo .`, []string{""}},
		{"cd to a variable is unknown", `cd "$REPO" && grep -r foo .`, []string{""}},
		{
			"subshell cd ends with the subshell",
			`(cd /a && grep -r x .) ; grep -r y .`,
			[]string{"/a", "/cwd"},
		},
		{"no cd keeps the cwd", `grep -r foo . && grep -r bar .`, []string{"/cwd", "/cwd"}},
		{"pushd moves like cd", `pushd /a && grep -r foo .`, []string{"/a"}},
		{
			"popd moves back",
			`pushd /a && grep -r x . && popd && grep -r y .`,
			[]string{"/a", "/cwd"},
		},
		{"popd with no pushd is unknown", `popd && grep -r foo .`, []string{""}},
		{"pushd rotating the stack is unknown", `pushd +1 && grep -r foo .`, []string{""}},
		{
			"a brace group cd stays in effect",
			`{ cd /a; grep -r x .; }; grep -r y .`,
			[]string{"/a", "/a"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dirs := workDirs{cur: "/cwd"}
			var got []string
			for _, seg := range segments(tc.cmd) {
				if cmd, dir := dirs.step(seg.cmd); cmd != "" {
					got = append(got, dir)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("dirs for %q = %q, want %q", tc.cmd, got, tc.want)
			}
		})
	}
}

// TestCheckNamesTheSearchedRepo is the MAD-518 regression: the advice names
// the repo the command searched, not the session's working directory.
func TestCheckNamesTheSearchedRepo(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "org", "alpha")
	beta := filepath.Join(root, "org", "beta")
	for _, repo := range []string{alpha, beta} {
		if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"a.go", "b.go", "c.go"} {
			if err := os.WriteFile(filepath.Join(repo, name), []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	h := NewPreferCSL(testConfig())
	h.indexed = func() map[string]bool { return map[string]bool{"org/alpha": true, "org/beta": true} }
	h.resolveRepo = func(string) string { return "" }

	tests := []struct {
		name string
		cmd  string
		want string // repo the advice names; "" = no advice
	}{
		{"cwd fallback", `grep -rn foo .`, "org/alpha"},
		{"cd target", "cd " + beta + ` && grep -rn foo .`, "org/beta"},
		{"pushd target", "pushd " + beta + ` && grep -rn foo .`, "org/beta"},
		{"brace group cd", "{ cd " + beta + `; grep -rn foo .; }`, "org/beta"},
		{"git -C target", "git -C " + beta + " grep -n foo", "org/beta"},
		{"absolute path argument", "grep -rn foo " + beta, "org/beta"},
		{"first indexed path wins", "grep -rn foo " + root + " " + beta, "org/beta"},
		{"unknown cd stays quiet", `cd "$OTHER" && grep -rn foo .`, ""},
		{"two named files stay quiet", `grep -n foo a.go a.go`, ""},
		{"stderr redirect stays quiet", `rg -n foo a.go 2>/dev/null`, ""},
		{"stdin redirect stays quiet", `rg foo < a.go`, ""},
		{"output redirect stays quiet", `grep -n foo a.go b.go > out`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := h.Check(Input{Event: EventBash, Command: tc.cmd, Cwd: alpha})
			if tc.want == "" {
				if got != nil {
					t.Errorf("Check(%q) = %q, want no advice", tc.cmd, got.Text)
				}
				return
			}
			if got == nil {
				t.Fatalf("Check(%q) = nil, want advice naming %s", tc.cmd, tc.want)
			}
			for _, want := range []string{"--repo '^" + tc.want + "$'", `repo="^` + tc.want + `$"`} {
				if !strings.Contains(got.Text, want) {
					t.Errorf("Check(%q) = %q, want it to hold %s", tc.cmd, got.Text, want)
				}
			}
		})
	}
}

func TestCheckWording(t *testing.T) {
	root := t.TempDir()
	alpha := filepath.Join(root, "org", "alpha")
	if err := os.MkdirAll(filepath.Join(alpha, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := NewPreferCSL(testConfig())
	h.indexed = func() map[string]bool { return map[string]bool{"org/alpha": true} }
	h.resolveRepo = func(string) string { return "" }

	tests := []struct {
		cmd  string
		want string
	}{
		{`grep -rn foo .`, "a recursive grep re-walks the filesystem"},
		{`grep -n foo a.go b.go c.go`, "a grep over several files reads them from disk"},
		{`find . -name '*.go' | xargs grep foo`, `csl search 'foo f:\.go$'`},
	}
	for _, tc := range tests {
		got := h.Check(Input{Event: EventBash, Command: tc.cmd, Cwd: alpha})
		if got == nil {
			t.Fatalf("Check(%q) = nil, want advice", tc.cmd)
		}
		if !strings.Contains(got.Text, tc.want) {
			t.Errorf("Check(%q) = %q, want it to hold %q", tc.cmd, got.Text, tc.want)
		}
	}
}

func TestAdviceRendersCLIFirst(t *testing.T) {
	sw := &sweep{tool: "grep", walks: true, pattern: `it's|x`, include: "*.go"}
	got := sw.advice("org/alpha")

	cli := `csl search 'it'\''s|x f:\.go$' --repo '^org/alpha$'.`
	mcp := `csl_search repo="^org/alpha$" query="it's|x f:\\.go$"`
	note := "Note zoekt is not grep"
	for _, want := range []string{cli, mcp, note} {
		if !strings.Contains(got, want) {
			t.Fatalf("advice = %q, missing %q", got, want)
		}
	}
	iCLI, iNote, iMCP := strings.Index(got, cli), strings.Index(got, note), strings.Index(got, mcp)
	if iCLI > iNote || iNote > iMCP {
		t.Errorf("advice = %q, want the CLI form, then the zoekt note, then the MCP form", got)
	}
}

func TestAdviceAnchorsTheRepo(t *testing.T) {
	sw := &sweep{tool: "grep", walks: true, pattern: "x"}
	got := sw.advice("org/a.b")
	for _, want := range []string{`--repo '^org/a\.b$'`, `repo="^org/a\\.b$"`} {
		if !strings.Contains(got, want) {
			t.Errorf("advice = %q, missing %q", got, want)
		}
	}
}

func TestAdviceForListings(t *testing.T) {
	tests := []struct {
		name string
		sw   sweep
		want []string // in order
	}{
		{
			"fd pattern becomes an f: filter",
			sweep{tool: "fd", walks: true, name: `\.go$`},
			[]string{`csl search 'f:\.go$' --repo '^org/alpha$'`, `csl_search repo="^org/alpha$"`},
		},
		{
			"find -name becomes an f: filter",
			sweep{tool: "find", walks: true, include: "*.md"},
			[]string{`csl search 'f:\.md$' --repo '^org/alpha$'`, `csl_search repo="^org/alpha$"`},
		},
		{
			"untranslatable listing falls back to a template",
			sweep{tool: "find", walks: true},
			[]string{`csl search 'f:<regex>' --repo '^org/alpha$'`, `csl_ls repo="^org/alpha$"`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.sw.advice("org/alpha")
			last := -1
			for _, w := range tc.want {
				i := strings.Index(got, w)
				if i < 0 {
					t.Fatalf("advice = %q, missing %q", got, w)
				}
				if i < last {
					t.Errorf("advice = %q, %q comes out of order", got, w)
				}
				last = i
			}
		})
	}
}

package hint

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSegmentsSkipsPipeStages(t *testing.T) {
	tests := []struct {
		name string
		cmd  string
		want []string // segments that must be present
		skip []string // substrings that must NOT appear in any segment
	}{
		{
			name: "pipe filter is dropped",
			cmd:  "make test | grep FAIL",
			skip: []string{"grep FAIL"},
		},
		{
			name: "command after && is kept",
			cmd:  "cd /tmp && grep -r foo .",
			want: []string{"grep -r foo ."},
		},
		{
			name: "|| starts a new command, not a pipe stage",
			cmd:  "test -f x || grep -r foo .",
			want: []string{"grep -r foo ."},
		},
		{
			name: "pipe inside quotes is not a separator",
			cmd:  `grep -r "foo|bar" .`,
			want: []string{`grep -r "foo|bar" .`},
		},
		{
			name: "only the stage after the pipe is dropped",
			cmd:  "grep -r foo . | head -5",
			want: []string{"grep -r foo . "},
			skip: []string{"head"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			segs := segments(tc.cmd)
			joined := strings.Join(segs, "\x00")
			for _, w := range tc.want {
				if !strings.Contains(joined, w) {
					t.Errorf("segments(%q) = %q, missing %q", tc.cmd, segs, w)
				}
			}
			for _, s := range tc.skip {
				if strings.Contains(joined, s) {
					t.Errorf("segments(%q) = %q, should not contain %q", tc.cmd, segs, s)
				}
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

		// Real sweeps.
		{"recursive grep", `grep -r foo /repo`, true},
		{"recursive no path", `grep -r foo`, true},
		{"bundled flags", `grep -rn foo /repo`, true},
		{"bundled with l", `grep -rln foo /repo`, true},
		{"include filter", `grep --include=*.go foo /repo`, true},
		{"three files is multi", `grep foo a.go b.go c.go`, true},
		{"glob operand", `grep foo *.go`, true},
		{"find walks by definition", `find /repo -name "*.go"`, true},
		{"git grep walks by definition", `git grep foo`, true},
		{"git -C grep", `git -C /repo grep -n foo`, true},
		{"fd with a path", `fd '\.go$' /repo`, true},
		{"rg recurses into a directory without -r", `rg foo /repo`, true},
		{"rg over two directories", `rg foo /repo/a /repo/b`, true},
		{"rg with no path may read stdin", `rg foo`, false},

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

func TestZoektTranslation(t *testing.T) {
	tests := []struct {
		pattern string
		include string
		want    string
	}{
		{`foo`, "", `foo`},
		{`foo\|bar`, "", `foo|bar`},
		{`foo`, "*.go", `foo f:\.go$`},
		{`foo`, "*.md", `foo f:\.md$`},
		// A glob that is not a plain extension is dropped, not mistranslated.
		{`foo`, "src/**/*.go", `foo`},
		{`foo`, "*.{go,md}", `foo`},
	}
	for _, tc := range tests {
		t.Run(tc.pattern+"|"+tc.include, func(t *testing.T) {
			got := zoektQuery(tc.pattern)
			if f := zoektFileFilter(tc.include); f != "" {
				got += " " + f
			}
			if got != tc.want {
				t.Errorf("translate(%q, %q) = %q, want %q", tc.pattern, tc.include, got, tc.want)
			}
		})
	}
}

func TestSplitFieldsKeepsQuotedSpans(t *testing.T) {
	got := splitFields(`grep -n "func Foo" x.go`)
	want := []string{"grep", "-n", `"func Foo"`, "x.go"}
	if len(got) != len(want) {
		t.Fatalf("splitFields = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("field %d = %q, want %q", i, got[i], want[i])
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
		target  string
	}{
		{
			"value flag before pattern",
			`grep -rn -C 3 foo /abs/repo`,
			"grep",
			"foo",
			"",
			"/abs/repo",
		},
		{
			"-e patterns join as alternation",
			`grep -r -e foo -e bar /abs/repo`,
			"grep",
			"foo|bar",
			"",
			"/abs/repo",
		},
		{
			"rg glob becomes the include",
			`rg -g '*.go' foo /abs/repo`,
			"rg",
			"foo",
			"*.go",
			"/abs/repo",
		},
		{"fd pattern first, then the path", `fd '\.go$' /abs/repo`, "fd", `\.go$`, "", "/abs/repo"},
		{"bare fd sweeps its directory", `fd -e go`, "fd", "", "*.go", "/cwd"},
		{
			"find -name becomes the include",
			`find /abs/repo -type f -name '*.md'`,
			"find",
			"",
			"*.md",
			"/abs/repo",
		},
		{
			"git -C names the directory",
			`git -C /abs/repo grep -n foo`,
			"git grep",
			"foo",
			"",
			"/abs/repo",
		},
		{
			"git -C nests and takes pathspecs",
			`git -C /abs -c core.x=y grep foo -- repo`,
			"git grep",
			"foo",
			"",
			"/abs/repo",
		},
		{"relative path joins the cwd", `grep -r foo sub`, "grep", "foo", "", "/cwd/sub"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseSweep(tc.seg, "/cwd")
			if got == nil {
				t.Fatalf("parseSweep(%q) = nil, want a sweep", tc.seg)
			}
			want := sweep{
				tool:    tc.tool,
				pattern: tc.pattern,
				include: tc.include,
				target:  tc.target,
			}
			if *got != want {
				t.Errorf("parseSweep(%q) = %+v, want %+v", tc.seg, *got, want)
			}
		})
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
		{"bare cd goes home", `cd && grep -r foo .`, []string{home}},
		{"cd - is unknown", `cd - && grep -r foo .`, []string{""}},
		{"cd to a variable is unknown", `cd "$REPO" && grep -r foo .`, []string{""}},
		{
			"subshell cd ends with the subshell",
			`(cd /a && grep -r x .) ; grep -r y .`,
			[]string{"/a", "/cwd"},
		},
		{"no cd keeps the cwd", `grep -r foo . && grep -r bar .`, []string{"/cwd", "/cwd"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dirs := workDirs{cur: "/cwd"}
			var got []string
			for _, seg := range segments(tc.cmd) {
				if cmd, dir := dirs.step(seg); cmd != "" {
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
		if err := os.WriteFile(filepath.Join(repo, "a.go"), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
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
		{"git -C target", "git -C " + beta + " grep -n foo", "org/beta"},
		{"absolute path argument", "grep -rn foo " + beta, "org/beta"},
		{"unknown cd stays quiet", `cd "$OTHER" && grep -rn foo .`, ""},
		{"two named files stay quiet", `grep -n foo a.go a.go`, ""},
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
			if !strings.Contains(got.Text, "--repo '"+tc.want+"'") {
				t.Errorf("Check(%q) = %q, want it to name %s", tc.cmd, got.Text, tc.want)
			}
		})
	}
}

func TestAdviceRendersCLIFirst(t *testing.T) {
	sw := &sweep{tool: "grep", pattern: `it's\|x`, include: "*.go"}
	got := sw.advice("org/alpha")

	cli := `csl search 'it'\''s|x f:\.go$' --repo 'org/alpha'.`
	mcp := `csl_search repo="org/alpha" query="it's|x f:\\.go$"`
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

func TestAdviceForListings(t *testing.T) {
	tests := []struct {
		name string
		sw   sweep
		want []string // in order
	}{
		{
			"fd pattern becomes an f: filter",
			sweep{tool: "fd", pattern: `\.go$`},
			[]string{`csl search 'f:\.go$' --repo 'org/alpha'`, `csl_search repo="org/alpha"`},
		},
		{
			"find -name becomes an f: filter",
			sweep{tool: "find", include: "*.md"},
			[]string{`csl search 'f:\.md$' --repo 'org/alpha'`, `csl_search repo="org/alpha"`},
		},
		{
			"untranslatable listing falls back to a template",
			sweep{tool: "find"},
			[]string{`csl search 'f:<regex>' --repo 'org/alpha'`, `csl_ls repo="org/alpha"`},
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

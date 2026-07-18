package hint

import (
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

		// Real sweeps.
		{"recursive grep", `grep -r foo /repo`, true},
		{"recursive no path", `grep -r foo`, true},
		{"bundled flags", `grep -rn foo /repo`, true},
		{"bundled with l", `grep -rln foo /repo`, true},
		{"include filter", `grep --include=*.go foo /repo`, true},
		{"two files is multi", `grep foo a.go b.go`, true},
		{"glob operand", `grep foo *.go`, true},
		{"find walks by definition", `find /repo -name "*.go"`, true},

		// Not searches at all.
		{"ls is not a search", `ls -la /repo`, false},
		{"cat is not a search", `cat /repo/main.go`, false},
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

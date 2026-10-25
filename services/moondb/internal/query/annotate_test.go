package query

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/moondb/internal/gitlink"
)

// lineSummary is the part of an AnnotatedLine a test pins, in one string.
func lineSummary(l AnnotatedLine) string {
	return fmt.Sprintf("%d %s %s %s", l.Line, l.Method, l.EditID, l.Commit)
}

func TestAnnotate(t *testing.T) {
	const added = "func Two() int { return compute(2) }"
	dir, head := gitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), committed+added+"\n}\n")
	made := edit("e1", 1, dir, "a.go")
	made.AfterBlob = gitlink.BlobID([]byte(committed))
	made.AgentType = "builder"

	tests := []struct {
		name string
		seed func(f *fixture) []any
		want []string
	}{
		{
			name: "committed by blame, uncommitted by content, trivial none",
			seed: func(f *fixture) []any {
				return []any{made, f.adding(edit("e2", 2, dir, "a.go"), added)}
			},
			want: []string{
				"1 blame e1 " + head,
				"2 blame e1 " + head,
				"3 blame e1 " + head,
				"4 content e2 ",
				"5 none  ",
			},
		},
		{
			name: "committed line no edit holds falls through to content",
			seed: func(f *fixture) []any {
				return []any{f.adding(edit("e3", 3, dir, "a.go"), "func One() int { return 1 }")}
			},
			want: []string{
				"1 none  " + head,
				"2 none  " + head,
				"3 content e3 " + head,
				"4 none  ",
				"5 none  ",
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.seed(tc.seed(f)...)
			res, err := f.svc.Annotate(context.Background(),
				AnnotateRequest{Worktree: dir, Path: "a.go"})
			if err != nil {
				t.Fatal(err)
			}
			got := make([]string, len(res.Lines))
			for i, l := range res.Lines {
				got[i] = lineSummary(l)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf(
					"lines =\n%s\nwant\n%s",
					strings.Join(got, "\n"),
					strings.Join(tc.want, "\n"),
				)
			}
			if res.Worktree != dir || res.Path != "a.go" || len(res.Lines) != 5 ||
				res.Lines[3].Text != added {
				t.Errorf("result = %s %s with %d lines, line 4 %q", res.Worktree, res.Path,
					len(res.Lines), res.Lines[3].Text)
			}
		})
	}
}

func TestAnnotateCarriesTheEdit(t *testing.T) {
	dir, _ := gitRepo(t)
	made := edit("e1", 1, dir, "a.go")
	made.AfterBlob = gitlink.BlobID([]byte(committed))
	made.AgentType = "builder"
	f := newFixture(t)
	f.seed(made)
	res, err := f.svc.Annotate(context.Background(), AnnotateRequest{Worktree: dir, Path: "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	l := res.Lines[2]
	if l.Text != "func One() int { return 1 }" || l.TurnID != "p1" || l.SessionID != "s1" ||
		l.AgentType != "builder" {
		t.Errorf("line 3 = %+v, want e1's text, turn, session and agent", l)
	}
}

// TestAnnotateAgreesWithWhy pins that a file view and a single why answer
// name the same edit for every line.
func TestAnnotateAgreesWithWhy(t *testing.T) {
	const added = "func Two() int { return compute(2) }"
	dir, _ := gitRepo(t)
	writeFile(t, filepath.Join(dir, "a.go"), committed+added+"\n")
	made := edit("e1", 1, dir, "a.go")
	made.AfterBlob = gitlink.BlobID([]byte(committed))
	f := newFixture(t)
	f.seed(made, f.adding(edit("e2", 2, dir, "a.go"), added))
	res, err := f.svc.Annotate(context.Background(), AnnotateRequest{Worktree: dir, Path: "a.go"})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range res.Lines {
		why, err := f.svc.Why(context.Background(),
			WhyRequest{Worktree: dir, Path: "a.go", Line: l.Line})
		if err != nil {
			t.Fatal(err)
		}
		whyEdit := ""
		if why.Edit != nil {
			whyEdit = why.Edit.ID
		}
		if why.Method != l.Method || whyEdit != l.EditID {
			t.Errorf("line %d: annotate %s %s, why %s %s", l.Line, l.Method, l.EditID,
				why.Method, whyEdit)
		}
	}
}

func TestAnnotateUntrackedFileByContent(t *testing.T) {
	dir, _ := gitRepo(t)
	writeFile(t, filepath.Join(dir, "new.go"), "x := one()\n\n")
	f := newFixture(t)
	f.seed(f.adding(edit("e1", 1, dir, "new.go"), "x := one()"))
	res, err := f.svc.Annotate(context.Background(), AnnotateRequest{Worktree: dir, Path: "new.go"})
	if err != nil {
		t.Fatal(err)
	}
	got := []string{lineSummary(res.Lines[0]), lineSummary(res.Lines[1])}
	if want := []string{"1 content e1 ", "2 none  "}; !slices.Equal(got, want) {
		t.Errorf("lines = %q, want %q", got, want)
	}
}

func TestAnnotateInvalidRequest(t *testing.T) {
	dir, _ := gitRepo(t)
	f := newFixture(t)
	f.seed(edit("e1", 1, dir, "a.go"))
	for _, req := range []AnnotateRequest{
		{Worktree: dir},
		{Path: "a.go"},
		{Worktree: "rel/wt", Path: "a.go"},
		{Worktree: dir, Path: "../escape.go"},
		{Worktree: dir, Path: "missing.go"},
		{Worktree: dir, Path: "."},
		{Worktree: t.TempDir(), Path: "a.go"},
	} {
		if _, err := f.svc.Annotate(context.Background(), req); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("Annotate(%+v) = %v, want ErrInvalidRequest", req, err)
		}
	}
}

func TestFileLines(t *testing.T) {
	tests := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"\n", []string{""}},
		{"a\nb\n", []string{"a", "b"}},
		{"a\nb", []string{"a", "b"}},
		{"a\r\n\r\nb\r\n", []string{"a", "", "b"}},
		{"a\n\n", []string{"a", ""}},
	}
	for _, tc := range tests {
		if got := fileLines([]byte(tc.in)); !slices.Equal(got, tc.want) {
			t.Errorf("fileLines(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseBlameFile(t *testing.T) {
	const one = "1a612e4313cddd60ef596c66f2fccaacea2e8410"
	two, zero := strings.Repeat("c2", 20), strings.Repeat("0", 40)
	out := strings.Join([]string{
		one + " 1 1 2", "author t", "summary init", "filename old.go", "\tpackage a",
		one + " 2 2", "\t",
		two + " 3 3 1", "author t", "filename a.go", "\tfunc One() int { return 1 }",
		zero + " 4 4 1", "author Not Committed Yet", "filename a.go", "\tx := 1",
		one + " 5 5 1", "filename old.go", "\t",
	}, "\n")
	// gitloc.Output trims the output, which cuts the last line's tab.
	got := parseBlameFile(strings.TrimSpace(out))
	want := []blamed{
		{commit: one, path: "old.go", text: "package a"},
		{commit: one, path: "old.go", text: ""},
		{commit: two, path: "a.go", text: "func One() int { return 1 }"},
		{path: "a.go", text: "x := 1"},
		{commit: one, path: "old.go", text: ""},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseBlameFile =\n%+v\nwant\n%+v", got, want)
	}
	for _, bad := range []string{"", "fatal: no such path 'a.go' in HEAD", "\tstray text"} {
		if got := parseBlameFile(bad); got != nil {
			t.Errorf("parseBlameFile(%q) = %+v, want nil", bad, got)
		}
	}
}

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repoRootDir is the thismoon checkout the package sits in, so the gates
// can read corpus files without a csl search.
func repoRootDir(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("repo root %s: %v", root, err)
	}
	return root
}

// anchorCorpus builds a case's corpus from its anchors alone, read from the
// checkout. The real run resolves the query; the gates only need the files
// the facts live in.
func anchorCorpus(t *testing.T, root string, c Case) Corpus {
	t.Helper()
	corpus := Corpus{Repo: c.Repo, Root: root}
	for _, a := range c.Anchors {
		f, err := readCorpusFile(root, a)
		if err != nil {
			t.Fatalf("%s: %v", c.ID, err)
		}
		corpus.Files = append(corpus.Files, f)
	}
	return corpus
}

func TestCasesLoad(t *testing.T) {
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) < 8 {
		t.Fatalf("got %d cases, want at least 8", len(cases))
	}
	for _, c := range cases {
		if len(c.Anchors) == 0 {
			t.Errorf("%s: every case needs an anchor so the gates can read its files", c.ID)
		}
		if c.Query == "" {
			t.Errorf("%s: every shipped case is query-driven; files is for ad-hoc runs", c.ID)
		}
	}
}

// TestGates is the offline half of -oracle: the oracle scores full marks
// with no wrong line claim, an empty answer scores none, and the corpus
// echoed back fails the budget.
func TestGates(t *testing.T) {
	root := repoRootDir(t)
	cases, err := loadCases("cases")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		corpus := anchorCorpus(t, root, c)
		g := grade(c, corpus, c.Oracle)
		if !g.Pass || g.LineClaimsWrong != 0 {
			t.Errorf(
				"%s: oracle scored %d/%d, forbidden %v, budget %d/%d, wrong line claims %d of %d; missing %v",
				c.ID,
				g.Found,
				g.Total,
				g.Forbidden,
				g.Chars,
				g.Budget,
				g.LineClaimsWrong,
				g.LineClaims,
				g.Missing,
			)
		}
		if g := grade(c, corpus, ""); g.Found != 0 || g.Pass {
			t.Errorf("%s: empty answer scored %d/%d", c.ID, g.Found, g.Total)
		}
		if g := grade(c, corpus, corpus.dump()); g.BudgetOK || g.Pass {
			t.Errorf(
				"%s: the corpus dump passed the budget (%d chars of %d)",
				c.ID,
				g.Chars,
				g.Budget,
			)
		}
	}
}

func TestParseCaseRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"unknown key", "---\nid: a\nquery: q\nexpected: [x]\noracle: o\nbogus: 1\n---\nq?\n", "field bogus not found"},
		{"no query", "---\nid: a\nexpected: [x]\noracle: o\n---\nq?\n", "query or a files list"},
		{"no expected", "---\nid: a\nquery: q\noracle: o\n---\nq?\n", "at least one fact"},
		{"bad regex", "---\nid: a\nquery: q\nexpected: ['(']\noracle: o\n---\nq?\n", "expected:"},
		{"no question", "---\nid: a\nquery: q\nexpected: [x]\noracle: o\n---\n", "the question"},
		{"bad split", "---\nid: a\nsplit: dev\nquery: q\nexpected: [x]\noracle: o\n---\nq?\n", "train or test"},
	} {
		_, err := parseCase([]byte(tc.raw))
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, tc.want)
		}
	}
}

func TestParseCaseDefaults(t *testing.T) {
	c, err := parseCase([]byte("---\nid: a\nquery: q\nexpected: [x]\noracle: o\n---\nq?\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Split != splitTrain || c.Repo != defaultRepo || c.MaxFiles != defaultMaxFiles ||
		c.MaxAnswerChars != defaultMaxAnswerChars {
		t.Errorf("defaults not applied: %+v", c)
	}
	if c.Question != "q?" {
		t.Errorf("question = %q", c.Question)
	}
}

func TestBuildMessageNumbersLines(t *testing.T) {
	corpus := Corpus{Files: []CorpusFile{{Path: "a.go", Lines: 2, lines: []string{"one", "two"}}}}
	msg := buildMessage(corpus, "why?")
	for _, want := range []string{`<file path="a.go" lines="2">`, "1: one\n2: two\n</file>", "Question: why?"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}

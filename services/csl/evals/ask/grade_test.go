package main

import "testing"

func testCorpus() Corpus {
	return Corpus{Files: []CorpusFile{{
		Path:  "pkg/thing.go",
		Lines: 4,
		lines: []string{
			"package pkg",
			"",
			"func validMode(mode string) error {",
			"	return nil",
		},
	}}}
}

func TestCheckLineClaims(t *testing.T) {
	corpus := testCorpus()
	for _, tc := range []struct {
		name          string
		answer        string
		claims, wrong int
	}{
		{"right path claim", "- validMode (pkg/thing.go:3) accepts hard", 1, 0},
		{"wrong path claim", "- validMode (pkg/thing.go:1) accepts hard", 1, 1},
		{"line word form", "- validMode is defined on line 3", 1, 0},
		{"line word wrong", "- validMode is defined on line 4", 1, 1},
		{"no identifier", "- see line 3", 0, 0},
		{"past the end", "- validMode (pkg/thing.go:40)", 1, 1},
		{"clock time ignored", "- validMode ran at 10:30", 0, 0},
		{"two claims", "- validMode (thing.go:3) and nil (thing.go:4)", 2, 0},
		{"range covers the line", "- the validMode body (pkg/thing.go:1-3)", 1, 0},
		{"range misses", "- the validMode body (pkg/thing.go:1-2)", 1, 1},
		{"range under the header", "- the validMode body (pkg/thing.go:4-5)", 1, 0},
		{"lines word range", "- validMode spans lines 3-4", 1, 0},
		{"en dash range", "- validMode spans lines 1\u20133", 1, 0},
		{"en dash path range", "- validMode (pkg/thing.go:1\u20133)", 1, 0},
		{"case folded", "- VALIDMODE (thing.go:3)", 1, 0},
		{"prose token on the line", "- returns nil for the default mode (thing.go:4)", 1, 0},
	} {
		claims, wrong, _ := checkLineClaims(corpus, tc.answer)
		if claims != tc.claims || wrong != tc.wrong {
			t.Errorf(
				"%s: claims=%d wrong=%d, want %d/%d",
				tc.name,
				claims,
				wrong,
				tc.claims,
				tc.wrong,
			)
		}
	}
}

func TestGradePassNeedsEverything(t *testing.T) {
	c := Case{
		ID: "x", Expected: []string{`\bhard\b`, `\bsoft\b`}, Forbidden: []string{`\bmedium\b`},
		MaxAnswerChars: 40, Oracle: "o", Question: "q", Query: "q",
	}
	if err := c.validate(); err != nil {
		t.Fatal(err)
	}
	corpus := testCorpus()
	for _, tc := range []struct {
		name   string
		answer string
		found  int
		pass   bool
	}{
		{"all facts", "hard and soft", 2, true},
		{"one fact", "hard only", 1, false},
		{"forbidden", "hard, soft, medium", 2, false},
		{"over budget", "hard and soft " + string(make([]byte, 40)), 2, false},
	} {
		g := grade(c, corpus, tc.answer)
		if g.Found != tc.found || g.Pass != tc.pass {
			t.Errorf(
				"%s: found=%d pass=%v, want %d/%v",
				tc.name,
				g.Found,
				g.Pass,
				tc.found,
				tc.pass,
			)
		}
	}
}

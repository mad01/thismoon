package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadFixture(t *testing.T, name string) Transcript {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	tr, err := parseStream(f)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

func TestParseStreamSuccess(t *testing.T) {
	tr := loadFixture(t, "stream-success.jsonl")
	if !tr.HasResult || tr.Subtype != "success" || tr.IsError {
		t.Fatalf("result = %v %q %v", tr.HasResult, tr.Subtype, tr.IsError)
	}
	if tr.SessionID != "11111111-2222-3333-4444-555555555555" || tr.Model != "claude-fable-5-1" ||
		tr.ClaudeVersion != "2.1.310" {
		t.Errorf("init = %q %q %q", tr.SessionID, tr.Model, tr.ClaudeVersion)
	}
	if tr.NumTurns != 4 || tr.CostUSD != 0.1234 || tr.DurationMS != 4200 {
		t.Errorf("numbers = %d %g %d", tr.NumTurns, tr.CostUSD, tr.DurationMS)
	}
	if got := tr.Usage.Total(); got != 52310 {
		t.Errorf("tokens = %d, want 52310", got)
	}
	if tr.PermissionDenials != 1 || tr.BadLines != 0 {
		t.Errorf("denials %d, bad lines %d", tr.PermissionDenials, tr.BadLines)
	}
	names := []string{}
	for _, c := range tr.ToolCalls {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "mcp__csl__csl_search,Read,Bash" {
		t.Errorf("tool calls = %v", names)
	}
	// Keys come back sorted and "&&" unescaped, whatever the model wrote.
	if want := `{"file_path":"/src/thismoon/kit/gitloc/gitloc.go","limit":20,"offset":115}`; tr.ToolCalls[1].Input != want {
		t.Errorf("read input = %s, want %s", tr.ToolCalls[1].Input, want)
	}
	if !strings.Contains(tr.ToolCalls[2].Input, "&& git log") {
		t.Errorf("bash input = %s, want && kept", tr.ToolCalls[2].Input)
	}
}

func TestParseStreamTruncated(t *testing.T) {
	tr := loadFixture(t, "stream-timeout.jsonl")
	if tr.HasResult {
		t.Error("a run without a result event reported one")
	}
	if len(tr.ToolCalls) != 2 || tr.BadLines != 1 {
		t.Errorf("calls %d, bad lines %d; want 2 and 1", len(tr.ToolCalls), tr.BadLines)
	}
}

// fakeJudge answers every rubric the same way and records what it saw.
type fakeJudge struct {
	verdict Verdict
	err     error
	rubric  string
}

func (f *fakeJudge) Judge(_ context.Context, rubric, _, _ string) (Verdict, error) {
	f.rubric = rubric
	return f.verdict, f.err
}

func grader(t *testing.T, g Grader, kind string) Grader {
	t.Helper()
	if err := compileGrader(&g, WorkspaceSpec{Kind: kind}); err != nil {
		t.Fatal(err)
	}
	return g
}

func TestEvaluate(t *testing.T) {
	ok := loadFixture(t, "stream-success.jsonl")
	timedOut := loadFixture(t, "stream-timeout.jsonl")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("buy milk\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	conventional := `^(feat|fix|docs)(\([a-z/-]+\))?: \S`
	tests := []struct {
		name    string
		g       Grader
		kind    string
		tr      Transcript
		commits []string
		want    bool
	}{
		{"regex hit", Grader{Type: gradeRegex, Pattern: `gitloc\.go:121`}, "repo", ok, nil, true},
		{"regex miss", Grader{Type: gradeRegex, Pattern: `finder\.go`}, "repo", ok, nil, false},
		{"file exists", Grader{Type: gradeFile, Path: "notes.txt"}, "tmpdir", ok, nil, true},
		{
			"file content",
			Grader{Type: gradeFile, Path: "notes.txt", Pattern: "milk"},
			"tmpdir", ok, nil, true,
		},
		{
			"file content miss",
			Grader{Type: gradeFile, Path: "notes.txt", Pattern: "bread"},
			"tmpdir", ok, nil, false,
		},
		{"file missing", Grader{Type: gradeFile, Path: "nope.txt"}, "tmpdir", ok, nil, false},
		{
			"commit conventional",
			Grader{Type: gradeCommit, Pattern: conventional},
			"worktree", ok,
			[]string{"docs(kit): add testing note"},
			true,
		},
		{
			"commit one bad subject",
			Grader{Type: gradeCommit, Pattern: conventional},
			"worktree", ok,
			[]string{"docs: fine", "Update README"},
			false,
		},
		{
			"commit none",
			Grader{Type: gradeCommit, Pattern: conventional},
			"worktree", ok, nil, false,
		},
		{
			"tool used with input",
			Grader{Type: gradeToolUsed, Tool: "^mcp__csl__csl_search$", Input: "sym:"},
			"repo", ok, nil, true,
		},
		{
			"tool used wrong input",
			Grader{Type: gradeToolUsed, Tool: "^mcp__csl__csl_search$", Input: "regex:"},
			"repo", ok, nil, false,
		},
		{
			"ranged read counts",
			Grader{Type: gradeToolUsed, Tool: "^Read$", Input: `"(offset|limit)"`},
			"repo", ok, nil, true,
		},
		{
			"no unranged read passes",
			Grader{
				Type: gradeToolNotUsed, Tool: "^Read$", Input: "gitloc", InputWithout: `"(offset|limit)"`,
			},
			"repo", ok, nil, true,
		},
		{
			"unranged read fails",
			Grader{
				Type: gradeToolNotUsed, Tool: "^Read$", Input: "config", InputWithout: `"(offset|limit)"`,
			},
			"repo", timedOut, nil, false,
		},
		{
			"push not used",
			Grader{Type: gradeToolNotUsed, Tool: "^Bash$", Input: "git push"},
			"repo", ok, nil, true,
		},
		{
			"git used",
			Grader{Type: gradeToolNotUsed, Tool: "^Bash$", Input: `\bgit\b`},
			"repo", ok, nil, false,
		},
		{
			"search before read",
			Grader{Type: gradeToolBefore, Tool: "csl_search", Before: "^Read$"},
			"repo", ok, nil, true,
		},
		{
			"outline after read",
			Grader{Type: gradeToolBefore, Tool: "csl_outline", Before: "^Read$"},
			"repo", timedOut, nil, false,
		},
		{
			"first tool never called",
			Grader{Type: gradeToolBefore, Tool: "csl_outline", Before: "^Read$"},
			"repo", ok, nil, false,
		},
		{"turns within", Grader{Type: gradeMaxTurns, Max: 4}, "repo", ok, nil, true},
		{"turns over", Grader{Type: gradeMaxTurns, Max: 3}, "repo", ok, nil, false},
		{"cost within", Grader{Type: gradeMaxCostUSD, Max: 0.5}, "repo", ok, nil, true},
		{"cost over", Grader{Type: gradeMaxCostUSD, Max: 0.1}, "repo", ok, nil, false},
		{"tool calls within", Grader{Type: gradeMaxToolCalls, Max: 3}, "repo", ok, nil, true},
		{"tool calls over", Grader{Type: gradeMaxToolCalls, Max: 2}, "repo", ok, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := grader(t, tt.g, tt.kind)
			ev := Evidence{Transcript: tt.tr, Dir: dir, Commits: tt.commits}
			res, _ := evaluate(context.Background(), g, ev, nil)
			if res.Pass != tt.want {
				t.Errorf("pass = %v, want %v (detail %q)", res.Pass, tt.want, res.Detail)
			}
			if !res.Pass && res.Detail == "" {
				t.Error("a failed grader carries no detail")
			}
		})
	}
}

func TestEvaluateLLM(t *testing.T) {
	ok := loadFixture(t, "stream-success.jsonl")
	g := grader(t, Grader{Type: gradeLLM, Rubric: "names the file"}, "repo")
	ev := Evidence{Transcript: ok}

	j := &fakeJudge{verdict: Verdict{Pass: true, Reason: "it does", CostUSD: 0.002}}
	res, cost := evaluate(context.Background(), g, ev, j)
	if !res.Pass || cost != 0.002 || j.rubric != "names the file" {
		t.Errorf("pass %v cost %g rubric %q", res.Pass, cost, j.rubric)
	}

	failing := &fakeJudge{err: errors.New("boom")}
	if res, _ := evaluate(context.Background(), g, ev, failing); res.Pass {
		t.Error("a judge error passed the grader")
	}
	if res, _ := evaluate(context.Background(), g, ev, nil); res.Pass {
		t.Error("no judge passed the grader")
	}
}

func TestGradeRunWeights(t *testing.T) {
	ok := loadFixture(t, "stream-success.jsonl")
	c := Case{Graders: []Grader{
		grader(t, Grader{Type: gradeRegex, Pattern: "gitloc", Weight: 3}, "repo"),
		grader(t, Grader{Type: gradeMaxTurns, Max: 1}, "repo"),
	}}
	g := gradeRun(context.Background(), c, Evidence{Transcript: ok}, nil)
	if g.Score != 0.75 || g.Pass {
		t.Errorf("score %g pass %v; want 0.75 and false", g.Score, g.Pass)
	}
	if len(g.Results) != 2 || !g.Results[0].Pass || g.Results[1].Pass {
		t.Errorf("results = %+v", g.Results)
	}
}

func TestBudgetDetail(t *testing.T) {
	pass, detail := atMost(1.3176865000000002, 0.6, "USD")
	if pass || detail != "1.3177 USD, max 0.6" {
		t.Errorf("atMost = %v %q", pass, detail)
	}
	if pass, detail := atMost(5, 5, "turns"); !pass || detail != "5 turns, max 5" {
		t.Errorf("at the limit = %v %q", pass, detail)
	}
}

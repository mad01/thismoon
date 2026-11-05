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
	diff    string
}

func (f *fakeJudge) Judge(_ context.Context, in JudgeInput) (Verdict, error) {
	f.rubric, f.diff = in.Rubric, in.Diff
	return f.verdict, f.err
}

// num points at x, for the pointer fields of a grader literal.
func num[T int | float64](x T) *T { return &x }

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
			Grader{Type: gradeToolUsed, Tool: "^mcp__csl__csl_search$", Input: regexList{"sym:"}},
			"repo", ok, nil, true,
		},
		{
			"tool used wrong input",
			Grader{Type: gradeToolUsed, Tool: "^mcp__csl__csl_search$", Input: regexList{"regex:"}},
			"repo", ok, nil, false,
		},
		{
			"ranged read counts",
			Grader{Type: gradeToolUsed, Tool: "^Read$", Input: regexList{`"(offset|limit)"`}},
			"repo", ok, nil, true,
		},
		{
			"no unranged read passes",
			Grader{
				Type: gradeToolNotUsed, Tool: "^Read$", Input: regexList{"gitloc"}, InputWithout: `"(offset|limit)"`,
			},
			"repo", ok, nil, true,
		},
		{
			"unranged read fails",
			Grader{
				Type: gradeToolNotUsed, Tool: "^Read$", Input: regexList{"config"}, InputWithout: `"(offset|limit)"`,
			},
			"repo", timedOut, nil, false,
		},
		{
			"push not used",
			Grader{Type: gradeToolNotUsed, Tool: "^Bash$", Input: regexList{"git push"}},
			"repo", ok, nil, true,
		},
		{
			"git used",
			Grader{Type: gradeToolNotUsed, Tool: "^Bash$", Input: regexList{`\bgit\b`}},
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
		{"turns within", Grader{Type: gradeMaxTurns, Max: num(4.0)}, "repo", ok, nil, true},
		{"turns over", Grader{Type: gradeMaxTurns, Max: num(3.0)}, "repo", ok, nil, false},
		{"cost within", Grader{Type: gradeMaxCostUSD, Max: num(0.5)}, "repo", ok, nil, true},
		{"cost over", Grader{Type: gradeMaxCostUSD, Max: num(0.1)}, "repo", ok, nil, false},
		{
			"tool calls within",
			Grader{Type: gradeMaxToolCalls, Max: num(3.0)},
			"repo",
			ok,
			nil,
			true,
		},
		{"tool calls over", Grader{Type: gradeMaxToolCalls, Max: num(2.0)}, "repo", ok, nil, false},
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
		grader(t, Grader{Type: gradeMaxTurns, Max: num(1.0)}, "repo"),
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

func TestAgentCallsAreTagged(t *testing.T) {
	tr := loadFixture(t, "stream-agents.jsonl")
	if got := len(tr.Calls(false)); got != 1 || tr.Calls(false)[0].Name != "Task" {
		t.Errorf("main calls = %+v, want the Task call alone", tr.Calls(false))
	}
	if got := len(tr.Calls(true)); got != 3 {
		t.Errorf("all calls = %d, want 3", got)
	}
	if c := tr.ToolCounts(originAgent); c["Grep"] != 1 || c["Read"] != 1 {
		t.Errorf("agent counts = %v", c)
	}

	tests := []struct {
		name string
		g    Grader
		want bool
	}{
		{"grep ignored by default", Grader{Type: gradeToolNotUsed, Tool: "^Grep$"}, true},
		{
			"grep seen with agents",
			Grader{Type: gradeToolNotUsed, Tool: "^Grep$", IncludeAgents: true},
			false,
		},
		{"main call budget", Grader{Type: gradeMaxToolCalls, Max: num(1.0)}, true},
		{
			"budget with agents",
			Grader{Type: gradeMaxToolCalls, Max: num(1.0), IncludeAgents: true},
			false,
		},
		{
			"order sees agents when asked",
			Grader{Type: gradeToolBefore, Tool: "^Task$", Before: "^Read$", IncludeAgents: true},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, _ := evaluate(
				context.Background(),
				grader(t, tt.g, "repo"),
				Evidence{Transcript: tr},
				nil,
			)
			if res.Pass != tt.want {
				t.Errorf("pass = %v, want %v (%s)", res.Pass, tt.want, res.Detail)
			}
		})
	}
}

func TestTranscriptProblems(t *testing.T) {
	if err := loadFixture(t, "stream-success.jsonl").Problems(); err != nil {
		t.Errorf("clean transcript: %v", err)
	}
	err := loadFixture(t, "stream-agents.jsonl").Problems()
	if err == nil || !strings.Contains(err.Error(), "is_error (error_during_execution)") {
		t.Errorf("API error result: %v", err)
	}
	err = loadFixture(t, "stream-timeout.jsonl").Problems()
	if err == nil || !strings.Contains(err.Error(), "1 stream lines did not parse") {
		t.Errorf("truncated stream: %v", err)
	}
}

func TestInputListMatchesAll(t *testing.T) {
	ok := loadFixture(t, "stream-success.jsonl")
	ranged := regexList{`"file_path":"[^"]*kit/gitloc/gitloc\.go"`, `"(offset|limit)"`}
	g := grader(t, Grader{Type: gradeToolUsed, Tool: "^Read$", Input: ranged}, "repo")
	if res, _ := evaluate(context.Background(), g, Evidence{Transcript: ok}, nil); !res.Pass {
		t.Errorf("ranged read of the file: %s", res.Detail)
	}
	other := regexList{`"file_path":"[^"]*finder\.go"`, `"(offset|limit)"`}
	g = grader(t, Grader{Type: gradeToolUsed, Tool: "^Read$", Input: other}, "repo")
	if res, _ := evaluate(context.Background(), g, Evidence{Transcript: ok}, nil); res.Pass {
		t.Error("a ranged read of another file matched")
	}
}

const policyNote = `---
title: Policy
owner: data-governance
tags:
    - policy
    - data
type: note
status: validated
---
See [[ledger-svc]] and [[notes/pg-failover#Verify|failover]].
`

// vaultEvidence is a brain workspace's evidence over a small vault where
// the run added inbox/new.md and changed notes/policy.md.
func vaultEvidence(t *testing.T) Evidence {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"notes/policy.md":      policyNote,
		"notes/ledger-svc.md":  "---\ntitle: ledger-svc\n---\nport 9090\n",
		"notes/pg-failover.md": "---\ntitle: failover\n---\nrunbook\n",
		"inbox/new.md":         "---\ntitle: New\ntype: source\n---\nlinks [[missing-note]] and `[[in-code]]`\n",
	})
	files, err := listFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	return Evidence{Dir: dir, Files: files, Changes: []Change{
		{Status: "A", Path: "inbox/new.md"},
		{Status: "M", Path: "notes/policy.md"},
	}}
}

// gradeVault runs each grader against ev in a brain workspace.
func gradeVault(t *testing.T, ev Evidence, tests []vaultTest) {
	t.Helper()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, _ := evaluate(context.Background(), grader(t, tc.g, workspaceBrain), ev, nil)
			if res.Pass != tc.want {
				t.Errorf("pass = %v (%s), want %v", res.Pass, res.Detail, tc.want)
			}
			if !res.Pass && res.Detail == "" {
				t.Error("a failed grader carries no detail")
			}
		})
	}
}

type vaultTest struct {
	name string
	g    Grader
	want bool
}

func TestRegexAbsent(t *testing.T) {
	ok := loadFixture(t, "stream-success.jsonl")
	gradeVault(t, Evidence{Transcript: ok}, []vaultTest{
		{"absent passes", Grader{Type: gradeRegex, Pattern: `finder\.go`, Absent: true}, true},
		{"absent fails", Grader{Type: gradeRegex, Pattern: `gitloc`, Absent: true}, false},
	})
}

func TestFileGlobGraders(t *testing.T) {
	gradeVault(t, vaultEvidence(t), []vaultTest{
		{"exists", Grader{Type: gradeFile, Path: "notes/policy.md"}, true},
		{"exists glob", Grader{Type: gradeFile, Path: "inbox/*.md"}, true},
		{"exists missing", Grader{Type: gradeFile, Path: "outputs/*.md"}, false},
		{"absent", Grader{Type: gradeFile, Path: "inbox/old.md", Absent: true}, true},
		{"absent present", Grader{Type: gradeFile, Path: "inbox/new.md", Absent: true}, false},
		{"content any file", Grader{Type: gradeFile, Path: "notes/*.md", Pattern: `9090`}, true},
		{"content no match", Grader{Type: gradeFile, Path: "notes/*.md", Pattern: `8080`}, false},
		{"content no file", Grader{Type: gradeFile, Path: "outputs/*.md", Pattern: `.`}, false},
		{
			"content new only",
			Grader{Type: gradeFile, Path: "**", Pattern: `9090`, NewOnly: true},
			false,
		},
		{
			"content absent everywhere",
			Grader{Type: gradeFile, Path: "**", Pattern: `secret`, Absent: true},
			true,
		},
		{
			"content absent found",
			Grader{Type: gradeFile, Path: "**", Pattern: `9090`, Absent: true},
			false,
		},
	})
}

func TestFrontmatterGrader(t *testing.T) {
	fm := func(p, key, pattern string, newOnly bool) Grader {
		return Grader{Type: gradeFrontmatter, Path: p, Key: key, Pattern: pattern, NewOnly: newOnly}
	}
	gradeVault(t, vaultEvidence(t), []vaultTest{
		{"extra key kept", fm("notes/policy.md", "owner", `^data-governance$`, false), true},
		{"list item", fm("notes/policy.md", "tags", `(?m)^data$`, false), true},
		{"list item missing", fm("notes/policy.md", "tags", `(?m)^gdpr$`, false), false},
		{"key missing", fm("notes/policy.md", "classification", `.`, false), false},
		{"new only", fm("**", "type", `^source$`, true), true},
		{"new only excludes old", fm("notes/*.md", "title", `.`, true), false},
	})
}

func TestNewNotesAndUnchangedGraders(t *testing.T) {
	gradeVault(t, vaultEvidence(t), []vaultTest{
		{
			"one new in inbox",
			Grader{Type: gradeNewNotes, Dir: "inbox", Min: num(1), Max: num(1.0)},
			true,
		},
		{"none in notes", Grader{Type: gradeNewNotes, Dir: "notes", Max: num(0.0)}, true},
		{"too many", Grader{Type: gradeNewNotes, Max: num(0.0)}, false},
		{"too few", Grader{Type: gradeNewNotes, Dir: "outputs", Min: num(1)}, false},
		{
			"content matches",
			Grader{Type: gradeNewNotes, Min: num(1), ContentRegex: `missing-note`},
			true,
		},
		{
			"content does not match",
			Grader{Type: gradeNewNotes, Min: num(1), ContentRegex: `nope`},
			false,
		},
		{
			"unchanged except covers",
			Grader{
				Type:  gradeVaultUnchangedExcept,
				Paths: []string{"inbox/*.md", "notes/policy.md"},
			},
			true,
		},
		{
			"unchanged except misses",
			Grader{Type: gradeVaultUnchangedExcept, Paths: []string{"inbox/*.md"}},
			false,
		},
		{"unchanged", Grader{Type: gradeVaultUnchangedExcept}, false},
	})
}

func TestWikilinksGrader(t *testing.T) {
	gradeVault(t, vaultEvidence(t), []vaultTest{
		{
			"alias, heading and path forms resolve",
			Grader{Type: gradeWikilinks, Path: "notes/policy.md", Min: num(2)},
			true,
		},
		{
			"too few links",
			Grader{Type: gradeWikilinks, Path: "notes/policy.md", Min: num(3)},
			false,
		},
		{"broken link", Grader{Type: gradeWikilinks, Path: "inbox/*.md"}, false},
		{"no file", Grader{Type: gradeWikilinks, Path: "outputs/*.md"}, false},
	})
}

// TestWikilinksTwoVault: a private note's link resolves in its own vault,
// then in the shared one, and a shared note's link never in the private.
func TestWikilinksTwoVault(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{
		"private/notes/local.md":    "[[team-note]], [[notes/mine]] and [[mine|me]]\n",
		"private/notes/mine.md":     "mine\n",
		"shared/notes/team-note.md": "[[notes/team-note]]\n",
		"shared/notes/leak.md":      "[[mine]]\n",
	})
	files, err := listFiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	ev := Evidence{Dir: dir, Files: files, TwoVault: true}
	wl := func(p string, minLinks int) Grader {
		return Grader{Type: gradeWikilinks, Path: p, Min: num(minLinks)}
	}
	gradeVault(t, ev, []vaultTest{
		{"private reaches shared", wl("private/notes/local.md", 2), true},
		{"vault-relative path link", wl("shared/notes/team-note.md", 1), true},
		{"shared never reaches private", wl("shared/notes/leak.md", 0), false},
	})
}

// TestFileGlobOutsideBrain: a glob walks a non-brain workspace, and a
// literal path does not need the walk.
func TestFileGlobOutsideBrain(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"docs/a.md": "alpha\n", ".git/HEAD": "ref\n"})
	for _, tc := range []struct {
		g    Grader
		want bool
	}{
		{Grader{Type: gradeFile, Path: "docs/*.md", Pattern: "alpha"}, true},
		{Grader{Type: gradeFile, Path: "**/HEAD"}, false},
		{Grader{Type: gradeFile, Path: "docs/a.md"}, true},
	} {
		res, _ := evaluate(
			context.Background(),
			grader(t, tc.g, workspaceTmpdir),
			Evidence{Dir: dir},
			nil,
		)
		if res.Pass != tc.want {
			t.Errorf("%s: pass = %v (%s), want %v", tc.g.Name(), res.Pass, res.Detail, tc.want)
		}
	}
}

func TestJudgeSeesVaultDiff(t *testing.T) {
	g := grader(t, Grader{Type: gradeLLM, Rubric: "records the learning"}, workspaceBrain)
	j := &fakeJudge{verdict: Verdict{Pass: true}}
	ev := Evidence{Diff: "+the rotation job must update secrets first"}
	if res, _ := evaluate(context.Background(), g, ev, j); !res.Pass || j.diff != ev.Diff {
		t.Errorf("pass %v, judge saw diff %q", res.Pass, j.diff)
	}
	prompt := judgePrompt(JudgeInput{Rubric: "r", Prompt: "p", Answer: "a", Diff: "d"})
	if !strings.HasSuffix(
		prompt,
		"Answer:\na\n\nVault diff (what the agent changed in the knowledge base):\nd",
	) {
		t.Errorf("judge prompt = %q", prompt)
	}
	if strings.Contains(judgePrompt(JudgeInput{Rubric: "r"}), "Vault diff") {
		t.Error("a run without a vault diff should not mention one")
	}
}

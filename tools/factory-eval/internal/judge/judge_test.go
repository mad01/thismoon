package judge

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/checkout"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
	"github.com/mad01/thismoon/tools/factory-eval/internal/outline"
)

const (
	goodClaude = "# svc, a test service\n\n## Module layout\n\n## Gotchas\n"
	goodReadme = "# svc\n\n## Install\n"
	badClaude  = "# svc\n\n## Gotchas\n\n## Module layout\n"
)

var testOutline = cases.Outline{
	ClaudeMD: outline.Spec{Title: outline.TitleNameComma, Sections: []outline.Section{
		{Heading: "Module layout", Required: true}, {Heading: "Gotchas", Required: true},
	}},
	Readme: outline.Spec{Title: outline.TitleName, Sections: []outline.Section{
		{Heading: "Install", Required: true},
	}},
}

var svcCase = cases.Case{
	Component: "svc", Branch: "eval/svc",
	AllowedPaths: []string{"svc/CLAUDE.md", "svc/README.md"},
}

// fakeCmd answers humanizer with canned JSON per file and make with makeErr.
type fakeCmd struct {
	findings map[string]string
	makeErr  error
	calls    []string
}

func (f *fakeCmd) Run(_ context.Context, _, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	if name == "make" {
		return nil, f.makeErr
	}
	if out, ok := f.findings[args[len(args)-1]]; ok {
		return []byte(out), nil
	}
	return []byte(`{"findings":[]}`), nil
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	full := append([]string{
		"-c", "user.name=test", "-c", "user.email=test@example.com",
		"-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main",
	}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// checkoutWithBranch pins a repo holding badClaude and goodReadme, and when
// edits is non-nil commits them on the case branch.
func checkoutWithBranch(t *testing.T, edits map[string]string) *checkout.Checkout {
	t.Helper()
	src := t.TempDir()
	runGit(t, src, "init", "--quiet")
	write(t, src, map[string]string{"svc/CLAUDE.md": badClaude, "svc/README.md": goodReadme})
	runGit(t, src, "add", ".")
	runGit(t, src, "commit", "--quiet", "-m", "init")
	co, err := checkout.Create(context.Background(), src, runGit(t, src, "rev-parse", "HEAD"),
		t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if edits != nil {
		runGit(t, co.Repo, "switch", "--quiet", "-c", svcCase.Branch)
		write(t, co.Repo, edits)
		runGit(t, co.Repo, "add", ".")
		runGit(t, co.Repo, "commit", "--quiet", "-m", "sweep")
	}
	return co
}

func checkByName(t *testing.T, checks []Check, name string) Check {
	t.Helper()
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no %s check in %+v", name, checks)
	return Check{}
}

func TestMissingBranchFailsAllFiveChecks(t *testing.T) {
	co := checkoutWithBranch(t, nil)
	d := Deterministic{Outline: testOutline, Cmd: &fakeCmd{}}
	checks := d.Run(context.Background(), co, svcCase)
	if len(checks) != 5 || AllPass(checks) {
		t.Fatalf("checks = %+v, want five failures", checks)
	}
	for _, c := range checks {
		if c.Pass || !strings.Contains(c.Detail, "missing") {
			t.Errorf("%s = %+v, want a failure naming the missing branch", c.Name, c)
		}
	}
}

func TestCleanSweepPassesEveryCheck(t *testing.T) {
	co := checkoutWithBranch(t, map[string]string{"svc/CLAUDE.md": goodClaude})
	cmd := &fakeCmd{findings: map[string]string{
		"svc/CLAUDE.md": `{"findings":[{"rule_id":"Humanizer.TitleCaseHeadings"}]}`,
	}}
	d := Deterministic{
		Outline: testOutline, Cmd: cmd,
		Humanizer: cases.Humanizer{
			MinSeverity: "warning", IgnoreRules: []string{"Humanizer.TitleCaseHeadings"},
		},
	}
	checks := d.Run(context.Background(), co, svcCase)
	if !AllPass(checks) {
		t.Errorf("checks = %+v, want all five to pass (the finding is an ignored rule)", checks)
	}
	if !strings.Contains(strings.Join(cmd.calls, "\n"), "make -C svc build test") {
		t.Errorf("calls = %v, want a make build test of the component", cmd.calls)
	}
}

func TestEachCheckReportsItsOwnFailure(t *testing.T) {
	co := checkoutWithBranch(t, map[string]string{"svc/README.md": "# svc\n\n## Install\n\nnew\n"})
	cmd := &fakeCmd{
		findings: map[string]string{
			"svc/README.md": `{"findings":[{"rule_id":"Humanizer.EmDashOveruse"},` +
				`{"rule_id":"Humanizer.EmDashOveruse"}]}`,
		},
		makeErr: errors.New("make: exit 2\n--- FAIL: TestX"),
	}
	d := Deterministic{
		Outline:   testOutline,
		Cmd:       cmd,
		Humanizer: cases.Humanizer{MinSeverity: "warning"},
	}
	checks := d.Run(context.Background(), co, svcCase)
	tests := []struct{ name, detail string }{
		{CheckOutline, "svc/CLAUDE.md: title: svc, order:"},
		{CheckHumanizer, "svc/README.md: EmDashOveruse=2"},
		{CheckBuildTest, "--- FAIL: TestX"},
	}
	for _, tt := range tests {
		c := checkByName(t, checks, tt.name)
		if c.Pass || !strings.Contains(c.Detail, tt.detail) {
			t.Errorf("%s = %+v, want a failure containing %q", tt.name, c, tt.detail)
		}
	}
	for _, name := range []string{CheckBranch, CheckAllowedPaths} {
		if c := checkByName(t, checks, name); !c.Pass {
			t.Errorf("%s = %+v, want pass", name, c)
		}
	}
}

// TestEditsOutsideTheCaseRunNoTool proves an arm that edited a Makefile
// never gets it run: humanizer and make are skipped, not called.
func TestEditsOutsideTheCaseRunNoTool(t *testing.T) {
	co := checkoutWithBranch(t, map[string]string{
		"svc/CLAUDE.md": goodClaude, "svc/Makefile": "build:\n\ttouch /tmp/pwned\n",
		"svc/docs/extra.md": "# extra\n",
	})
	cmd := &fakeCmd{}
	d := Deterministic{
		Outline:   testOutline,
		Cmd:       cmd,
		Humanizer: cases.Humanizer{MinSeverity: "warning"},
	}
	checks := d.Run(context.Background(), co, svcCase)
	if c := checkByName(t, checks, CheckAllowedPaths); c.Pass ||
		!strings.Contains(c.Detail, "svc/Makefile, svc/docs/extra.md") {
		t.Errorf("allowed_paths = %+v, want a failure naming both paths", c)
	}
	for _, name := range []string{CheckHumanizer, CheckBuildTest} {
		if c := checkByName(t, checks, name); c.Pass || !strings.HasPrefix(c.Detail, "skipped") {
			t.Errorf("%s = %+v, want a skipped failure", name, c)
		}
	}
	if len(cmd.calls) != 0 {
		t.Errorf("tools ran on a branch that edits outside the case: %v", cmd.calls)
	}
	if c := checkByName(t, checks, CheckOutline); !c.Pass {
		t.Errorf("outline = %+v, want it still checked and passing", c)
	}
}

// fakeRunner returns stdout for every call and records the invocation.
type fakeRunner struct {
	stdout string
	got    claude.Invocation
}

func (f *fakeRunner) Run(_ context.Context, inv claude.Invocation) (claude.Output, error) {
	f.got = inv
	return claude.Output{Stdout: []byte(f.stdout)}, nil
}

var testJudge = cases.Judge{
	Model: "haiku", BudgetUSD: 0.5, Rubric: "Score clarity.",
	DoNotFail: []string{"short sections"}, GlossaryNotes: "Exception: Agents are fine.",
}

func TestScoreParsesTheStructuredVerdict(t *testing.T) {
	r := &fakeRunner{stdout: `{"type":"result","subtype":"success","total_cost_usd":0.02,` +
		`"structured_output":{"score":8,"reasons":["one long sentence"],` +
		`"do_not_fail_applied":["short sections"]}}`}
	s := Scorer{Runner: r, Judge: testJudge, Dir: "/empty"}
	v, err := s.Score(context.Background(), Request{
		Component: "svc",
		Files:     []FilePair{{Path: "svc/README.md", Before: "old text\n", After: "new text\n"}},
		Glossary: []Doc{
			{Path: "CONTEXT.md", Content: "**Component**:\nA releasable unit.\n"},
			{Path: "svc/CONTEXT.md", Content: "**Rule ignore**:\nPer repo.\n"},
		},
	})
	if err != nil {
		t.Fatalf("Score() error = %v", err)
	}
	if v.Score != 8 || v.CostUSD != 0.02 || len(v.DoNotFailApplied) != 1 {
		t.Errorf("Score() = %+v, want 8 at $0.02 with one do-not-fail item", v)
	}
	if r.got.Dir != "/empty" || !strings.Contains(strings.Join(r.got.Args, " "), "--model haiku") {
		t.Errorf("invocation = %+v, want the judge model in the empty dir", r.got)
	}
	for _, want := range []string{
		"Score clarity.", "- short sections",
		"<glossary path=\"CONTEXT.md\">\n**Component**:\nA releasable unit.\n</glossary>",
		"<glossary path=\"svc/CONTEXT.md\">\n**Rule ignore**:",
		"<glossary_notes>\nException: Agents are fine.\n</glossary_notes>",
		`<file path="svc/README.md" version="before">` + "\nold text\n",
		`<file path="svc/README.md" version="after">` + "\nnew text\n",
	} {
		if !strings.Contains(r.got.Stdin, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
}

func TestScoreRejectsVerdictsOutOfContract(t *testing.T) {
	for name, stdout := range map[string]string{
		"score too high": `{"type":"result","subtype":"success",` +
			`"structured_output":{"score":11,"reasons":["x"]}}`,
		"no reasons": `{"type":"result","subtype":"success",` +
			`"structured_output":{"score":5,"reasons":[]}}`,
		"budget stop": `{"type":"result","subtype":"error_max_budget_usd","is_error":true,` +
			`"total_cost_usd":0.5,"errors":["budget"]}`,
		"API error under subtype success": `{"type":"result","subtype":"success","is_error":true,` +
			`"api_error_status":529,"result":"API Error: overloaded"}`,
		"not json": "Error: not logged in",
	} {
		s := Scorer{Runner: &fakeRunner{stdout: stdout}, Judge: testJudge}
		v, err := s.Score(context.Background(), Request{Component: "svc"})
		if err == nil {
			t.Errorf("%s: Score() = %+v, nil error; want a contract error", name, v)
		}
		if name == "budget stop" && v.CostUSD != 0.5 {
			t.Errorf("budget stop: CostUSD = %v, want the spend kept", v.CostUSD)
		}
	}
}

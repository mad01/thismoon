package dryrun

import (
	"bytes"
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
)

func TestAgentLeavesBranchesAtThePinAndReportsZeroSpend(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet"},
		{
			"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "--quiet",
			"--allow-empty", "-m", "pin",
		},
	} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).
			CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	var log bytes.Buffer
	a := Agent{Cases: []cases.Case{{Branch: "eval/a"}, {Branch: "eval/b"}}, Out: &log}
	out, err := a.Run(context.Background(), claude.Invocation{
		Dir: dir, Args: []string{"-p", "--allowedTools", "Bash(git add *)"}, Stdin: "prompt",
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	rep, err := claude.ParseReport(out.Stdout)
	if err != nil || !rep.Finished() || rep.TotalCostUSD != 0 {
		t.Errorf("report = %+v, %v; want a finished zero-cost result", rep, err)
	}
	if !strings.Contains(log.String(), "claude -p --allowedTools 'Bash(git add *)'") {
		t.Errorf("log = %q, want the quoted argv", log.String())
	}
	branches, _ := exec.Command("git", "-C", dir, "branch", "--list", "eval/*").Output()
	if got := strings.Fields(string(branches)); len(got) != 2 {
		t.Errorf("branches = %v, want eval/a and eval/b", got)
	}
}

func TestJudgePrintsOnceAndReturnsAContractVerdict(t *testing.T) {
	var log bytes.Buffer
	j := NewJudge(&log)
	for range 3 {
		out, err := j.Run(context.Background(), claude.Invocation{Args: []string{"-p"}})
		if err != nil {
			t.Fatal(err)
		}
		rep, err := claude.ParseReport(out.Stdout)
		if err != nil || !strings.Contains(string(rep.StructuredOutput), `"score":0`) {
			t.Errorf("report = %+v, %v", rep, err)
		}
	}
	if n := strings.Count(log.String(), "the judge would run"); n != 1 {
		t.Errorf("printed the judge argv %d times, want once", n)
	}
}

func TestQuote(t *testing.T) {
	got := Quote([]string{"-p", "", "Bash(git add *)", "it's"})
	if want := `-p '' 'Bash(git add *)' 'it'\''s'`; got != want {
		t.Errorf("Quote() = %s, want %s", got, want)
	}
}

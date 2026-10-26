package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// testGrace keeps the kill tests fast; production uses killGrace.
const testGrace = 200 * time.Millisecond

func fakeClaude(t *testing.T) claudeCmd {
	t.Helper()
	bin, err := filepath.Abs(filepath.Join("testdata", "fake-claude.sh"))
	if err != nil {
		t.Fatal(err)
	}
	return claudeCmd{bin: bin, grace: testGrace}
}

func setStream(t *testing.T, fixture string) {
	t.Helper()
	path, err := filepath.Abs(filepath.Join("testdata", fixture))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_CLAUDE_STREAM", path)
}

// waitGone polls until pid no longer exists, or fails the test.
func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = syscall.Kill(pid, syscall.SIGKILL)
	t.Errorf("pid %d outlived the run", pid)
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	return pid
}

func TestClaudeCmdTimeoutKillsGroup(t *testing.T) {
	setStream(t, "stream-timeout.jsonl")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("FAKE_CLAUDE_PIDFILE", pidFile)
	t.Setenv("FAKE_CLAUDE_IGNORE_TERM", "1")
	t.Setenv("FAKE_CLAUDE_SLEEP", "30")

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	var out strings.Builder
	err := fakeClaude(t).run(ctx, t.TempDir(), invocation{Prompt: "p"}, &out)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want the deadline", err)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Errorf("run took %s; SIGTERM-proof processes were not killed", elapsed)
	}
	if !strings.Contains(out.String(), `"subtype":"init"`) {
		t.Error("transcript written before the timeout was lost")
	}
	waitGone(t, readPID(t, pidFile))
}

func TestClaudeCmdReapsStragglersOnNormalExit(t *testing.T) {
	setStream(t, "stream-success.jsonl")
	pidFile := filepath.Join(t.TempDir(), "child.pid")
	t.Setenv("FAKE_CLAUDE_PIDFILE", pidFile)
	t.Setenv("FAKE_CLAUDE_IGNORE_TERM", "1")

	if err := fakeClaude(t).run(context.Background(), t.TempDir(), invocation{Prompt: "p"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	waitGone(t, readPID(t, pidFile))
}

func testCase(t *testing.T, id string, ws WorkspaceSpec) Case {
	t.Helper()
	return Case{
		ID: id, Workspace: ws, Timeout: 10 * time.Second, MaxTurns: 3, Prompt: "p",
		DisallowedTools: []string{"CaseOnly"},
		Graders:         []Grader{grader(t, Grader{Type: gradeMaxTurns, Max: 10}, ws.Kind)},
	}
}

func testRunner(t *testing.T, roots workspaceRoots) *runner {
	t.Helper()
	return &runner{
		claude: fakeClaude(t), roots: roots, streamDir: t.TempDir(),
		now: time.Now, log: io.Discard,
	}
}

func TestRunCaseRepoWorkspace(t *testing.T) {
	setStream(t, "stream-success.jsonl")
	_, repoDir := newRepo(t, "demo")
	argsFile := filepath.Join(t.TempDir(), "args")
	t.Setenv("FAKE_CLAUDE_ARGS", argsFile)
	r := testRunner(t, workspaceRoots{})
	c := testCase(t, "readonly", WorkspaceSpec{Kind: workspaceRepo, Repo: repoDir})

	row := r.runCase(context.Background(), c, 0)
	if row.Error != "" || !row.Pass || row.SessionID == "" || row.ToolCalls != 3 {
		t.Fatalf("clean run: %+v", row)
	}
	args, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"CaseOnly", "Edit", "Bash(git commit:*)", "mcp__kof__kof_assert"} {
		if !strings.Contains(string(args), want+"\n") {
			t.Errorf("claude was not told to disallow %s", want)
		}
	}

	t.Setenv("FAKE_CLAUDE_TOUCH", "README.md")
	row = r.runCase(context.Background(), c, 1)
	if !strings.Contains(row.Error, "changed during the case") ||
		!strings.Contains(row.Error, "status went from") {
		t.Errorf("a write to the canonical checkout went unreported: %q", row.Error)
	}
}

func TestRunCaseWorktreeGuardsCanonical(t *testing.T) {
	setStream(t, "stream-success.jsonl")
	srcRoot, repoDir := newRepo(t, "demo")
	r := testRunner(t, workspaceRoots{SrcRoot: srcRoot, WorktreeRoot: t.TempDir()})
	c := testCase(t, "edit", WorkspaceSpec{Kind: workspaceWorktree, Repo: "demo"})

	t.Setenv("FAKE_CLAUDE_TOUCH", "README.md")
	if row := r.runCase(context.Background(), c, 0); row.Error != "" {
		t.Errorf("an edit inside the worktree was flagged: %q", row.Error)
	}
	if out := mustGit(t, repoDir, "branch", "--list", "eval/*"); out != "" {
		t.Errorf("eval branch left behind: %q", out)
	}

	t.Setenv("FAKE_CLAUDE_TOUCH", filepath.Join(repoDir, "README.md"))
	if row := r.runCase(context.Background(), c, 1); !strings.Contains(
		row.Error,
		"changed during the case",
	) {
		t.Errorf("a write to the canonical checkout went unreported: %q", row.Error)
	}
}

func TestRunCaseReportsTranscriptProblems(t *testing.T) {
	setStream(t, "stream-agents.jsonl")
	r := testRunner(t, workspaceRoots{TmpRoot: t.TempDir()})
	row := r.runCase(
		context.Background(),
		testCase(t, "scratch", WorkspaceSpec{Kind: workspaceTmpdir}),
		0,
	)
	if !strings.Contains(row.Error, "is_error") {
		t.Errorf("an API error result was not an error: %q", row.Error)
	}
	if row.ToolCalls != 1 || row.AgentToolCalls != 2 || row.AgentTools["Grep"] != 1 {
		t.Errorf(
			"tool counts = %d main, %d agent %v",
			row.ToolCalls,
			row.AgentToolCalls,
			row.AgentTools,
		)
	}
}

func TestRunAllStopsAtMaxTotal(t *testing.T) {
	setStream(t, "stream-success.jsonl") // each run costs $0.1234
	r := testRunner(t, workspaceRoots{TmpRoot: t.TempDir()})
	r.maxTotalUSD = 0.2
	cases := []Case{
		testCase(t, "a", WorkspaceSpec{Kind: workspaceTmpdir}),
		testCase(t, "b", WorkspaceSpec{Kind: workspaceTmpdir}),
		testCase(t, "c", WorkspaceSpec{Kind: workspaceTmpdir}),
	}
	rows, stopped := r.runAll(context.Background(), cases, 1)
	if len(rows) != 2 || !strings.Contains(stopped, "-max-total-usd") {
		t.Errorf("ran %d cases, stopped %q; want 2 and a cap message", len(rows), stopped)
	}

	r.maxTotalUSD = 0
	if rows, stopped := r.runAll(context.Background(), cases, 1); len(rows) != 3 || stopped != "" {
		t.Errorf("no cap: ran %d, stopped %q", len(rows), stopped)
	}
}

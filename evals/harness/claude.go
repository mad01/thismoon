package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// killGrace is how long claude's process group gets after SIGTERM
	// before it is killed outright.
	killGrace = 10 * time.Second
	// judgeTimeout bounds one llm grader call.
	judgeTimeout = 2 * time.Minute
	// judgeMaxBudgetUSD caps one llm grader call; a haiku verdict costs
	// about a cent.
	judgeMaxBudgetUSD = "0.05"
	// scorecardTimeout bounds the optional moondb scorecard fetch.
	scorecardTimeout = 3 * time.Second
	// stderrTail is how much of claude's stderr a failed run keeps.
	stderrTail = 2000
	// groupPoll is how often reapGroup checks whether the group is gone.
	groupPoll = 50 * time.Millisecond
)

// effortLevels are the values `claude --effort` accepts.
var effortLevels = []string{"low", "medium", "high", "xhigh", "max"}

// sessionEnv names the variables a run must not inherit. The session ones
// tie a process to the calling Claude Code session, so dropping them makes
// the run its own session instead of nesting in the caller's (the same
// list as services/keeper-of-facts/internal/recall). The effort and agent
// teams ones would silently change how the run behaves, so a run sets
// effort only through -effort and records it.
var sessionEnv = map[string]bool{
	"CLAUDECODE":                           true,
	"CLAUDE_CODE_ENTRYPOINT":               true,
	"CLAUDE_CODE_SESSION_ID":               true,
	"CLAUDE_CODE_CHILD_SESSION":            true,
	"CLAUDE_CODE_SESSION_ATTENDED":         true,
	"CLAUDE_CODE_MESSAGING_SOCKET":         true,
	"CLAUDE_CODE_MESSAGING_TOKEN":          true,
	"CLAUDE_PROJECT_DIR":                   true,
	"CLAUDE_PID":                           true,
	"CLAUDE_CODE_EFFORT_LEVEL":             true,
	"CLAUDE_EFFORT":                        true,
	"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": true,
}

// claudeEnv returns environ without the variables in sessionEnv.
func claudeEnv(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, kv := range environ {
		name, _, _ := strings.Cut(kv, "=")
		if !sessionEnv[name] {
			out = append(out, kv)
		}
	}
	return out
}

// moondbTagsEnv is the variable moondb's capture hook reads a session's
// tags from (services/moondb, SessionTagsEnv), and evalTag the tag every
// case run's session gets, so moondb can tell eval sessions apart.
const (
	moondbTagsEnv = "MOONDB_SESSION_TAGS"
	evalTag       = "eval"
)

// caseEnv is the environment of one case run: claudeEnv with the session
// tags set to evalTag alone, whatever the caller's environment set.
func caseEnv(environ []string) []string {
	out := claudeEnv(environ)
	out = slices.DeleteFunc(out, func(kv string) bool {
		return strings.HasPrefix(kv, moondbTagsEnv+"=")
	})
	return append(out, moondbTagsEnv+"="+evalTag)
}

// claudeBin resolves the claude CLI, falling back to the install location
// under ~/.local/bin when PATH does not have it.
func claudeBin() string {
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "claude"
	}
	return filepath.Join(home, ".local", "bin", "claude")
}

// claudeVersion returns `claude --version`, or "unknown".
func claudeVersion(ctx context.Context, bin string) string {
	out, err := exec.CommandContext(ctx, bin, "--version").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

// invocation is one case run's claude command line. Model, Effort, and
// MaxBudgetUSD left zero leave the user's settings in charge.
type invocation struct {
	SessionID       string
	MaxTurns        int
	Model           string
	Effort          string
	PermissionMode  string
	DisallowedTools []string
	MaxBudgetUSD    float64
	Prompt          string
}

// args renders the command line. Settings, hooks, and MCP servers load the
// way an interactive session loads them; nothing here narrows them. The
// session is kept, not discarded: moondb reads the transcript Claude Code
// writes for it, and the scorecard's tokens, cost, searches and reads come
// from there. The prompt goes after "--" because --disallowedTools takes a
// list.
func (inv invocation) args() []string {
	args := []string{
		"-p",
		"--output-format", "stream-json",
		"--verbose",
		"--session-id", inv.SessionID,
		"--max-turns", strconv.Itoa(inv.MaxTurns),
	}
	if inv.Model != "" {
		args = append(args, "--model", inv.Model)
	}
	if inv.Effort != "" {
		args = append(args, "--effort", inv.Effort)
	}
	if inv.PermissionMode != "" {
		args = append(args, "--permission-mode", inv.PermissionMode)
	}
	if inv.MaxBudgetUSD > 0 {
		args = append(args, "--max-budget-usd", strconv.FormatFloat(inv.MaxBudgetUSD, 'f', -1, 64))
	}
	if len(inv.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools")
		args = append(args, inv.DisallowedTools...)
	}
	return append(args, "--", inv.Prompt)
}

// flagTemplate is the command line every case run shares, with the
// per-case values as placeholders, for run.json.
func flagTemplate(model, effort string, maxBudgetUSD float64) []string {
	args := invocation{
		SessionID: "<uuid>", Model: model, Effort: effort, MaxBudgetUSD: maxBudgetUSD,
		Prompt: "<case prompt>",
	}.args()
	for i, a := range args {
		if a == "--max-turns" {
			args[i+1] = "<case max_turns>"
		}
	}
	return args
}

// claudeCmd runs claude in its own process group, so the runner can signal
// claude together with the MCP servers and hooks it starts.
type claudeCmd struct {
	bin   string
	grace time.Duration
}

// run runs one case in dir and writes claude's stdout, the stream-json
// transcript, to stream. Its session carries the eval tag (caseEnv). When
// ctx ends first the group gets SIGTERM. On
// every exit path, timeout or not, whatever is left of the group gets
// SIGTERM and then SIGKILL after the grace period, so no MCP server or
// hook outlives the case. The error carries the tail of stderr.
func (c claudeCmd) run(ctx context.Context, dir string, inv invocation, stream io.Writer) error {
	cmd := exec.CommandContext(ctx, c.bin, inv.args()...)
	cmd.Dir = dir
	cmd.Env = caseEnv(os.Environ())
	cmd.Stdout = stream
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return signalGroup(cmd.Process.Pid, syscall.SIGTERM) }
	cmd.WaitDelay = c.grace
	err := cmd.Run()
	if cmd.Process != nil {
		reapGroup(cmd.Process.Pid, c.grace)
	}
	if err != nil {
		if ctx.Err() != nil {
			err = fmt.Errorf("%w (%w)", err, ctx.Err())
		}
		return fmt.Errorf("claude: %w: %s", err, tail(stderr.String(), stderrTail))
	}
	return nil
}

// signalGroup sends sig to every process in group pgid. A group that is
// already gone is not an error.
func signalGroup(pgid int, sig syscall.Signal) error {
	if err := syscall.Kill(-pgid, sig); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// reapGroup ends what is left of group pgid: SIGTERM, a wait of up to
// grace for the group to empty, then SIGKILL.
func reapGroup(pgid int, grace time.Duration) {
	if errors.Is(syscall.Kill(-pgid, syscall.SIGTERM), syscall.ESRCH) {
		return
	}
	deadline := time.Now().Add(grace)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(-pgid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(groupPoll)
	}
	_ = signalGroup(pgid, syscall.SIGKILL)
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n:]
}

// judgeSystem frames the llm grader. The rubric comes in the user prompt.
const judgeSystem = `You grade one answer from a coding agent against a rubric.
Read the task, the rubric, and the answer. Decide whether the answer meets
the rubric. Reply with a single JSON object and nothing else:
{"pass": true or false, "reason": "one sentence"}`

// claudeJudge is the production judge: a one-shot haiku call with no
// tools, no MCP servers, no session, and a small spend cap. User settings
// still load, so their hooks would fire; the flag settings turn every hook
// off (disableAllHooks), and the run starts in the temp directory so no
// repo's project settings or CLAUDE.md apply.
type claudeJudge struct {
	bin string
}

// Judge asks haiku whether answer meets rubric for the task in prompt.
func (j claudeJudge) Judge(ctx context.Context, rubric, prompt, answer string) (Verdict, error) {
	ctx, cancel := context.WithTimeout(ctx, judgeTimeout)
	defer cancel()
	user := "Task:\n" + prompt + "\n\nRubric:\n" + rubric + "\n\nAnswer:\n" + answer
	cmd := exec.CommandContext(ctx, j.bin,
		"-p",
		"--model", "haiku",
		"--tools", "",
		"--strict-mcp-config",
		"--settings", `{"disableAllHooks":true}`,
		"--no-session-persistence",
		"--max-budget-usd", judgeMaxBudgetUSD,
		"--output-format", "json",
		"--system-prompt", judgeSystem,
		"--", user,
	)
	cmd.Dir = os.TempDir()
	cmd.Env = claudeEnv(os.Environ())
	out, err := cmd.Output()
	if err != nil {
		return Verdict{}, fmt.Errorf("exec claude -p: %w", err)
	}
	return parseJudgeOutput(out)
}

// parseJudgeOutput reads `claude -p --output-format json` output whose
// result is the judge's JSON verdict, possibly wrapped in a code fence.
func parseJudgeOutput(out []byte) (Verdict, error) {
	var res struct {
		Result       string  `json:"result"`
		IsError      bool    `json:"is_error"`
		TotalCostUSD float64 `json:"total_cost_usd"`
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return Verdict{}, fmt.Errorf("decode claude -p output: %w", err)
	}
	v := Verdict{CostUSD: res.TotalCostUSD}
	if res.IsError {
		return v, fmt.Errorf("judge errored: %s", clip(res.Result, 200))
	}
	start, end := strings.Index(res.Result, "{"), strings.LastIndex(res.Result, "}")
	if start < 0 || end < start {
		return v, fmt.Errorf("no JSON object in judge reply: %s", clip(res.Result, 200))
	}
	if err := json.Unmarshal([]byte(res.Result[start:end+1]), &v); err != nil {
		return v, fmt.Errorf("decode judge verdict: %w", err)
	}
	return v, nil
}

// scorecardUnavailable is the status a row records when moondb has no
// scorecard for the session.
const scorecardUnavailable = "unavailable"

// fetchScorecard asks moondb for the session's scorecard. It never fails
// the run: anything but a 200 with a JSON body records "unavailable".
func fetchScorecard(
	ctx context.Context,
	client *http.Client,
	baseURL, sessionID string,
) (json.RawMessage, string) {
	if baseURL == "" || sessionID == "" {
		return nil, scorecardUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, scorecardTimeout)
	defer cancel()
	url := strings.TrimSuffix(baseURL, "/") + "/api/sessions/" + sessionID + "/scorecard"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, scorecardUnavailable
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, scorecardUnavailable
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil || resp.StatusCode != http.StatusOK || !json.Valid(body) {
		return nil, scorecardUnavailable
	}
	return body, "ok"
}

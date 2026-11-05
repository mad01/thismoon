package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Row is one (case, rep) result as results.jsonl stores it. DurationMS is
// claude's own figure; WallMS is the runner's clock around the process, so
// it includes MCP server startup and every hook. ToolCalls and Tools count
// the main session; the Agent fields count subagents. CSLCalls counts
// csl use, MCP or CLI, in the main session and subagents together, and
// BrainCalls and BrainCommands the brain CLI runs that were allowed, by
// subcommand; CPUSeconds is claude's CPU time plus its waited-for
// children. A brain workspace's row lists the vault paths the run changed
// and names the diff file under the run directory. Error collects anything
// that went wrong around the run, a changed canonical checkout included;
// the graders still score what there is.
type Row struct {
	Case                string          `json:"case"`
	Rep                 int             `json:"rep"`
	Workspace           string          `json:"workspace"`
	Score               float64         `json:"score"`
	Pass                bool            `json:"pass"`
	Graders             []GraderResult  `json:"graders"`
	Turns               int             `json:"turns"`
	CostUSD             float64         `json:"cost_usd"`
	JudgeCostUSD        float64         `json:"judge_cost_usd"`
	DurationMS          int64           `json:"duration_ms"`
	WallMS              int64           `json:"wall_ms"`
	InputTokens         int             `json:"input_tokens"`
	OutputTokens        int             `json:"output_tokens"`
	CacheReadTokens     int             `json:"cache_read_tokens"`
	CacheCreationTokens int             `json:"cache_creation_tokens"`
	Tokens              int             `json:"tokens"`
	ToolCalls           int             `json:"tool_calls"`
	CSLCalls            int             `json:"csl_calls"`
	BrainCalls          int             `json:"brain_calls,omitempty"`
	BrainCommands       map[string]int  `json:"brain_commands,omitempty"`
	CPUSeconds          float64         `json:"cpu_seconds"`
	Tools               map[string]int  `json:"tools"`
	AgentToolCalls      int             `json:"agent_tool_calls"`
	AgentTools          map[string]int  `json:"agent_tools,omitempty"`
	PermissionDenials   int             `json:"permission_denials"`
	Stop                string          `json:"stop"`
	Model               string          `json:"model"`
	SessionID           string          `json:"session_id"`
	Stream              string          `json:"stream"`
	Diff                string          `json:"diff,omitempty"`
	Changes             []Change        `json:"changes,omitempty"`
	Scorecard           json.RawMessage `json:"scorecard,omitempty"`
	ScorecardStatus     string          `json:"scorecard_status"`
	Error               string          `json:"error,omitempty"`
}

// errNoResult marks a run whose transcript has no result event.
var errNoResult = errors.New("harness: run ended without a result event")

// runner runs cases through claude one at a time. Model, effort, and
// maxBudgetUSD left zero leave the user's settings in charge; maxTotalUSD
// 0 means no run-wide cap. streamDir and diffDir are the run directory's
// streams/ and diffs/.
type runner struct {
	claude       claudeCmd
	profile      profile
	model        string
	effort       string
	maxBudgetUSD float64
	maxTotalUSD  float64
	roots        workspaceRoots
	scorecardURL string
	client       *http.Client
	judge        judge
	streamDir    string
	diffDir      string
	now          func() time.Time
	log          io.Writer
}

// runAll runs every case reps times, case by case. It stops early when ctx
// is cancelled or, checked before each case, when the run has spent
// maxTotalUSD; stopped says why, or is "" for a full run.
func (r *runner) runAll(ctx context.Context, cases []Case, reps int) (rows []Row, stopped string) {
	total := len(cases) * reps
	var spent float64
	for _, c := range cases {
		for rep := range reps {
			if ctx.Err() != nil {
				return rows, "interrupted"
			}
			if r.maxTotalUSD > 0 && spent >= r.maxTotalUSD {
				stopped = fmt.Sprintf(
					"spent $%.2f, at or over -max-total-usd $%.2f, with %d of %d runs done",
					spent,
					r.maxTotalUSD,
					len(rows),
					total,
				)
				fmt.Fprintln(r.log, "stopping: "+stopped)
				return rows, stopped
			}
			fmt.Fprintf(r.log, "[%d/%d] %s rep %d ... ", len(rows)+1, total, c.ID, rep)
			row := r.runCase(ctx, c, rep)
			spent += row.CostUSD + row.JudgeCostUSD
			fmt.Fprintf(r.log, "score %.2f, %d turns, %d tool calls, $%.3f\n",
				row.Score, row.Turns, row.ToolCalls, row.CostUSD)
			if row.Error != "" {
				fmt.Fprintf(r.log, "    error: %s\n", clip(row.Error, 300))
			}
			rows = append(rows, row)
		}
	}
	return rows, ""
}

// runCase prepares the workspace and snapshots its canonical checkout,
// runs claude, grades the run while the workspace still exists, tears the
// workspace down, and then checks the canonical checkout did not change.
func (r *runner) runCase(ctx context.Context, c Case, rep int) Row {
	row := Row{
		Case: c.ID, Rep: rep, Workspace: c.Workspace.String(),
		ScorecardStatus: scorecardUnavailable,
	}
	ws, err := prepareWorkspace(ctx, c.Workspace, r.roots, c.ID, r.now())
	if err != nil {
		row.Error = err.Error()
		return row
	}
	before, snapErr := takeSnapshot(ctx, ws.Canonical())
	errs := []error{snapErr, r.runIn(ctx, c, rep, ws, &row)}
	if err := ws.Cleanup(); err != nil {
		errs = append(errs, fmt.Errorf("cleanup: %w", err))
	}
	if snapErr == nil {
		errs = append(errs, checkUnchanged(ws.Canonical(), before))
	}
	if err := errors.Join(errs...); err != nil {
		row.Error = err.Error()
	}
	return row
}

// checkUnchanged compares the canonical checkout with its snapshot from
// before the case. It runs on a fresh context so an interrupted run still
// reports a change it made.
func checkUnchanged(dir string, before snapshot) error {
	after, err := takeSnapshot(context.Background(), dir)
	if err != nil {
		return fmt.Errorf("canonical checkout snapshot: %w", err)
	}
	if d := before.changes(after); d != "" {
		return fmt.Errorf("canonical checkout %s changed during the case: %s", dir, d)
	}
	return nil
}

// runIn runs and grades one case in a prepared workspace and fills row.
func (r *runner) runIn(ctx context.Context, c Case, rep int, ws *Workspace, row *Row) error {
	sessionID := uuid.NewString()
	name := fmt.Sprintf("%s-r%d", c.ID, rep)
	row.Stream = filepath.Join("streams", name+".jsonl")
	t, used, runErr := r.execute(ctx, c, ws, sessionID, filepath.Join(r.streamDir, name+".jsonl"))
	commits, commitErr := ws.Commits(ctx)
	vault, vaultErr := r.keepVault(ctx, ws, name, row)
	ev := Evidence{
		Transcript: t, Dir: ws.ContentDir(), Commits: commits, Prompt: c.Prompt,
		Files: vault.Files, Changes: vault.Changes, Diff: vault.Diff,
	}
	fillRow(row, t, gradeRun(ctx, c, ev, r.judge), used)
	if row.SessionID == "" {
		row.SessionID = sessionID
	}
	row.Scorecard, row.ScorecardStatus = fetchScorecard(
		ctx,
		r.client,
		r.scorecardURL,
		row.SessionID,
	)
	return errors.Join(runErr, t.Problems(), commitErr, vaultErr)
}

// keepVault reads back what the run left in a brain workspace's vault,
// writes its diff to diffs/<name>.patch and records the changes on row.
// Other workspaces have no vault and keep nothing.
func (r *runner) keepVault(
	ctx context.Context,
	ws *Workspace,
	name string,
	row *Row,
) (vaultState, error) {
	if ws.Kind != workspaceBrain {
		return vaultState{}, nil
	}
	vault, err := ws.Vault(ctx)
	if err != nil {
		return vaultState{}, fmt.Errorf("vault diff: %w", err)
	}
	row.Changes = vault.Changes
	row.Diff = filepath.Join("diffs", name+".patch")
	diff := vault.Diff
	if diff != "" {
		diff += "\n"
	}
	if err := os.WriteFile(filepath.Join(r.diffDir, name+".patch"), []byte(diff), 0o644); err != nil {
		return vault, fmt.Errorf("vault diff: %w", err)
	}
	return vault, nil
}

// resources is what one run spent outside the transcript: the runner's
// clock around the process and the CPU time the process reported.
type resources struct {
	Wall time.Duration
	CPU  time.Duration
}

// execute runs claude for one case under the case timeout, keeps the raw
// transcript at streamPath, and parses it back.
func (r *runner) execute(
	ctx context.Context,
	c Case,
	ws *Workspace,
	sessionID, streamPath string,
) (Transcript, resources, error) {
	f, err := os.Create(streamPath)
	if err != nil {
		return Transcript{}, resources{}, fmt.Errorf("transcript file: %w", err)
	}
	defer func() { _ = f.Close() }()
	runCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	start := r.now()
	cpu, runErr := r.claude.run(runCtx, ws, r.invocation(c, sessionID), f)
	used := resources{Wall: r.now().Sub(start), CPU: cpu}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Transcript{}, used, errors.Join(runErr, err)
	}
	t, parseErr := parseStream(f)
	if !t.HasResult {
		runErr = errors.Join(runErr, errNoResult)
	}
	return t, used, errors.Join(runErr, parseErr)
}

// invocation is the claude command line for one run of c. The case's
// allowed_tools replace the profile's allow list, and the profile's
// permission mode applies when the case sets none.
func (r *runner) invocation(c Case, sessionID string) invocation {
	allowed := r.profile.Allow
	if len(c.AllowedTools) > 0 {
		allowed = c.AllowedTools
	}
	mode := c.PermissionMode
	if mode == "" {
		mode = r.profile.PermissionMode
	}
	return invocation{
		SessionID:       sessionID,
		MaxTurns:        c.MaxTurns,
		Model:           r.model,
		Effort:          r.effort,
		PermissionMode:  mode,
		Tools:           r.profile.tools(allowed),
		DisallowedTools: appendNew(c.Disallowed(), r.profile.Deny...),
		AllowedTools:    allowed,
		MaxBudgetUSD:    r.maxBudgetUSD,
		Prompt:          c.Prompt,
		Profile:         r.profile,
	}
}

// fillRow copies the transcript's numbers and the grade onto row.
func fillRow(row *Row, t Transcript, g Grade, used resources) {
	row.Score, row.Pass, row.Graders = g.Score, g.Pass, g.Results
	row.JudgeCostUSD = g.JudgeCostUSD
	row.Turns, row.CostUSD, row.DurationMS = t.NumTurns, t.CostUSD, t.DurationMS
	row.WallMS = used.Wall.Milliseconds()
	row.CPUSeconds = used.CPU.Seconds()
	row.CSLCalls = cslCalls(t)
	brain := brainCallsIn(t.ToolCalls)
	row.BrainCalls, row.BrainCommands = len(brain), brainCommandCounts(brain)
	row.InputTokens, row.OutputTokens = t.Usage.InputTokens, t.Usage.OutputTokens
	row.CacheReadTokens, row.CacheCreationTokens = t.Usage.CacheReadTokens, t.Usage.CacheCreationTokens
	row.Tokens = t.Usage.Total()
	row.Tools, row.AgentTools = t.ToolCounts(originMain), t.ToolCounts(originAgent)
	row.ToolCalls, row.AgentToolCalls = len(t.Calls(false)), len(t.ToolCalls)-len(t.Calls(false))
	row.PermissionDenials, row.Stop = t.PermissionDenials, t.Subtype
	row.Model, row.SessionID = t.Model, t.SessionID
}

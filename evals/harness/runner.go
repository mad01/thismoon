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
// it includes MCP server startup and every hook. Error collects anything
// that went wrong around the run; the graders still score what there is.
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
	Tools               map[string]int  `json:"tools"`
	PermissionDenials   int             `json:"permission_denials"`
	Stop                string          `json:"stop"`
	Model               string          `json:"model"`
	SessionID           string          `json:"session_id"`
	Stream              string          `json:"stream"`
	Scorecard           json.RawMessage `json:"scorecard,omitempty"`
	ScorecardStatus     string          `json:"scorecard_status"`
	Error               string          `json:"error,omitempty"`
}

// errNoResult marks a run whose transcript has no result event.
var errNoResult = errors.New("harness: run ended without a result event")

// runner runs cases through claude one at a time. Model "" and
// MaxBudgetUSD 0 leave the user's settings in charge.
type runner struct {
	bin          string
	model        string
	maxBudgetUSD float64
	roots        workspaceRoots
	scorecardURL string
	client       *http.Client
	judge        judge
	streamDir    string
	now          func() time.Time
	log          io.Writer
}

// runAll runs every case reps times, case by case, and stops early when
// ctx is cancelled.
func (r *runner) runAll(ctx context.Context, cases []Case, reps int) []Row {
	var rows []Row
	total := len(cases) * reps
	for _, c := range cases {
		for rep := range reps {
			if ctx.Err() != nil {
				return rows
			}
			fmt.Fprintf(r.log, "[%d/%d] %s rep %d ... ", len(rows)+1, total, c.ID, rep)
			row := r.runCase(ctx, c, rep)
			fmt.Fprintf(r.log, "score %.2f, %d turns, %d tool calls, $%.3f\n",
				row.Score, row.Turns, row.ToolCalls, row.CostUSD)
			if row.Error != "" {
				fmt.Fprintf(r.log, "    error: %s\n", clip(row.Error, 300))
			}
			rows = append(rows, row)
		}
	}
	return rows
}

// runCase prepares the workspace, runs claude, grades the run while the
// workspace still exists, then tears the workspace down.
func (r *runner) runCase(ctx context.Context, c Case, rep int) (row Row) {
	row = Row{
		Case: c.ID, Rep: rep, Workspace: c.Workspace.String(),
		ScorecardStatus: scorecardUnavailable,
	}
	ws, err := prepareWorkspace(ctx, c.Workspace, r.roots, c.ID, r.now())
	if err != nil {
		row.Error = err.Error()
		return row
	}
	defer func() {
		if err := ws.Cleanup(); err != nil {
			row.Error = joinErrText(row.Error, "cleanup: "+err.Error())
		}
	}()
	sessionID := uuid.NewString()
	streamName := fmt.Sprintf("%s-r%d.jsonl", c.ID, rep)
	row.Stream = filepath.Join("streams", streamName)
	t, wall, runErr := r.execute(ctx, c, ws.Dir, sessionID, filepath.Join(r.streamDir, streamName))
	commits, commitErr := ws.Commits(ctx)
	ev := Evidence{Transcript: t, Dir: ws.Dir, Commits: commits, Prompt: c.Prompt}
	fillRow(&row, t, gradeRun(ctx, c, ev, r.judge), wall)
	if row.SessionID == "" {
		row.SessionID = sessionID
	}
	if err := errors.Join(runErr, commitErr); err != nil {
		row.Error = err.Error()
	}
	row.Scorecard, row.ScorecardStatus = fetchScorecard(
		ctx,
		r.client,
		r.scorecardURL,
		row.SessionID,
	)
	return row
}

// execute runs claude for one case under the case timeout, keeps the raw
// transcript at streamPath, and parses it back.
func (r *runner) execute(
	ctx context.Context,
	c Case,
	dir, sessionID, streamPath string,
) (Transcript, time.Duration, error) {
	f, err := os.Create(streamPath)
	if err != nil {
		return Transcript{}, 0, fmt.Errorf("transcript file: %w", err)
	}
	defer func() { _ = f.Close() }()
	runCtx, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	inv := invocation{
		SessionID:       sessionID,
		MaxTurns:        c.MaxTurns,
		Model:           r.model,
		PermissionMode:  c.PermissionMode,
		DisallowedTools: c.DisallowedTools,
		MaxBudgetUSD:    r.maxBudgetUSD,
		Prompt:          c.Prompt,
	}
	start := r.now()
	runErr := runClaude(runCtx, r.bin, dir, inv, f)
	wall := r.now().Sub(start)
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Transcript{}, wall, errors.Join(runErr, err)
	}
	t, parseErr := parseStream(f)
	if !t.HasResult {
		runErr = errors.Join(runErr, errNoResult)
	}
	return t, wall, errors.Join(runErr, parseErr)
}

// fillRow copies the transcript's numbers and the grade onto row.
func fillRow(row *Row, t Transcript, g Grade, wall time.Duration) {
	row.Score, row.Pass, row.Graders = g.Score, g.Pass, g.Results
	row.JudgeCostUSD = g.JudgeCostUSD
	row.Turns, row.CostUSD, row.DurationMS = t.NumTurns, t.CostUSD, t.DurationMS
	row.WallMS = wall.Milliseconds()
	row.InputTokens, row.OutputTokens = t.Usage.InputTokens, t.Usage.OutputTokens
	row.CacheReadTokens, row.CacheCreationTokens = t.Usage.CacheReadTokens, t.Usage.CacheCreationTokens
	row.Tokens = t.Usage.Total()
	row.ToolCalls, row.Tools = len(t.ToolCalls), t.ToolCounts()
	row.PermissionDenials, row.Stop = t.PermissionDenials, t.Subtype
	row.Model, row.SessionID = t.Model, t.SessionID
}

func joinErrText(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

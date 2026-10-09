package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mad01/thismoon/tools/humanizer/internal/backend"
	"github.com/mad01/thismoon/tools/humanizer/internal/judge"
)

const (
	callTimeout = 90 * time.Second
	maxAttempts = 3
)

// Arm is one judge configuration under test. All arms go through OpenRouter
// and the production judge.Run path; only the model and the two optional
// request knobs differ.
type Arm struct {
	ID          string   `json:"id"`
	Model       string   `json:"model"`
	Temperature *float64 `json:"temperature,omitempty"`
	Effort      string   `json:"effort,omitempty"`
	Notes       string   `json:"notes"`
}

func allArms() []Arm {
	zero := 0.0
	return []Arm{
		{
			ID: "haiku45-t0", Model: "anthropic/claude-haiku-4.5", Temperature: &zero,
			Notes: "today's production: Haiku 4.5 pinned to temperature 0",
		},
		{
			ID: "haiku45", Model: "anthropic/claude-haiku-4.5",
			Notes: "Haiku 4.5 at the provider default temperature; isolates the sampling effect",
		},
		{
			ID: "haiku55", Model: "anthropic/claude-haiku-5.5",
			Notes: "Haiku 5.5 with provider defaults (adaptive thinking, medium effort)",
		},
		{
			ID: "haiku55-low", Model: "anthropic/claude-haiku-5.5", Effort: "low",
			Notes: "Haiku 5.5 with reasoning_effort low",
		},
	}
}

func selectArms(ids string) ([]Arm, error) {
	all := allArms()
	if ids == "" {
		return all, nil
	}
	var out []Arm
	for id := range strings.SplitSeq(ids, ",") {
		id = strings.TrimSpace(id)
		i := -1
		for k, a := range all {
			if a.ID == id {
				i = k
			}
		}
		if i < 0 {
			return nil, fmt.Errorf("unknown arm %q", id)
		}
		out = append(out, all[i])
	}
	return out, nil
}

// prices is USD per million tokens, input then output, as OpenRouter listed
// the Anthropic ids on 2026-10-08. Cost is derived from the row's recorded
// usage and model, never estimated from text length.
var prices = map[string][2]float64{
	"anthropic/claude-haiku-4.5": {1.00, 5.00},
	"anthropic/claude-haiku-5.5": {0.10, 0.50},
}

func costUSD(model string, promptTokens, completionTokens int) float64 {
	p, ok := prices[model]
	if !ok {
		return 0
	}
	return (float64(promptTokens)*p[0] + float64(completionTokens)*p[1]) / 1e6
}

// Row is one scored (case, arm, rep) result. Grade is nil for the
// ambiguous bucket. Token counts sum every HTTP call the attempt made, so
// a contract retry is paid for; latency is the final call alone.
type Row struct {
	ID               string             `json:"id"`
	Bucket           string             `json:"bucket"`
	Split            string             `json:"split"`
	Label            string             `json:"label"`
	Variant          string             `json:"variant"`
	Rep              int                `json:"rep"`
	Verdict          string             `json:"verdict"`
	Confidence       float64            `json:"confidence"`
	Signals          int                `json:"signals"`
	Summary          string             `json:"summary"`
	Grade            map[string]float64 `json:"grade,omitempty"`
	Model            string             `json:"model"`
	PromptTokens     int                `json:"prompt_tokens"`
	CompletionTokens int                `json:"completion_tokens"`
	ReasoningTokens  int                `json:"reasoning_tokens"`
	CostUSD          float64            `json:"cost_usd"`
	LatencyMS        int64              `json:"latency_ms"`
	Attempts         int                `json:"attempts"`
	Trace            string             `json:"trace"`
}

// ErrRow is an attempt that produced no scorable output. It lives in
// errors.jsonl so plumbing failures never land in the score.
type ErrRow struct {
	ID       string `json:"id"`
	Variant  string `json:"variant"`
	Rep      int    `json:"rep"`
	Class    string `json:"class"`
	Error    string `json:"error"`
	Attempts int    `json:"attempts"`
	Trace    string `json:"trace"`
}

type job struct {
	c   Case
	arm Arm
	rep int
}

// recordingBackend keeps every raw model reply for the trace, including the
// confidence_basis the judge parser drops.
type recordingBackend struct {
	backend.Backend
	raws []string
}

func (r *recordingBackend) Complete(ctx context.Context, system, user string) (string, error) {
	out, err := r.Backend.Complete(ctx, system, user)
	if err == nil {
		r.raws = append(r.raws, out)
	}
	return out, err
}

type trace struct {
	Case    string             `json:"case"`
	Variant string             `json:"variant"`
	Rep     int                `json:"rep"`
	Arm     Arm                `json:"arm"`
	Text    string             `json:"text"`
	Replies []string           `json:"replies"`
	Calls   []backend.CallInfo `json:"calls"`
	Verdict *judge.Verdict     `json:"verdict,omitempty"`
	Error   string             `json:"error,omitempty"`
}

// runJob judges one case once on one arm through the production path,
// retrying transient provider failures with jittered backoff. judge.Run's
// own contract retry stays in: the observer counts every HTTP call.
func runJob(ctx context.Context, key string, j job, traceDir string) (*Row, *ErrRow) {
	var calls []backend.CallInfo
	client, err := backend.NewOpenRouter(backend.ChatConfig{
		APIKey:          key,
		Model:           j.arm.Model,
		Temperature:     j.arm.Temperature,
		ReasoningEffort: j.arm.Effort,
		Observe:         func(ci backend.CallInfo) { calls = append(calls, ci) },
	})
	if err != nil {
		return nil, &ErrRow{
			ID:      j.c.ID,
			Variant: j.arm.ID,
			Rep:     j.rep,
			Class:   "harness",
			Error:   err.Error(),
		}
	}
	rec := &recordingBackend{Backend: client}
	var v *judge.Verdict
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, callTimeout)
		v, err = judge.Run(cctx, rec, j.c.Text)
		cancel()
		if err == nil || !transient(err, calls) || attempt == maxAttempts {
			break
		}
		time.Sleep(time.Duration(attempt*attempt)*time.Second + rand.N(time.Second))
	}
	tracePath := writeTrace(traceDir, j, rec.raws, calls, v, err)
	if err != nil {
		return nil, &ErrRow{
			ID: j.c.ID, Variant: j.arm.ID, Rep: j.rep,
			Class: classify(err, calls), Error: err.Error(), Attempts: len(calls), Trace: tracePath,
		}
	}
	last := calls[len(calls)-1]
	if !servedMatches(j.arm.Model, last.Model) {
		return nil, &ErrRow{
			ID: j.c.ID, Variant: j.arm.ID, Rep: j.rep, Class: "served-model-mismatch",
			Error:    fmt.Sprintf("asked %q, served %q", j.arm.Model, last.Model),
			Attempts: len(calls), Trace: tracePath,
		}
	}
	var in, out, reasoning int
	for _, c := range calls {
		in += c.PromptTokens
		out += c.CompletionTokens
		reasoning += c.ReasoningTokens
	}
	return &Row{
		ID: j.c.ID, Bucket: j.c.Bucket, Split: j.c.Split, Label: j.c.Label,
		Variant: j.arm.ID, Rep: j.rep, Verdict: v.Verdict, Confidence: v.Confidence, Signals: len(v.Signals), Summary: v.Summary,
		Grade: grade(j.c, v.Verdict), Model: last.Model,
		PromptTokens: in, CompletionTokens: out, ReasoningTokens: reasoning,
		CostUSD:   costUSD(j.arm.Model, in, out),
		LatencyMS: last.Latency.Milliseconds(), Attempts: len(calls), Trace: tracePath,
	}, nil
}

// transient reports whether the failure is worth another attempt: a rate
// limit or server error on the last call, or no response at all.
func transient(err error, calls []backend.CallInfo) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	if len(calls) == 0 {
		return true
	}
	status := calls[len(calls)-1].Status
	return status == http.StatusTooManyRequests || status >= 500
}

func classify(err error, calls []backend.CallInfo) string {
	msg := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case strings.Contains(msg, "JSON contract"):
		return "contract"
	case strings.Contains(msg, "no content"):
		return "empty"
	case len(calls) == 0:
		return "network"
	}
	status := calls[len(calls)-1].Status
	switch {
	case status == http.StatusTooManyRequests || status >= 500:
		return "transient-exhausted"
	case status >= 400:
		return fmt.Sprintf("http-%d", status)
	}
	return "other"
}

// servedMatches accepts the exact id or a provider suffix on it (OpenRouter
// may report a routing variant); anything else is a substitution.
func servedMatches(asked, served string) bool {
	return served == asked || strings.HasPrefix(served, asked+":")
}

func writeTrace(
	dir string, j job, raws []string, calls []backend.CallInfo, v *judge.Verdict, err error,
) string {
	t := trace{
		Case: j.c.ID, Variant: j.arm.ID, Rep: j.rep, Arm: j.arm, Text: j.c.Text,
		Replies: raws, Calls: calls, Verdict: v,
	}
	if err != nil {
		t.Error = err.Error()
	}
	path := filepath.Join(dir, fmt.Sprintf("%s__%s__rep%d.json", j.arm.ID, j.c.ID, j.rep))
	data, merr := json.MarshalIndent(t, "", "  ")
	if merr == nil {
		merr = os.WriteFile(path, data, 0o644)
	}
	if merr != nil {
		fmt.Fprintf(os.Stderr, "trace %s: %v\n", path, merr)
	}
	return path
}

// runAll drives the jobs through a worker pool and returns rows and errors
// in a stable order.
func runAll(
	ctx context.Context,
	key string,
	jobs []job,
	traceDir string,
	workers int,
) ([]Row, []ErrRow) {
	var (
		mu   sync.Mutex
		rows []Row
		errs []ErrRow
		wg   sync.WaitGroup
	)
	queue := make(chan job)
	for range workers {
		wg.Go(func() {
			for j := range queue {
				row, erow := runJob(ctx, key, j, traceDir)
				mu.Lock()
				if row != nil {
					rows = append(rows, *row)
					fmt.Fprintf(os.Stderr, "%-12s %-40s rep%d %-12s %.2f %5dms\n",
						j.arm.ID, j.c.ID, j.rep, row.Verdict, row.Confidence, row.LatencyMS)
				} else {
					errs = append(errs, *erow)
					fmt.Fprintf(os.Stderr, "%-12s %-40s rep%d ERROR %s: %s\n",
						j.arm.ID, j.c.ID, j.rep, erow.Class, erow.Error)
				}
				mu.Unlock()
			}
		})
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	sortRows(rows)
	return rows, errs
}

package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"
)

// runMeta is run.json: what ran, with which claude and flags, and the
// totals once the run finishes.
type runMeta struct {
	RunID         string    `json:"run_id"`
	StartedAt     time.Time `json:"started_at"`
	FinishedAt    time.Time `json:"finished_at,omitzero"`
	ClaudeVersion string    `json:"claude_version"`
	Model         string    `json:"model"`
	Flags         []string  `json:"flags"`
	Cases         []string  `json:"cases"`
	Reps          int       `json:"reps"`
	MaxBudgetUSD  float64   `json:"max_budget_usd"`
	Totals        totals    `json:"totals"`
}

// totals sums a run's rows.
type totals struct {
	Rows         int     `json:"rows"`
	Passed       int     `json:"passed"`
	MeanScore    float64 `json:"mean_score"`
	CostUSD      float64 `json:"cost_usd"`
	JudgeCostUSD float64 `json:"judge_cost_usd"`
	Tokens       int     `json:"tokens"`
	ToolCalls    int     `json:"tool_calls"`
	WallMS       int64   `json:"wall_ms"`
	Errors       int     `json:"errors"`
}

func totalsOf(rows []Row) totals {
	var t totals
	for _, r := range rows {
		t.Rows++
		if r.Pass {
			t.Passed++
		}
		if r.Error != "" {
			t.Errors++
		}
		t.MeanScore += r.Score
		t.CostUSD += r.CostUSD
		t.JudgeCostUSD += r.JudgeCostUSD
		t.Tokens += r.Tokens
		t.ToolCalls += r.ToolCalls
		t.WallMS += r.WallMS
	}
	if t.Rows > 0 {
		t.MeanScore /= float64(t.Rows)
	}
	return t
}

// modelLabel names the model a run used for reports.
func modelLabel(model string) string {
	if model == "" {
		return "user default"
	}
	return model
}

// writeSummary renders summary.md: the headline, one row per (case, rep),
// the tool calls of each row, failed graders, errors, and scorecards.
func writeSummary(w io.Writer, meta runMeta, rows []Row) {
	t := totalsOf(rows)
	fmt.Fprintf(w, "# Harness eval: %s\n\n", meta.RunID)
	fmt.Fprintf(w, "%s, model %s, cases %d, reps %d.\n\n",
		meta.ClaudeVersion, modelLabel(meta.Model), len(meta.Cases), meta.Reps)
	fmt.Fprintf(w, "%d rows, %d passed, mean score %.2f. Cost $%.3f plus judge $%.3f. "+
		"%s tokens, %d tool calls, %s wall.\n\n",
		t.Rows, t.Passed, t.MeanScore, t.CostUSD, t.JudgeCostUSD,
		kilo(t.Tokens), t.ToolCalls, time.Duration(t.WallMS)*time.Millisecond)

	fmt.Fprintln(
		w,
		"| case | rep | score | pass | turns | tool calls | tokens | cost | seconds | stop |",
	)
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|")
	for _, r := range rows {
		fmt.Fprintf(w, "| %s | %d | %.2f | %s | %d | %d | %s | $%.3f | %.0f | %s |\n",
			r.Case, r.Rep, r.Score, yesNo(r.Pass), r.Turns, r.ToolCalls, kilo(r.Tokens),
			r.CostUSD, float64(r.WallMS)/1000, orDash(r.Stop))
	}

	fmt.Fprintln(w, "\n## Tool calls")
	fmt.Fprintln(w)
	for _, r := range rows {
		fmt.Fprintf(w, "- %s rep %d: %s\n", r.Case, r.Rep, orDash(formatCounts(r.Tools)))
	}
	writeFailures(w, rows)

	unavailable := 0
	for _, r := range rows {
		if r.ScorecardStatus != "ok" {
			unavailable++
		}
	}
	fmt.Fprintf(w, "\nScorecard: %d of %d rows unavailable.\n", unavailable, len(rows))
}

func writeFailures(w io.Writer, rows []Row) {
	var lines []string
	for _, r := range rows {
		for _, g := range r.Graders {
			if !g.Pass {
				lines = append(lines, fmt.Sprintf("- %s rep %d: `%s`: %s",
					r.Case, r.Rep, g.Grader, orDash(g.Detail)))
			}
		}
		if r.Error != "" {
			lines = append(lines, fmt.Sprintf("- %s rep %d: error: %s",
				r.Case, r.Rep, clip(r.Error, 300)))
		}
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(w, "\n## Failures")
	fmt.Fprintln(w)
	fmt.Fprintln(w, strings.Join(lines, "\n"))
}

// caseMeans is one case's numbers averaged over its reps.
type caseMeans struct {
	Reps      int
	Score     float64
	CostUSD   float64
	Turns     float64
	Tokens    float64
	ToolCalls float64
}

func meansByCase(rows []Row) map[string]caseMeans {
	out := map[string]caseMeans{}
	for _, r := range rows {
		m := out[r.Case]
		m.Reps++
		m.Score += r.Score
		m.CostUSD += r.CostUSD
		m.Turns += float64(r.Turns)
		m.Tokens += float64(r.Tokens)
		m.ToolCalls += float64(r.ToolCalls)
		out[r.Case] = m
	}
	for id, m := range out {
		n := float64(m.Reps)
		out[id] = caseMeans{
			Reps: m.Reps, Score: m.Score / n, CostUSD: m.CostUSD / n,
			Turns: m.Turns / n, Tokens: m.Tokens / n, ToolCalls: m.ToolCalls / n,
		}
	}
	return out
}

// writeCompare renders per-case deltas from run a to run b, each value the
// mean over that run's reps. A case in only one run gets a note instead of
// deltas.
func writeCompare(w io.Writer, labelA, labelB string, a, b []Row) {
	ma, mb := meansByCase(a), meansByCase(b)
	ids := maps.Clone(ma)
	maps.Copy(ids, mb)
	fmt.Fprintf(w, "# Compare %s to %s\n\n", labelA, labelB)
	fmt.Fprintln(w, "Each cell is run B's mean over its reps, then the change from run A.")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| case | reps A/B | score | cost | turns | tokens | tool calls |")
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|")
	for _, id := range slices.Sorted(maps.Keys(ids)) {
		x, inA := ma[id]
		y, inB := mb[id]
		switch {
		case !inB:
			fmt.Fprintf(w, "| %s | %d/0 | only in A | | | | |\n", id, x.Reps)
		case !inA:
			fmt.Fprintf(w, "| %s | 0/%d | only in B | | | | |\n", id, y.Reps)
		default:
			fmt.Fprintf(w, "| %s | %d/%d | %.2f (%+.2f) | $%.3f (%+.3f) | %.1f (%+.1f) "+
				"| %s (%s) | %.1f (%+.1f) |\n",
				id, x.Reps, y.Reps, y.Score, y.Score-x.Score, y.CostUSD, y.CostUSD-x.CostUSD,
				y.Turns, y.Turns-x.Turns, kilo(int(y.Tokens)), signedKilo(y.Tokens-x.Tokens),
				y.ToolCalls, y.ToolCalls-x.ToolCalls)
		}
	}
	ta, tb := totalsOf(a), totalsOf(b)
	fmt.Fprintf(w, "\nTotals: mean score %.2f (%+.2f), cost $%.3f (%+.3f), "+
		"tool calls %d (%+d).\n",
		tb.MeanScore, tb.MeanScore-ta.MeanScore, tb.CostUSD, tb.CostUSD-ta.CostUSD,
		tb.ToolCalls, tb.ToolCalls-ta.ToolCalls)
}

// kilo renders a token count in thousands.
func kilo(n int) string {
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}

func signedKilo(d float64) string {
	return fmt.Sprintf("%+.1fk", d/1000)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

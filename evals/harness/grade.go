package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Evidence is what graders read: the run's transcript, the workspace it
// ran in, the subjects of the commits it made there, and the prompt the
// llm grader shows its judge.
type Evidence struct {
	Transcript Transcript
	Dir        string
	Commits    []string
	Prompt     string
}

// judge is the llm grader's model call. It is an interface so tests grade
// without the claude binary.
type judge interface {
	Judge(ctx context.Context, rubric, prompt, answer string) (Verdict, error)
}

// Verdict is a judge's answer on one rubric, with what the call cost.
type Verdict struct {
	Pass    bool    `json:"pass"`
	Reason  string  `json:"reason"`
	CostUSD float64 `json:"-"`
}

// GraderResult is one grader's outcome on one run.
type GraderResult struct {
	Grader string  `json:"grader"`
	Weight float64 `json:"weight"`
	Pass   bool    `json:"pass"`
	Detail string  `json:"detail,omitempty"`
}

// Grade is the scored outcome of one run. Score is the passed share of the
// total grader weight; Pass means every grader passed.
type Grade struct {
	Score        float64
	Pass         bool
	Results      []GraderResult
	JudgeCostUSD float64
}

// gradeRun runs every grader of c against ev. A nil judge fails llm
// graders with a detail that says so, rather than skipping them.
func gradeRun(ctx context.Context, c Case, ev Evidence, j judge) Grade {
	g := Grade{Pass: true}
	var total, passed float64
	for _, gr := range c.Graders {
		res, cost := evaluate(ctx, gr, ev, j)
		g.Results = append(g.Results, res)
		g.JudgeCostUSD += cost
		total += gr.Weight
		if res.Pass {
			passed += gr.Weight
		} else {
			g.Pass = false
		}
	}
	if total > 0 {
		g.Score = passed / total
	}
	return g
}

// evaluate runs one grader and returns its result and any judge cost.
func evaluate(ctx context.Context, g Grader, ev Evidence, j judge) (GraderResult, float64) {
	res := GraderResult{Grader: g.Name(), Weight: g.Weight}
	var cost float64
	calls := ev.Transcript.Calls(g.IncludeAgents)
	switch g.Type {
	case gradeRegex:
		res.Pass = g.patternRE.MatchString(ev.Transcript.Result)
		if !res.Pass {
			res.Detail = "result does not match"
		}
	case gradeFile:
		res.Pass, res.Detail = checkFile(g, ev.Dir)
	case gradeCommit:
		res.Pass, res.Detail = checkCommits(g, ev.Commits)
	case gradeToolUsed:
		res.Pass = len(matchingCalls(g, calls)) > 0
		if !res.Pass {
			res.Detail = "no matching call; tools: " + toolSummary(calls)
		}
	case gradeToolNotUsed:
		matched := matchingCalls(g, calls)
		res.Pass = len(matched) == 0
		if !res.Pass {
			res.Detail = fmt.Sprintf("%d matching calls, first %s %s",
				len(matched), matched[0].Name, clip(matched[0].Input, 160))
		}
	case gradeToolBefore:
		res.Pass, res.Detail = checkOrder(g, calls)
	case gradeMaxTurns:
		res.Pass, res.Detail = atMost(float64(ev.Transcript.NumTurns), g.Max, "turns")
	case gradeMaxCostUSD:
		res.Pass, res.Detail = atMost(ev.Transcript.CostUSD, g.Max, "USD")
	case gradeMaxToolCalls:
		res.Pass, res.Detail = atMost(float64(len(calls)), g.Max, "tool calls")
	case gradeLLM:
		res.Pass, res.Detail, cost = checkLLM(ctx, g, ev, j)
	default:
		res.Detail = "unknown grader type"
	}
	return res, cost
}

func checkFile(g Grader, dir string) (bool, string) {
	data, err := os.ReadFile(filepath.Join(dir, g.Path))
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, "missing"
	case err != nil:
		return false, err.Error()
	case g.patternRE != nil && !g.patternRE.Match(data):
		return false, "content does not match"
	}
	return true, ""
}

func checkCommits(g Grader, subjects []string) (bool, string) {
	if len(subjects) == 0 {
		return false, "no commits"
	}
	for _, s := range subjects {
		if !g.patternRE.MatchString(s) {
			return false, fmt.Sprintf("subject %q does not match", s)
		}
	}
	return true, ""
}

// matchingCalls returns the calls whose name matches g.Tool and whose input
// matches g.Input and not g.InputWithout, each only when set.
func matchingCalls(g Grader, calls []ToolCall) []ToolCall {
	var out []ToolCall
	for _, c := range calls {
		if callMatches(g, c) {
			out = append(out, c)
		}
	}
	return out
}

func callMatches(g Grader, c ToolCall) bool {
	if !g.toolRE.MatchString(c.Name) {
		return false
	}
	for _, re := range g.inputREs {
		if !re.MatchString(c.Input) {
			return false
		}
	}
	return g.withoutRE == nil || !g.withoutRE.MatchString(c.Input)
}

// checkOrder passes when a call matching g.Tool comes before every call
// whose name matches g.Before. A run that never calls the Before tool
// passes as long as it calls the first one.
func checkOrder(g Grader, calls []ToolCall) (bool, string) {
	first := slices.IndexFunc(calls, func(c ToolCall) bool { return callMatches(g, c) })
	if first < 0 {
		return false, "first tool never called"
	}
	before := slices.IndexFunc(
		calls,
		func(c ToolCall) bool { return g.beforeRE.MatchString(c.Name) },
	)
	if before >= 0 && before < first {
		return false, fmt.Sprintf("%s at call %d, first match at call %d",
			calls[before].Name, before+1, first+1)
	}
	return true, ""
}

func atMost(got, limit float64, unit string) (bool, string) {
	detail := fmt.Sprintf("%s %s, max %s", roundNum(got), unit, roundNum(limit))
	return got <= limit, detail
}

// roundNum renders a number with at most four decimals, so a summed cost
// reads 1.3177 and not 1.3176865000000002.
func roundNum(x float64) string {
	return strconv.FormatFloat(math.Round(x*1e4)/1e4, 'f', -1, 64)
}

func checkLLM(ctx context.Context, g Grader, ev Evidence, j judge) (bool, string, float64) {
	if j == nil {
		return false, "no judge configured", 0
	}
	v, err := j.Judge(ctx, g.Rubric, ev.Prompt, ev.Transcript.Result)
	if err != nil {
		return false, "judge: " + err.Error(), v.CostUSD
	}
	return v.Pass, clip(v.Reason, 300), v.CostUSD
}

// toolSummary renders calls as "Name n, Name n", sorted by name, for
// failure details.
func toolSummary(calls []ToolCall) string {
	if len(calls) == 0 {
		return "none"
	}
	counts := map[string]int{}
	for _, c := range calls {
		counts[c.Name]++
	}
	return formatCounts(counts)
}

func formatCounts(counts map[string]int) string {
	parts := make([]string, 0, len(counts))
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		parts = append(parts, fmt.Sprintf("%s %d", name, counts[name]))
	}
	return strings.Join(parts, ", ")
}

// clip bounds a detail excerpt.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

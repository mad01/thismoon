// Package prereq checks what a run needs before it spends anything: the
// tools on PATH, a claude new enough for the unattended flags, and for the
// factory arm the factory and loom skills, the three worker definitions,
// and the belt hook that hands each worker its brief.
package prereq

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MinClaude is the oldest claude that takes --permission-prompts none.
var MinClaude = [3]int{2, 1, 259}

// WorkerDefinitions are the three agent definitions the factory skill
// spawns workers from, one per effort level.
var WorkerDefinitions = []string{
	"factory-worker-low",
	"factory-worker-medium",
	"factory-worker-high",
}

// Result is one prerequisite's outcome.
type Result struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// Env is the host the checks look at. Tests point Home at a temp dir.
type Env struct {
	Home          string
	LookPath      func(file string) (string, error)
	ClaudeVersion func(ctx context.Context) ([3]int, string, error)
}

// Common checks what every arm needs.
func Common(ctx context.Context, env Env) []Result {
	results := []Result{claudeVersion(ctx, env)}
	for _, bin := range []string{"git", "make", "go", "humanizer"} {
		results = append(results, onPath(env, bin))
	}
	return results
}

// Factory checks what the factory arm needs on top of Common.
func Factory(env Env) []Result {
	claudeDir := filepath.Join(env.Home, ".claude")
	results := []Result{
		file("factory skill", filepath.Join(claudeDir, "skills", "factory", "SKILL.md")),
		file("loom skill", filepath.Join(claudeDir, "skills", "loom", "SKILL.md")),
	}
	for _, w := range WorkerDefinitions {
		results = append(results, file("worker definition "+w,
			filepath.Join(claudeDir, "agents", w+".md")))
	}
	return append(results, beltSubagentHook(filepath.Join(claudeDir, "settings.json")))
}

// Failed returns the results that did not pass.
func Failed(results []Result) []Result {
	var failed []Result
	for _, r := range results {
		if !r.OK {
			failed = append(failed, r)
		}
	}
	return failed
}

// Explain renders failed results as one refusal message.
func Explain(arm string, failed []Result) string {
	lines := []string{
		fmt.Sprintf("the %s arm cannot run, %d prerequisites missing:", arm, len(failed)),
	}
	for _, r := range failed {
		lines = append(lines, fmt.Sprintf("  - %s: %s", r.Name, r.Detail))
	}
	if arm == "factory" {
		lines = append(lines, "The worker definitions and the belt hook wiring land with MAD-398; "+
			"see the factory-eval README, prerequisites.")
	}
	return strings.Join(lines, "\n")
}

func claudeVersion(ctx context.Context, env Env) Result {
	r := Result{Name: "claude version"}
	v, raw, err := env.ClaudeVersion(ctx)
	switch {
	case err != nil:
		r.Detail = err.Error()
	case older(v, MinClaude):
		r.Detail = fmt.Sprintf("%s is older than %d.%d.%d, which --permission-prompts needs",
			raw, MinClaude[0], MinClaude[1], MinClaude[2])
	default:
		r.OK, r.Detail = true, raw
	}
	return r
}

func older(v, min [3]int) bool {
	for i := range v {
		if v[i] != min[i] {
			return v[i] < min[i]
		}
	}
	return false
}

func onPath(env Env, bin string) Result {
	p, err := env.LookPath(bin)
	if err != nil {
		return Result{Name: bin, Detail: "not on PATH"}
	}
	return Result{Name: bin, OK: true, Detail: p}
}

func file(name, p string) Result {
	if _, err := os.Stat(p); err != nil {
		return Result{Name: name, Detail: p + " not found"}
	}
	return Result{Name: name, OK: true, Detail: p}
}

type settings struct {
	Hooks map[string][]struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	} `json:"hooks"`
}

// beltSubagentHook checks that a SubagentStart hook runs belt, the hint
// that delivers the worker brief before a worker's first prompt.
func beltSubagentHook(p string) Result {
	r := Result{Name: "belt SubagentStart hook"}
	raw, err := os.ReadFile(p)
	if err != nil {
		r.Detail = err.Error()
		return r
	}
	var s settings
	if err := json.Unmarshal(raw, &s); err != nil {
		r.Detail = fmt.Sprintf("%s: %v", p, err)
		return r
	}
	for _, group := range s.Hooks["SubagentStart"] {
		for _, h := range group.Hooks {
			if strings.Contains(h.Command, "belt") {
				r.OK, r.Detail = true, h.Command
				return r
			}
		}
	}
	r.Detail = "no SubagentStart hook in " + p + " runs belt"
	return r
}

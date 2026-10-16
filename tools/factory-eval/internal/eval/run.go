// Package eval runs the factory eval matrix: every run index, every tier,
// every arm, each in its own throwaway checkout, one at a time so no two
// runs compete for the machine. It scores each run's cases, applies the
// MAD-402 pass rule, and builds the one results document.
package eval

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/checkout"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
	"github.com/mad01/thismoon/tools/factory-eval/internal/judge"
	"github.com/mad01/thismoon/tools/factory-eval/internal/prereq"
)

// stopError is the Stop value of a run that ended without a result message.
const stopError = "error"

// AgentRunner makes one headless arm session.
type AgentRunner interface {
	Run(ctx context.Context, inv claude.Invocation) (claude.Output, error)
}

// CaseChecker runs the deterministic checks on one case's branch.
type CaseChecker interface {
	Run(ctx context.Context, co *checkout.Checkout, c cases.Case) []judge.Check
}

// CaseScorer makes the LLM judge call for one case.
type CaseScorer interface {
	Score(ctx context.Context, component string, files []judge.FilePair) (judge.Verdict, error)
}

// Config is one batch: what to run and the seams it runs through.
type Config struct {
	Cases         *cases.File
	CasesPath     string
	Source        string
	Arms          []string
	Tiers         []cases.Tier
	Runs          int
	TempParent    string
	DryRun        bool
	ClaudeVersion string
	Prereqs       []prereq.Result
	Agent         AgentRunner
	Checker       CaseChecker
	Scorer        CaseScorer
	Now           func() time.Time
	Progress      io.Writer
}

// Run executes the batch and returns its results. Runs interleave arms
// and tiers per run index, so drift over the day hits both arms alike. A
// cancelled ctx stops the batch after the run in flight.
func Run(ctx context.Context, cfg Config) Results {
	res := Results{
		Schema:        SchemaID,
		DryRun:        cfg.DryRun,
		StartedAt:     cfg.Now().UTC(),
		Commit:        cfg.Cases.Commit,
		CasesFile:     cfg.CasesPath,
		SignOff:       cfg.Cases.SignOff,
		ClaudeVersion: cfg.ClaudeVersion,
		Prereqs:       nonNil(cfg.Prereqs),
		Runs:          []RunResult{},
	}
	for i := 1; i <= cfg.Runs && ctx.Err() == nil; i++ {
		for _, t := range cfg.Tiers {
			for _, arm := range cfg.Arms {
				if ctx.Err() != nil {
					break
				}
				fmt.Fprintf(cfg.Progress, "run %d, tier %s, arm %s: starting\n", i, t.Name, arm)
				rr := runOne(ctx, cfg, arm, t, i)
				fmt.Fprintln(cfg.Progress, progressLine(rr))
				res.Runs = append(res.Runs, rr)
			}
		}
	}
	res.FinishedAt = cfg.Now().UTC()
	res.Summary = Summarize(res.Runs, cfg.Tiers, primaryTier(cfg))
	return res
}

func primaryTier(cfg Config) string {
	primary := cfg.Cases.PrimaryTier().Name
	for _, t := range cfg.Tiers {
		if t.Name == primary {
			return primary
		}
	}
	return cfg.Tiers[0].Name
}

func progressLine(rr RunResult) string {
	line := fmt.Sprintf("run %d, tier %s, arm %s: pass=%v stop=%s $%.2f %d tokens %s judge %.1f",
		rr.Index, rr.Tier, rr.Arm, rr.Pass, rr.Stop, rr.Dollars, rr.TokensTotal,
		time.Duration(rr.WallClockMS)*time.Millisecond, rr.JudgeScore)
	if rr.KeptDir != "" {
		line += " kept " + rr.KeptDir
	}
	if rr.Error != "" {
		line += "\n  error: " + rr.Error
	}
	return line
}

func runOne(ctx context.Context, cfg Config, arm string, tier cases.Tier, index int) RunResult {
	rr := RunResult{
		Arm: arm, Tier: tier.Name, Index: index,
		Model: tier.Model, Effort: tier.Effort, BudgetUSD: tier.BudgetUSD,
		Cases: []CaseResult{}, JudgeReasons: []string{},
	}
	co, err := checkout.Create(ctx, cfg.Source, cfg.Cases.Commit, cfg.TempParent)
	if err != nil {
		rr.Stop, rr.Error = stopError, err.Error()
		return rr
	}
	invokeArm(ctx, cfg, co, arm, tier, &rr)
	scoreCases(ctx, cfg, co, &rr)
	rr.Pass = rr.Error == "" && rr.Stop == claude.SubtypeSuccess && allCasesPass(rr.Cases)
	settle(co, &rr)
	return rr
}

// settle removes a passing run's checkout and keeps a failing one for
// inspection, its path in the results.
func settle(co *checkout.Checkout, rr *RunResult) {
	if rr.Pass {
		if err := co.Remove(); err == nil {
			return
		}
	}
	rr.KeptDir = co.Root
}

func armSpec(f *cases.File, arm string) cases.Arm {
	if arm == ArmFactory {
		return f.Arms.Factory
	}
	return f.Arms.Single
}

func invokeArm(ctx context.Context, cfg Config, co *checkout.Checkout, arm string,
	tier cases.Tier, rr *RunResult,
) {
	prompt, err := ArmPrompt(cfg.Cases, armSpec(cfg.Cases, arm), PromptInput{
		Repo: co.Repo, Worktrees: co.Worktrees, Commit: co.Commit,
		Model: tier.Model, Effort: tier.Effort, Worker: tier.Worker,
	})
	if err != nil {
		rr.Stop, rr.Error = stopError, err.Error()
		return
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.Cases.TimeoutMinutes)*time.Minute)
	defer cancel()
	out, err := cfg.Agent.Run(runCtx, claude.Invocation{
		Dir: co.Repo, Args: armArgs(arm, tier, co), Stdin: prompt,
	})
	rr.WallClockMS = out.Wall.Milliseconds()
	rep, perr := claude.ParseReport(out.Stdout)
	if perr == nil {
		recordReport(rr, rep)
	}
	switch {
	case err != nil:
		rr.Stop, rr.Error = stopError, err.Error()
	case perr != nil:
		rr.Stop = stopError
		rr.Error = fmt.Sprintf("%v (exit %d): %s", perr, out.ExitCode, lastLines(out.Stderr, 3))
	}
}

func recordReport(rr *RunResult, rep claude.Report) {
	t := rep.Tokens()
	rr.Stop = rep.Subtype
	rr.Dollars = rep.TotalCostUSD
	rr.Tokens = t
	rr.TokensIn, rr.TokensOut, rr.TokensTotal = t.In(), t.Output, t.Total()
	rr.NumTurns = rep.NumTurns
	rr.SessionID = rep.SessionID
	rr.PermissionDenials = len(rep.PermissionDenials)
	rr.AgentReport = rep.StructuredOutput
	if rep.IsError && len(rep.Errors) > 0 {
		rr.Error = strings.Join(rep.Errors, "; ")
	}
}

func lastLines(b []byte, n int) string {
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

func scoreCases(ctx context.Context, cfg Config, co *checkout.Checkout, rr *RunResult) {
	total := 0
	for _, c := range cfg.Cases.Cases {
		cr, cost := scoreCase(ctx, cfg, co, c)
		rr.Cases = append(rr.Cases, cr)
		rr.JudgeDollars += cost
		total += cr.Score
		for _, reason := range cr.JudgeReasons {
			rr.JudgeReasons = append(rr.JudgeReasons, c.Name()+": "+reason)
		}
	}
	if n := len(cfg.Cases.Cases); n > 0 {
		rr.JudgeScore = float64(total) / float64(n)
	}
}

// scoreCase runs the deterministic checks, then the judge whenever the
// branch exists. The judge's raw score is kept either way; the case's
// Score counts it only when every check passed.
func scoreCase(ctx context.Context, cfg Config, co *checkout.Checkout,
	c cases.Case,
) (CaseResult, float64) {
	cr := CaseResult{Component: c.Component, Branch: c.Branch, JudgeReasons: []string{}}
	cr.Checks = cfg.Checker.Run(ctx, co, c)
	cr.DeterministicPass = judge.AllPass(cr.Checks)
	if !co.BranchExists(ctx, c.Branch) {
		cr.JudgeError = "skipped: branch missing"
		return cr, 0
	}
	v, err := cfg.Scorer.Score(ctx, c.Component, filePairs(ctx, co, c))
	if err != nil {
		cr.JudgeError = err.Error()
		return cr, v.CostUSD
	}
	cr.JudgeScore = &v.Score
	cr.JudgeReasons = nonNil(v.Reasons)
	if cr.DeterministicPass {
		cr.Score = v.Score
	}
	return cr, v.CostUSD
}

func filePairs(ctx context.Context, co *checkout.Checkout, c cases.Case) []judge.FilePair {
	var pairs []judge.FilePair
	for _, p := range judge.TargetFiles(c) {
		pairs = append(pairs, judge.FilePair{
			Path: p, Before: co.Original(ctx, p), After: co.AtBranch(ctx, c.Branch, p),
		})
	}
	return pairs
}

func allCasesPass(crs []CaseResult) bool {
	for _, cr := range crs {
		if !cr.DeterministicPass {
			return false
		}
	}
	return len(crs) > 0
}

// nonNil turns a nil slice into an empty one, so the results file always
// holds an array where the schema promises one.
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

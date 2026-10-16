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

// judgeAttempts is how many calls a case's judge gets before its score is
// zeroed: one retry, so a single flaky call does not decide a case.
const judgeAttempts = 2

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
	Score(ctx context.Context, req judge.Request) (judge.Verdict, error)
}

// Config is one batch: what to run and the seams it runs through. Home is
// the user's home dir the isolation rules protect. Snapshot records what
// the host has installed; it runs before every run.
type Config struct {
	Cases         *cases.File
	CasesPath     string
	Source        string
	Home          string
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
	Snapshot      func(ctx context.Context) prereq.Installed
	Now           func() time.Time
	Progress      io.Writer
}

func (cfg Config) isolation() Isolation { return Isolation{Home: cfg.Home, Source: cfg.Source} }

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
		CasesDigest:   cfg.Cases.Digest(),
		SignOff:       cfg.Cases.SignOff,
		ClaudeVersion: cfg.ClaudeVersion,
		Prereqs:       nonNil(cfg.Prereqs),
		SettingsDeny:  cfg.isolation().DenyRules(),
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
	res.Summary = Summarize(res.Runs, cfg.Tiers, primaryTier(cfg), cfg.Runs)
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
	if rr.Tainted {
		line += "\n  tainted, the source checkout changed: " + strings.Join(rr.Taint, "; ")
	}
	return line
}

// runOne makes one arm session in a fresh clone and scores it. The source
// checkout is fingerprinted around the session, so a session that reached
// it is tainted even when no deny rule caught it.
func runOne(ctx context.Context, cfg Config, arm string, tier cases.Tier, index int) RunResult {
	rr := RunResult{
		Arm: arm, Tier: tier.Name, Index: index,
		Model: tier.Model, Effort: tier.Effort, BudgetUSD: tier.BudgetUSD,
		Installed: cfg.Snapshot(ctx), Taint: []string{},
		Cases: []CaseResult{}, JudgeReasons: []string{},
	}
	co, err := checkout.Create(ctx, cfg.Source, cfg.Cases.Commit, cfg.TempParent)
	if err != nil {
		rr.Stop, rr.Error = stopError, err.Error()
		return rr
	}
	if rr.SettingsFile, err = cfg.isolation().WriteSettings(co.Root); err != nil {
		rr.Stop, rr.Error = stopError, err.Error()
		settle(co, &rr)
		return rr
	}
	before, beforeErr := checkout.Fingerprint(ctx, cfg.Source)
	invokeArm(ctx, cfg, co, arm, tier, &rr)
	rr.Taint = taint(ctx, cfg.Source, before, beforeErr)
	rr.Tainted = len(rr.Taint) > 0
	scoreCases(ctx, cfg, co, &rr)
	rr.Pass = !rr.Errored() && allCasesPass(rr.Cases)
	settle(co, &rr)
	return rr
}

// taint lists how the source checkout changed between before and now. Any
// change taints the run, since factory-eval cannot tell the arm's change
// from another session's; so does a fingerprint that could not be read.
func taint(ctx context.Context, source string, before checkout.Print, beforeErr error) []string {
	if beforeErr != nil {
		return []string{"fingerprint before the arm: " + beforeErr.Error()}
	}
	after, err := checkout.Fingerprint(ctx, source)
	if err != nil {
		return []string{"fingerprint after the arm: " + err.Error()}
	}
	return nonNil(before.Changes(after))
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
		Dir: co.Repo, Args: armArgs(arm, tier, co, rr.SettingsFile), Stdin: prompt,
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
	req, err := judgeRequest(ctx, co, c)
	if err != nil {
		cr.JudgeError = err.Error()
		return cr, 0
	}
	v, cost, err := judgeWithRetry(ctx, cfg.Scorer, req, &cr)
	if err != nil {
		cr.JudgeError = fmt.Sprintf("after %d attempts: %v", cr.JudgeAttempts, err)
		return cr, cost
	}
	cr.JudgeScore = &v.Score
	cr.JudgeReasons = nonNil(v.Reasons)
	if cr.DeterministicPass {
		cr.Score = v.Score
	}
	return cr, cost
}

// judgeWithRetry calls the judge up to judgeAttempts times, stopping at
// the first verdict, and returns it with the spend of every attempt.
func judgeWithRetry(ctx context.Context, s CaseScorer, req judge.Request,
	cr *CaseResult,
) (judge.Verdict, float64, error) {
	var cost float64
	for {
		cr.JudgeAttempts++
		v, err := s.Score(ctx, req)
		cost += v.CostUSD
		if err == nil || cr.JudgeAttempts >= judgeAttempts || ctx.Err() != nil {
			return v, cost, err
		}
	}
}

// judgeRequest gathers what the judge grades: both docs before and after,
// and the case's glossary files as they read at the pin.
func judgeRequest(ctx context.Context, co *checkout.Checkout, c cases.Case) (judge.Request, error) {
	req := judge.Request{Component: c.Component}
	for _, p := range judge.TargetFiles(c) {
		req.Files = append(req.Files, judge.FilePair{
			Path: p, Before: co.Original(ctx, p), After: co.AtBranch(ctx, c.Branch, p),
		})
	}
	for _, p := range c.GlossaryFiles() {
		content := co.Original(ctx, p)
		if content == "" {
			return req, fmt.Errorf("glossary %s is missing or empty at the pinned commit", p)
		}
		req.Glossary = append(req.Glossary, judge.Doc{Path: p, Content: content})
	}
	return req, nil
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

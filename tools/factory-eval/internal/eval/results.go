package eval

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
	"github.com/mad01/thismoon/tools/factory-eval/internal/judge"
	"github.com/mad01/thismoon/tools/factory-eval/internal/prereq"
)

// SchemaID names the results document's schema; schema/results.schema.json
// describes it.
const SchemaID = "factory-eval/results/v1"

// Pass-rule thresholds from MAD-402: the factory must reach an equal or
// better judge score, at most half the wall clock, and at most one and a
// half times the tokens of the single agent.
const (
	MaxWallRatio  = 0.5
	MaxTokenRatio = 1.5
)

// Results is the one document a batch writes.
type Results struct {
	Schema        string          `json:"schema"`
	DryRun        bool            `json:"dry_run"`
	StartedAt     time.Time       `json:"started_at"`
	FinishedAt    time.Time       `json:"finished_at"`
	Commit        string          `json:"commit"`
	CasesFile     string          `json:"cases_file"`
	SignOff       cases.SignOff   `json:"sign_off"`
	ClaudeVersion string          `json:"claude_version"`
	Prereqs       []prereq.Result `json:"prereqs"`
	Runs          []RunResult     `json:"runs"`
	Summary       Summary         `json:"summary"`
}

// RunResult is one arm session at one tier, with its cases scored.
type RunResult struct {
	Arm               string          `json:"arm"`
	Tier              string          `json:"tier"`
	Index             int             `json:"run_index"`
	Model             string          `json:"model"`
	Effort            string          `json:"effort"`
	BudgetUSD         float64         `json:"budget_usd"`
	TokensIn          int64           `json:"tokens_in"`
	TokensOut         int64           `json:"tokens_out"`
	TokensTotal       int64           `json:"tokens_total"`
	Tokens            claude.Tokens   `json:"tokens"`
	Dollars           float64         `json:"dollars"`
	JudgeDollars      float64         `json:"judge_dollars"`
	WallClockMS       int64           `json:"wall_clock_ms"`
	NumTurns          int             `json:"num_turns"`
	SessionID         string          `json:"session_id,omitempty"`
	Stop              string          `json:"stop"`
	Error             string          `json:"error,omitempty"`
	PermissionDenials int             `json:"permission_denials"`
	AgentReport       json.RawMessage `json:"agent_report,omitempty"`
	Cases             []CaseResult    `json:"cases"`
	JudgeScore        float64         `json:"judge_score"`
	JudgeReasons      []string        `json:"judge_reasons"`
	Pass              bool            `json:"pass"`
	KeptDir           string          `json:"kept_dir,omitempty"`
}

// CaseResult is one case of one run: its deterministic checks and its
// judge verdict. Score is the judge score when every check passed and 0
// otherwise, so a broken branch never wins on prose alone.
type CaseResult struct {
	Component         string        `json:"component"`
	Branch            string        `json:"branch"`
	Checks            []judge.Check `json:"checks"`
	DeterministicPass bool          `json:"deterministic_pass"`
	JudgeScore        *int          `json:"judge_score"`
	JudgeReasons      []string      `json:"judge_reasons"`
	JudgeError        string        `json:"judge_error,omitempty"`
	Score             int           `json:"score"`
}

// Summary holds per-arm means and the pass decision per tier. Pass is the
// primary tier's decision.
type Summary struct {
	Arms        []ArmStats `json:"arms"`
	Decisions   []Decision `json:"decisions"`
	PrimaryTier string     `json:"primary_tier"`
	Pass        bool       `json:"pass"`
	Decision    string     `json:"decision"`
}

// ArmStats are one arm's means over its runs at one tier.
type ArmStats struct {
	Arm             string  `json:"arm"`
	Tier            string  `json:"tier"`
	Runs            int     `json:"runs"`
	RunsPassed      int     `json:"runs_passed"`
	MeanJudgeScore  float64 `json:"mean_judge_score"`
	MeanWallClockMS float64 `json:"mean_wall_clock_ms"`
	MeanTokens      float64 `json:"mean_tokens"`
	MeanDollars     float64 `json:"mean_dollars"`
}

// Decision applies the pass rule at one tier.
type Decision struct {
	Tier         string  `json:"tier"`
	Complete     bool    `json:"complete"`
	JudgeSingle  float64 `json:"judge_single"`
	JudgeFactory float64 `json:"judge_factory"`
	WallRatio    float64 `json:"wall_ratio"`
	TokenRatio   float64 `json:"token_ratio"`
	JudgeOK      bool    `json:"judge_ok"`
	WallOK       bool    `json:"wall_ok"`
	TokensOK     bool    `json:"tokens_ok"`
	Pass         bool    `json:"pass"`
	Reason       string  `json:"reason"`
}

// Summarize computes the per-arm means and the decisions over runs.
func Summarize(runs []RunResult, tiers []cases.Tier, primary string) Summary {
	s := Summary{PrimaryTier: primary, Arms: []ArmStats{}, Decisions: []Decision{}}
	for _, t := range tiers {
		single := stats(runs, ArmSingle, t.Name)
		factory := stats(runs, ArmFactory, t.Name)
		for _, a := range []ArmStats{single, factory} {
			if a.Runs > 0 {
				s.Arms = append(s.Arms, a)
			}
		}
		d := Decide(t.Name, single, factory)
		s.Decisions = append(s.Decisions, d)
		if t.Name == primary {
			s.Pass, s.Decision = d.Pass, d.Reason
		}
	}
	return s
}

func stats(runs []RunResult, arm, tier string) ArmStats {
	a := ArmStats{Arm: arm, Tier: tier}
	for _, r := range runs {
		if r.Arm != arm || r.Tier != tier {
			continue
		}
		a.Runs++
		if r.Pass {
			a.RunsPassed++
		}
		a.MeanJudgeScore += r.JudgeScore
		a.MeanWallClockMS += float64(r.WallClockMS)
		a.MeanTokens += float64(r.TokensTotal)
		a.MeanDollars += r.Dollars
	}
	if a.Runs > 0 {
		n := float64(a.Runs)
		a.MeanJudgeScore /= n
		a.MeanWallClockMS /= n
		a.MeanTokens /= n
		a.MeanDollars /= n
	}
	return a
}

// Decide applies the MAD-402 pass rule to one tier's arm means. A ratio
// against a zero single-arm mean is undefined and fails its check.
func Decide(tier string, single, factory ArmStats) Decision {
	d := Decision{
		Tier:         tier,
		JudgeSingle:  single.MeanJudgeScore,
		JudgeFactory: factory.MeanJudgeScore,
	}
	if single.Runs == 0 || factory.Runs == 0 {
		d.Reason = fmt.Sprintf("FAIL at tier %s: incomplete, single ran %d and factory ran %d",
			tier, single.Runs, factory.Runs)
		return d
	}
	d.Complete = true
	d.JudgeOK = factory.MeanJudgeScore >= single.MeanJudgeScore
	if single.MeanWallClockMS > 0 {
		d.WallRatio = factory.MeanWallClockMS / single.MeanWallClockMS
		d.WallOK = d.WallRatio <= MaxWallRatio
	}
	if single.MeanTokens > 0 {
		d.TokenRatio = factory.MeanTokens / single.MeanTokens
		d.TokensOK = d.TokenRatio <= MaxTokenRatio
	}
	d.Pass = d.JudgeOK && d.WallOK && d.TokensOK
	verdict := "FAIL"
	if d.Pass {
		verdict = "PASS"
	}
	d.Reason = fmt.Sprintf("%s at tier %s: judge %.1f vs %.1f (needs >=), "+
		"wall %.2fx (needs <=%.1fx), tokens %.2fx (needs <=%.1fx)",
		verdict, tier, d.JudgeFactory, d.JudgeSingle, d.WallRatio, MaxWallRatio,
		d.TokenRatio, MaxTokenRatio)
	return d
}

// WorstCaseUSD is the most a batch can spend: every arm run at its tier's
// cap, plus every judge call at the judge's cap. A paid run refuses to
// start when this exceeds the cases file's batch ceiling.
func WorstCaseUSD(f *cases.File, arms int, tiers []cases.Tier, runs int) float64 {
	total := 0.0
	for _, t := range tiers {
		total += t.BudgetUSD * float64(arms*runs)
	}
	judgeCalls := len(f.Cases) * arms * len(tiers) * runs
	return total + f.Judge.BudgetUSD*float64(judgeCalls)
}

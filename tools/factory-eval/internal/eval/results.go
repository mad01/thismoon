package eval

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
	"github.com/mad01/thismoon/tools/factory-eval/internal/judge"
	"github.com/mad01/thismoon/tools/factory-eval/internal/prereq"
)

// SchemaID names the results document's schema; schema/results.schema.json
// describes it. v2 replaced v1's settings_deny with settings_permissions.
const SchemaID = "factory-eval/results/v2"

// Pass-rule thresholds from MAD-402: the factory must reach an equal or
// better judge score, at most half the wall clock, and at most one and a
// half times the tokens of the single agent.
const (
	MaxWallRatio  = 0.5
	MaxTokenRatio = 1.5
)

// Results is the one document a batch writes. CasesDigest is the cases
// file's content digest, which a signed-off file also holds in its
// sign-off. SettingsPermissions maps each arm to the permissions block
// its runs' settings files held. UnsandboxedCommands lists the user's
// sandbox exclusions the run could not deny, those overlapping an arm
// Bash rule and wrapper words; those commands ran outside the sandbox.
type Results struct {
	Schema              string                 `json:"schema"`
	DryRun              bool                   `json:"dry_run"`
	StartedAt           time.Time              `json:"started_at"`
	FinishedAt          time.Time              `json:"finished_at"`
	Commit              string                 `json:"commit"`
	CasesFile           string                 `json:"cases_file"`
	CasesDigest         string                 `json:"cases_digest"`
	SignOff             cases.SignOff          `json:"sign_off"`
	ClaudeVersion       string                 `json:"claude_version"`
	Prereqs             []prereq.Result        `json:"prereqs"`
	SettingsPermissions map[string]Permissions `json:"settings_permissions"`
	UnsandboxedCommands []string               `json:"unsandboxed_commands"`
	Runs                []RunResult            `json:"runs"`
	Summary             Summary                `json:"summary"`
}

// RunResult is one arm session at one tier, with its cases scored.
// SettingsSandbox is the sandbox block its settings file held, kept here
// because a passing run's checkout, file included, is removed. Installed
// is what the host had installed when the run started. Taint lists how
// the source checkout changed while the arm ran; a tainted run counts as
// errored.
type RunResult struct {
	Arm               string           `json:"arm"`
	Tier              string           `json:"tier"`
	Index             int              `json:"run_index"`
	Model             string           `json:"model"`
	Effort            string           `json:"effort"`
	BudgetUSD         float64          `json:"budget_usd"`
	TokensIn          int64            `json:"tokens_in"`
	TokensOut         int64            `json:"tokens_out"`
	TokensTotal       int64            `json:"tokens_total"`
	Tokens            claude.Tokens    `json:"tokens"`
	Dollars           float64          `json:"dollars"`
	JudgeDollars      float64          `json:"judge_dollars"`
	WallClockMS       int64            `json:"wall_clock_ms"`
	NumTurns          int              `json:"num_turns"`
	SessionID         string           `json:"session_id,omitempty"`
	Stop              string           `json:"stop"`
	Error             string           `json:"error,omitempty"`
	PermissionDenials int              `json:"permission_denials"`
	AgentReport       json.RawMessage  `json:"agent_report,omitempty"`
	SettingsFile      string           `json:"settings_file,omitempty"`
	SettingsSandbox   *Sandbox         `json:"settings_sandbox,omitempty"`
	Installed         prereq.Installed `json:"installed"`
	Tainted           bool             `json:"tainted"`
	Taint             []string         `json:"taint"`
	Cases             []CaseResult     `json:"cases"`
	JudgeScore        float64          `json:"judge_score"`
	JudgeReasons      []string         `json:"judge_reasons"`
	Pass              bool             `json:"pass"`
	KeptDir           string           `json:"kept_dir,omitempty"`
}

// Errored reports whether a run cannot count toward a decision: the
// harness, the session or the judge failed (Error is set, which a result
// with is_error does under subtype success too), the session stopped
// before it finished its turn (a budget stop included), or the run
// changed the source checkout.
func (r RunResult) Errored() bool {
	return r.Error != "" || r.Stop != claude.SubtypeSuccess || r.Tainted
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
	JudgeAttempts     int           `json:"judge_attempts"`
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

// ArmStats are one arm's counts and means at one tier. Runs counts every
// recorded run; the means cover only the runs that did not error.
type ArmStats struct {
	Arm             string  `json:"arm"`
	Tier            string  `json:"tier"`
	Runs            int     `json:"runs"`
	RunsErrored     int     `json:"runs_errored"`
	RunsPassed      int     `json:"runs_passed"`
	MeanJudgeScore  float64 `json:"mean_judge_score"`
	MeanWallClockMS float64 `json:"mean_wall_clock_ms"`
	MeanTokens      float64 `json:"mean_tokens"`
	MeanDollars     float64 `json:"mean_dollars"`
}

// Decision applies the pass rule at one tier.
type Decision struct {
	Tier              string  `json:"tier"`
	ExpectedRuns      int     `json:"expected_runs"`
	Complete          bool    `json:"complete"`
	JudgeSingle       float64 `json:"judge_single"`
	JudgeFactory      float64 `json:"judge_factory"`
	WallRatio         float64 `json:"wall_ratio"`
	TokenRatio        float64 `json:"token_ratio"`
	JudgeOK           bool    `json:"judge_ok"`
	WallOK            bool    `json:"wall_ok"`
	TokensOK          bool    `json:"tokens_ok"`
	FactoryRunsPassed int     `json:"factory_runs_passed"`
	Pass              bool    `json:"pass"`
	Reason            string  `json:"reason"`
}

// Summarize computes the per-arm stats and the decisions over runs, where
// each arm was meant to run expected times per tier.
func Summarize(runs []RunResult, tiers []cases.Tier, primary string, expected int) Summary {
	s := Summary{PrimaryTier: primary, Arms: []ArmStats{}, Decisions: []Decision{}}
	for _, t := range tiers {
		single := stats(runs, ArmSingle, t.Name)
		factory := stats(runs, ArmFactory, t.Name)
		for _, a := range []ArmStats{single, factory} {
			if a.Runs > 0 {
				s.Arms = append(s.Arms, a)
			}
		}
		d := Decide(t.Name, expected, single, factory)
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
		if r.Errored() {
			a.RunsErrored++
			continue
		}
		if r.Pass {
			a.RunsPassed++
		}
		a.MeanJudgeScore += r.JudgeScore
		a.MeanWallClockMS += float64(r.WallClockMS)
		a.MeanTokens += float64(r.TokensTotal)
		a.MeanDollars += r.Dollars
	}
	if clean := a.Runs - a.RunsErrored; clean > 0 {
		n := float64(clean)
		a.MeanJudgeScore /= n
		a.MeanWallClockMS /= n
		a.MeanTokens /= n
		a.MeanDollars /= n
	}
	return a
}

// Decide applies the MAD-402 pass rule at one tier. The tier must be
// complete: each arm recorded all expected runs and none errored. Then
// the factory passes only when the single arm set a baseline (a mean judge
// score above 0), at least one factory run passed all five checks on every
// case, and the factory's means meet the three thresholds. A ratio against
// a zero single-arm mean is undefined and fails its check.
func Decide(tier string, expected int, single, factory ArmStats) Decision {
	d := Decision{
		Tier: tier, ExpectedRuns: expected, FactoryRunsPassed: factory.RunsPassed,
		JudgeSingle: single.MeanJudgeScore, JudgeFactory: factory.MeanJudgeScore,
	}
	if !complete(single, expected) || !complete(factory, expected) {
		d.Reason = fmt.Sprintf("FAIL at tier %s: incomplete; %s; %s", tier,
			cleanRuns(single, expected), cleanRuns(factory, expected))
		return d
	}
	d.Complete = true
	d.JudgeOK = single.MeanJudgeScore > 0 && factory.MeanJudgeScore >= single.MeanJudgeScore
	d.WallRatio, d.WallOK = ratio(factory.MeanWallClockMS, single.MeanWallClockMS, MaxWallRatio)
	d.TokenRatio, d.TokensOK = ratio(factory.MeanTokens, single.MeanTokens, MaxTokenRatio)
	d.Pass = d.JudgeOK && d.WallOK && d.TokensOK && factory.RunsPassed > 0
	d.Reason = decisionReason(d, single)
	return d
}

func complete(a ArmStats, expected int) bool {
	return expected > 0 && a.Runs == expected && a.RunsErrored == 0
}

func cleanRuns(a ArmStats, expected int) string {
	return fmt.Sprintf("%s has %d of %d runs without error", a.Arm, a.Runs-a.RunsErrored, expected)
}

// ratio is factory over single and whether it is at most limit. A zero
// single mean leaves the ratio undefined: 0 and failed.
func ratio(factory, single, limit float64) (float64, bool) {
	if single <= 0 {
		return 0, false
	}
	r := factory / single
	return r, r <= limit
}

func decisionReason(d Decision, single ArmStats) string {
	verdict := "FAIL"
	if d.Pass {
		verdict = "PASS"
	}
	var parts []string
	if single.MeanJudgeScore <= 0 {
		parts = append(parts, "the single arm's mean judge score is 0, no baseline to match")
	}
	if d.FactoryRunsPassed == 0 {
		parts = append(parts, "no factory run passed all five checks")
	}
	parts = append(parts,
		fmt.Sprintf("judge %.1f vs %.1f (needs >=)", d.JudgeFactory, d.JudgeSingle),
		ratioText("wall", d.WallRatio, single.MeanWallClockMS, MaxWallRatio),
		ratioText("tokens", d.TokenRatio, single.MeanTokens, MaxTokenRatio))
	return fmt.Sprintf("%s at tier %s: %s", verdict, d.Tier, strings.Join(parts, "; "))
}

func ratioText(name string, r, single, limit float64) string {
	if single <= 0 {
		return name + " undefined, the single arm's mean is 0"
	}
	return fmt.Sprintf("%s %.2fx (needs <=%.1fx)", name, r, limit)
}

// WorstCaseUSD is the most a batch can spend: every arm run at its tier's
// cap, plus every judge call at the judge's cap, counting the retry each
// case's judge may make. A paid run refuses to start when this exceeds the
// cases file's batch ceiling.
func WorstCaseUSD(f *cases.File, arms int, tiers []cases.Tier, runs int) float64 {
	total := 0.0
	for _, t := range tiers {
		total += t.BudgetUSD * float64(arms*runs)
	}
	judgeCalls := len(f.Cases) * arms * len(tiers) * runs * judgeAttempts
	return total + f.Judge.BudgetUSD*float64(judgeCalls)
}

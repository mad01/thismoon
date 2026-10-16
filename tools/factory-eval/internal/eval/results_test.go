package eval

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/claude"
)

// armStats is a complete arm at tier mid: three clean runs, all passed.
func armStats(arm string, judge, wallMS, tokens float64) ArmStats {
	return ArmStats{
		Arm: arm, Tier: "mid", Runs: 3, RunsPassed: 3,
		MeanJudgeScore: judge, MeanWallClockMS: wallMS, MeanTokens: tokens,
	}
}

func TestDecideAppliesTheTicketsPassRule(t *testing.T) {
	single := armStats(ArmSingle, 7, 60_000, 1_000_000)
	tests := []struct {
		name    string
		factory ArmStats
		pass    bool
		failing string
	}{
		{
			"equal judge, half the wall, 1.5x tokens",
			armStats(ArmFactory, 7, 30_000, 1_500_000),
			true,
			"",
		},
		{"better on every axis", armStats(ArmFactory, 9, 10_000, 500_000), true, ""},
		{"lower judge", armStats(ArmFactory, 6.9, 10_000, 500_000), false, "judge"},
		{"wall only 0.6x", armStats(ArmFactory, 8, 36_000, 900_000), false, "wall"},
		{"tokens 1.6x", armStats(ArmFactory, 8, 20_000, 1_600_000), false, "tokens"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide("mid", 3, single, tt.factory)
			if d.Pass != tt.pass || !d.Complete {
				t.Fatalf("Decide() = %+v, want pass=%v", d, tt.pass)
			}
			failed := map[string]bool{"judge": !d.JudgeOK, "wall": !d.WallOK, "tokens": !d.TokensOK}
			for axis, isFailed := range failed {
				if isFailed != (axis == tt.failing) {
					t.Errorf("%s failed = %v, want only %q to fail", axis, isFailed, tt.failing)
				}
			}
			prefix := map[bool]string{true: "PASS", false: "FAIL"}[tt.pass]
			if !strings.HasPrefix(d.Reason, prefix) {
				t.Errorf("Reason = %q, want it to start with %s", d.Reason, prefix)
			}
		})
	}
}

// TestDecideNeverPassesOnNothing covers the false PASS a probe found: both
// judge means 0 and a factory run that did no work printed a PASS.
func TestDecideNeverPassesOnNothing(t *testing.T) {
	idle := ArmStats{Arm: ArmFactory, Tier: "mid", Runs: 3}
	tests := []struct {
		name            string
		single, factory ArmStats
		want, dont      []string
	}{
		{
			"0 vs 0 with no measurements",
			ArmStats{Arm: ArmSingle, Tier: "mid", Runs: 3},
			idle,
			[]string{
				"FAIL at tier mid", "no baseline", "no factory run passed all five checks",
				"wall undefined, the single arm's mean is 0", "tokens undefined",
			},
			[]string{"0.00x"},
		},
		{
			"0 vs 0 with real wall and tokens", armStats(ArmSingle, 0, 60_000, 1_000_000),
			armStats(ArmFactory, 0, 10_000, 500_000),
			[]string{"FAIL at tier mid", "no baseline", "judge 0.0 vs 0.0"},
			nil,
		},
		{
			"no factory run passed", armStats(ArmSingle, 7, 60_000, 1_000_000),
			ArmStats{
				Arm: ArmFactory, Tier: "mid", Runs: 3,
				MeanJudgeScore: 8, MeanWallClockMS: 10_000, MeanTokens: 500_000,
			},
			[]string{"FAIL", "no factory run passed all five checks"},
			[]string{"no baseline"},
		},
		{
			"zero single wall clock", armStats(ArmSingle, 7, 0, 1_000_000),
			armStats(ArmFactory, 8, 10_000, 500_000),
			[]string{"FAIL", "wall undefined, the single arm's mean is 0"},
			[]string{"wall 0.00x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide("mid", 3, tt.single, tt.factory)
			if d.Pass || !d.Complete {
				t.Fatalf("Decide() = %+v, want a complete failure", d)
			}
			for _, w := range tt.want {
				if !strings.Contains(d.Reason, w) {
					t.Errorf("Reason = %q, want it to contain %q", d.Reason, w)
				}
			}
			for _, w := range tt.dont {
				if strings.Contains(d.Reason, w) {
					t.Errorf("Reason = %q, want it free of %q", d.Reason, w)
				}
			}
		})
	}
}

func TestDecideFailsAnIncompleteTier(t *testing.T) {
	good := armStats(ArmFactory, 9, 10_000, 500_000)
	single := armStats(ArmSingle, 7, 60_000, 1_000_000)
	short := good
	short.Runs = 2
	errored := single
	errored.RunsErrored = 1
	tests := []struct {
		name            string
		single, factory ArmStats
		want            string
	}{
		{"factory ran 2 of 3", single, short, "factory has 2 of 3 runs without error"},
		{"one single run errored", errored, good, "single has 2 of 3 runs without error"},
		{"factory never ran", single, ArmStats{Arm: ArmFactory}, "factory has 0 of 3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := Decide("mid", 3, tt.single, tt.factory)
			if d.Pass || d.Complete || !strings.Contains(d.Reason, "incomplete") ||
				!strings.Contains(d.Reason, tt.want) {
				t.Errorf("Decide() = %+v, want an incomplete failure naming %q", d, tt.want)
			}
		})
	}
	if d := Decide("mid", 0, single, good); d.Pass || d.Complete {
		t.Errorf("Decide(expected 0) = %+v, want an incomplete failure", d)
	}
}

func ok(arm, tier string, judge float64, wallMS, tokens int64) RunResult {
	return RunResult{
		Arm: arm, Tier: tier, Stop: claude.SubtypeSuccess, Pass: true,
		JudgeScore: judge, WallClockMS: wallMS, TokensTotal: tokens,
	}
}

func TestStatsLeaveErroredRunsOutOfTheMeans(t *testing.T) {
	budget := ok(ArmSingle, "mid", 0, 90_000, 9_000)
	budget.Stop, budget.Pass = claude.SubtypeMaxBudget, false
	tainted := ok(ArmSingle, "mid", 9, 1_000, 1_000)
	tainted.Tainted, tainted.Pass = true, false
	crashed := ok(ArmSingle, "mid", 0, 5_000, 0)
	crashed.Error, crashed.Pass = "claude: run stopped", false
	runs := []RunResult{ok(ArmSingle, "mid", 6, 60_000, 1000), budget, tainted, crashed}
	a := stats(runs, ArmSingle, "mid")
	if a.Runs != 4 || a.RunsErrored != 3 || a.RunsPassed != 1 {
		t.Errorf("counts = %+v, want 4 runs, 3 errored, 1 passed", a)
	}
	if a.MeanJudgeScore != 6 || a.MeanWallClockMS != 60_000 || a.MeanTokens != 1000 {
		t.Errorf("means = %+v, want the one clean run's numbers", a)
	}
}

func TestSummarizeReportsThePrimaryTier(t *testing.T) {
	runs := []RunResult{
		ok(ArmSingle, "mid", 7, 60_000, 1000),
		ok(ArmFactory, "mid", 8, 20_000, 1200),
		ok(ArmSingle, "top", 9, 60_000, 1000),
		ok(ArmFactory, "top", 5, 20_000, 1200),
	}
	tiers := []cases.Tier{{Name: "mid"}, {Name: "top"}}
	s := Summarize(runs, tiers, "mid", 1)
	if !s.Pass || !strings.Contains(s.Decision, "tier mid") || len(s.Decisions) != 2 {
		t.Errorf("Summarize() = %+v, want the mid tier's pass as the headline", s)
	}
	if s.Decisions[1].Pass {
		t.Errorf("top decision = %+v, want a failure on judge score", s.Decisions[1])
	}
	if len(s.Arms) != 4 || s.Arms[0].RunsPassed != 1 {
		t.Errorf("Arms = %+v, want four arm rows", s.Arms)
	}
	runs[1].Tainted = true
	if s := Summarize(runs, tiers, "mid", 1); s.Pass ||
		!strings.Contains(s.Decision, "incomplete") {
		t.Errorf("Summarize(tainted factory run) = %+v, want an incomplete failure", s)
	}
}

func TestWorstCaseCountsEveryArmRunAndJudgeCall(t *testing.T) {
	f := &cases.File{Judge: cases.Judge{BudgetUSD: 0.25}, Cases: make([]cases.Case, 4)}
	tiers := []cases.Tier{{BudgetUSD: 10}, {BudgetUSD: 25}}
	// 3 runs x 2 arms x ($10 + $25) = $210, plus 4 cases x 2 arms x 2 tiers x 3 runs
	// x 2 judge attempts x $0.25 = $24.
	if got := WorstCaseUSD(f, 2, tiers, 3); got != 234 {
		t.Errorf("WorstCaseUSD() = %v, want 234", got)
	}
}

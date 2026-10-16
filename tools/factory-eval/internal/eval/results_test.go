package eval

import (
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
)

func armStats(arm string, judge, wallMS, tokens float64) ArmStats {
	return ArmStats{
		Arm: arm, Tier: "mid", Runs: 3,
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
			d := Decide("mid", single, tt.factory)
			if d.Pass != tt.pass || !d.Complete {
				t.Fatalf("Decide() = %+v, want pass=%v", d, tt.pass)
			}
			failed := map[string]bool{"judge": !d.JudgeOK, "wall": !d.WallOK, "tokens": !d.TokensOK}
			for axis, isFailed := range failed {
				if isFailed != (axis == tt.failing) {
					t.Errorf("%s failed = %v, want only %q to fail", axis, isFailed, tt.failing)
				}
			}
			if prefix := map[bool]string{true: "PASS", false: "FAIL"}[tt.pass]; !strings.HasPrefix(
				d.Reason,
				prefix,
			) {
				t.Errorf("Reason = %q, want it to start with %s", d.Reason, prefix)
			}
		})
	}
}

func TestDecideFailsWithoutBothArmsOrMeasurements(t *testing.T) {
	single := armStats(ArmSingle, 7, 60_000, 1_000_000)
	if d := Decide("mid", single, ArmStats{}); d.Pass || d.Complete ||
		!strings.Contains(d.Reason, "incomplete") {
		t.Errorf("Decide(no factory runs) = %+v, want an incomplete failure", d)
	}
	zero := armStats(ArmSingle, 0, 0, 0)
	if d := Decide("mid", zero, armStats(ArmFactory, 0, 0, 0)); d.Pass {
		t.Errorf("Decide(zero measurements) = %+v, want a failure, not a division by zero", d)
	}
}

func TestSummarizeReportsThePrimaryTier(t *testing.T) {
	runs := []RunResult{
		{
			Arm:         ArmSingle,
			Tier:        "mid",
			JudgeScore:  7,
			WallClockMS: 60_000,
			TokensTotal: 1000,
			Pass:        true,
		},
		{
			Arm:         ArmFactory,
			Tier:        "mid",
			JudgeScore:  8,
			WallClockMS: 20_000,
			TokensTotal: 1200,
			Pass:        true,
		},
		{Arm: ArmSingle, Tier: "top", JudgeScore: 9, WallClockMS: 60_000, TokensTotal: 1000},
		{Arm: ArmFactory, Tier: "top", JudgeScore: 5, WallClockMS: 20_000, TokensTotal: 1200},
	}
	tiers := []cases.Tier{{Name: "mid"}, {Name: "top"}}
	s := Summarize(runs, tiers, "mid")
	if !s.Pass || !strings.Contains(s.Decision, "tier mid") || len(s.Decisions) != 2 {
		t.Errorf("Summarize() = %+v, want the mid tier's pass as the headline", s)
	}
	if s.Decisions[1].Pass {
		t.Errorf("top decision = %+v, want a failure on judge score", s.Decisions[1])
	}
	if len(s.Arms) != 4 || s.Arms[0].RunsPassed != 1 {
		t.Errorf("Arms = %+v, want four arm rows", s.Arms)
	}
}

func TestWorstCaseCountsEveryArmRunAndJudgeCall(t *testing.T) {
	f := &cases.File{Judge: cases.Judge{BudgetUSD: 0.25}, Cases: make([]cases.Case, 4)}
	tiers := []cases.Tier{{BudgetUSD: 10}, {BudgetUSD: 25}}
	// 3 runs x 2 arms x ($10 + $25) = $210, plus 4 cases x 2 arms x 2 tiers x 3 runs x $0.25 = $12.
	if got := WorstCaseUSD(f, 2, tiers, 3); got != 222 {
		t.Errorf("WorstCaseUSD() = %v, want 222", got)
	}
}

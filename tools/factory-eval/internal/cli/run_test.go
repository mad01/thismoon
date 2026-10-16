package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/eval"
	"github.com/mad01/thismoon/tools/factory-eval/internal/prereq"
)

func signedBatch(t *testing.T) batch {
	t.Helper()
	f, err := cases.Load("../../cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f.SignOff = cases.SignOff{By: "someone", Date: "2026-10-17"}
	return batch{
		file: f, arms: []string{eval.ArmSingle, eval.ArmFactory}, tiers: f.Tiers, runs: f.Runs,
		common:  []prereq.Result{{Name: "git", OK: true}},
		factory: []prereq.Result{{Name: "factory skill", OK: true}},
	}
}

func TestGateLetsASignedReadyBatchThrough(t *testing.T) {
	if err := gate(signedBatch(t)); err != nil {
		t.Errorf("gate() = %v, want nil", err)
	}
}

func TestGateRefusesBeforeAnySpend(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*batch)
		want   string
	}{
		{"unsigned", func(b *batch) { b.file.SignOff = cases.SignOff{} }, "not signed off"},
		{"missing tool", func(b *batch) {
			b.common = []prereq.Result{{Name: "humanizer", Detail: "not on PATH"}}
		}, "humanizer: not on PATH"},
		{"factory not installed", func(b *batch) {
			b.factory = []prereq.Result{
				{Name: "worker definition factory-worker-medium", Detail: "not found"},
			}
		}, "the factory arm cannot run"},
		{"over the ceiling", func(b *batch) { b.runs = 10 }, "exceeds the batch ceiling"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := signedBatch(t)
			tt.mutate(&b)
			err := gate(b)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("gate() = %v, want an error containing %q", err, tt.want)
			}
		})
	}
}

func TestPrintPlanShowsWhatAPaidRunWouldRefuse(t *testing.T) {
	b := signedBatch(t)
	b.file.SignOff = cases.SignOff{}
	b.factory = []prereq.Result{{Name: "belt SubagentStart hook", Detail: "no hook runs belt"}}
	var out bytes.Buffer
	printPlan(&out, b, true)
	for _, want := range []string{
		"dry run, no model call", "NOT SIGNED OFF", "FAIL belt SubagentStart hook",
		"the factory arm cannot run", "a paid run refuses here", "worst case: $222.00",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("plan lacks %q:\n%s", want, out.String())
		}
	}
}

func TestSelectTiersAndArms(t *testing.T) {
	b := signedBatch(t)
	tiers, err := selectTiers(b.file, []string{"top"})
	if err != nil || len(tiers) != 1 || tiers[0].Model != "opus" {
		t.Errorf("selectTiers(top) = %+v, %v", tiers, err)
	}
	if _, err := selectTiers(b.file, []string{"huge"}); err == nil {
		t.Error("selectTiers(huge) = nil error")
	}
	if _, err := parseArms([]string{"single", "swarm"}); err == nil {
		t.Error("parseArms(swarm) = nil error")
	}
}

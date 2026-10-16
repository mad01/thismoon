package cli

import (
	"bytes"
	"os"
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
	f.SignOff = cases.SignOff{By: "someone", Date: "2026-10-17", Digest: f.Digest()}
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
		{"blank by", func(b *batch) { b.file.SignOff.By = " " }, "sign_off.by is blank"},
		{"bad date", func(b *batch) { b.file.SignOff.Date = "Oct 17" }, "is not YYYY-MM-DD"},
		{"task edited after sign-off", func(b *batch) {
			b.file = editedAfterSignOff(t, b.file.SignOff)
		}, "the file changed after the sign-off"},
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

// editedAfterSignOff loads cases.yaml with one task reworded and the
// sign-off of the unedited file.
func editedAfterSignOff(t *testing.T, s cases.SignOff) *cases.File {
	t.Helper()
	raw, err := os.ReadFile("../../cases.yaml")
	if err != nil {
		t.Fatal(err)
	}
	f, err := cases.Parse([]byte(strings.Replace(string(raw),
		"Move and reword text only.", "Reword freely.", 1)))
	if err != nil {
		t.Fatal(err)
	}
	f.SignOff = s
	return f
}

func TestRunsMustBeAtLeastOne(t *testing.T) {
	for _, tt := range []struct {
		name string
		o    runOptions
		want int
	}{
		{"cases file default", runOptions{}, 3},
		{"dry run default", runOptions{dryRun: true}, 1},
		{"explicit", runOptions{runs: 2, runsSet: true}, 2},
		{"explicit in a dry run", runOptions{runs: 2, runsSet: true, dryRun: true}, 2},
	} {
		if got, err := resolveRuns(tt.o, 3); err != nil || got != tt.want {
			t.Errorf("%s: resolveRuns() = %d, %v; want %d", tt.name, got, err, tt.want)
		}
	}
	for _, n := range []int{0, -1} {
		_, err := resolveRuns(runOptions{runs: n, runsSet: true}, 3)
		if err == nil || !strings.Contains(err.Error(), "want at least 1") {
			t.Errorf("resolveRuns(--runs %d) = %v, want a refusal", n, err)
		}
	}
}

func TestTempDirMustSitOutsideTheProtectedPaths(t *testing.T) {
	iso := eval.Isolation{Home: "/Users/me", Source: "/Users/me/code/src/github.com/mad01/thismoon"}
	if err := checkTempDir("/Users/me/code/tmp", iso); err == nil {
		t.Error("checkTempDir(~/code/tmp) = nil, want a refusal")
	}
	if err := checkTempDir("/tmp/runs", iso); err != nil {
		t.Errorf("checkTempDir(/tmp/runs) = %v, want nil", err)
	}
}

func TestPrintPlanShowsWhatAPaidRunWouldRefuse(t *testing.T) {
	b := signedBatch(t)
	b.file.SignOff = cases.SignOff{}
	b.factory = []prereq.Result{{Name: "belt SubagentStart hook", Detail: "no hook runs belt"}}
	var out bytes.Buffer
	printPlan(&out, b, true)
	for _, want := range []string{
		"dry run, no model call", "NOT SIGNED OFF", "sign_off.by is blank",
		"sign-off digest (sign_off.digest must hold it): " + b.file.Digest(),
		"FAIL belt SubagentStart hook",
		"the factory arm cannot run", "a paid run refuses here", "worst case: $234.00",
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

func TestPrintPlanConfirmsAValidSignOff(t *testing.T) {
	var out bytes.Buffer
	printPlan(&out, signedBatch(t), true)
	if !strings.Contains(out.String(), "signed off by someone on 2026-10-17, digest matches") {
		t.Errorf("plan does not confirm the sign-off:\n%s", out.String())
	}
}

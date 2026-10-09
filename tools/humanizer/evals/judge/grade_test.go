package main

import (
	"math"
	"testing"
)

func row(id, label, variant string, rep int, verdict string, conf float64) Row {
	c := Case{ID: id, Label: label, Bucket: "human"}
	if label == "unknown" {
		c.Bucket = "ambiguous"
	}
	return Row{
		ID: id, Label: label, Bucket: c.Bucket, Variant: variant, Rep: rep,
		Verdict: verdict, Confidence: conf, Grade: grade(c, verdict), LatencyMS: int64(100 * (rep + 1)),
	}
}

func TestSummarize(t *testing.T) {
	arm := Arm{ID: "a"}
	rows := []Row{
		row("ai-1", "likely_ai", "a", 0, "likely_ai", 0.80),
		row("ai-1", "likely_ai", "a", 1, "likely_ai", 0.70),
		row("ai-1", "likely_ai", "a", 2, "mixed", 0.40),
		row("human-1", "likely_human", "a", 0, "likely_human", 0.50),
		row("human-1", "likely_human", "a", 1, "likely_human", 0.50),
		row("human-1", "likely_human", "a", 2, "likely_human", 0.50),
		row("amb-1", "unknown", "a", 0, "likely_ai", 0.90),
		row("other", "likely_ai", "b", 0, "likely_human", 0.90), // another arm, ignored
	}
	errs := []ErrRow{{Variant: "a", Class: "timeout"}, {Variant: "b", Class: "timeout"}}
	st, cs := summarize(arm, rows, errs)
	approx := func(name string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 1e-6 {
			t.Errorf("%s = %v, want %v (%s)", name, got, want, st)
		}
	}
	if st.Rows != 7 || st.Errors != 1 || st.ScoredRows != 6 || st.ScoredCases != 2 {
		t.Errorf(
			"counts = rows %d errors %d scored rows %d cases %d",
			st.Rows,
			st.Errors,
			st.ScoredRows,
			st.ScoredCases,
		)
	}
	approx("accuracy", st.Accuracy, 5.0/6)
	approx("recall", st.RecallAI, 2.0/3)
	approx("specificity", st.SpecificityHuman, 1)
	approx("fpr", st.FPR, 0)
	approx("mixed", st.MixedRate, 1.0/6)
	approx("flip", st.FlipRate, 0.5)
	approx("agreement", st.Agreement, (2.0/3+1)/2)
	_, sdAI := meanSD([]float64{0.80, 0.70, 0.40})
	approx("conf sd", st.ConfSDPoints, sdAI*100/2)
	if st.LatencyP50 != 200 || st.LatencyP95 != 300 {
		t.Errorf("latency p50 %d p95 %d", st.LatencyP50, st.LatencyP95)
	}
	if c := cs["ai-1"]; c.Majority != "likely_ai" || c.Agree != 2 || !c.Correct() {
		t.Errorf("ai-1 = %+v", c)
	}
	if c := cs["amb-1"]; c.Correct() {
		t.Errorf("ambiguous case must never count as correct: %+v", c)
	}
}

func TestSummarizeEmptyArm(t *testing.T) {
	st, _ := summarize(Arm{ID: "empty"}, nil, nil)
	if !math.IsNaN(st.Accuracy) || !math.IsNaN(st.FlipRate) {
		t.Errorf("empty arm should report NaN rates, got %s", st)
	}
	if pct(st.Accuracy) != "n/a" {
		t.Errorf("pct(NaN) = %q", pct(st.Accuracy))
	}
}

func TestGradeMixedIsMiss(t *testing.T) {
	c := Case{ID: "x", Label: "likely_ai", Bucket: "ai"}
	if g := grade(c, "mixed"); g["correct"] != 0 {
		t.Errorf("mixed graded %v, want 0", g)
	}
	if g := grade(c, "likely_ai"); g["correct"] != 1 {
		t.Errorf("match graded %v, want 1", g)
	}
	if g := grade(Case{Label: "unknown", Bucket: "ambiguous"}, "likely_ai"); g != nil {
		t.Errorf("ambiguous graded %v, want nil", g)
	}
}

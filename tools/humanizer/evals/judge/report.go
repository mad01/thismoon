package main

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

const metricNotes = `Accuracy: share of scored rows whose verdict equals the label; mixed counts as a miss.
Recall AI: likely_ai on AI-labeled rows. Spec. human: likely_human on human-labeled rows.
FPR: likely_ai on human-labeled rows. Mixed: share of scored rows answering mixed.
Retried: rows whose first reply broke the JSON contract and the judge's one retry
produced the verdict. Flip rate: share of scored cases whose reps disagree on the verdict. Agreement: mean share
of reps matching the case majority. Conf sd: mean per-case standard deviation of
confidence, in points of 100. Latency: the final HTTP call only. Tokens: mean per row,
summed over every HTTP call the row made; think tok is the reasoning share of out tok
where the provider reports it. Cost: recorded token usage at the model's list price. Rows that produced no verdict are in errors.jsonl and
never in these numbers.`

const armHeader = "| arm | model | temp | effort | rows | err | retried | accuracy | recall AI " +
	"| spec. human | FPR | mixed | flip rate | agreement | conf sd | p50 ms | p95 ms " +
	"| in tok | out tok | think tok | cost |"

var armRule = strings.Repeat("|---", 21) + "|"

// armRow renders one arm's headline as a markdown table row, in armHeader's
// column order.
func armRow(st ArmStats) string {
	cells := []string{
		st.Arm.ID, st.Arm.Model, tempCell(st.Arm), orDash(st.Arm.Effort),
		strconv.Itoa(st.Rows), strconv.Itoa(st.Errors), strconv.Itoa(st.Retried),
		pct(st.Accuracy), pct(st.RecallAI), pct(st.SpecificityHuman), pct(st.FPR),
		pct(st.MixedRate), pct(st.FlipRate), pct(st.Agreement), pts(st.ConfSDPoints),
		strconv.FormatInt(st.LatencyP50, 10), strconv.FormatInt(st.LatencyP95, 10),
		fmt.Sprintf("%.0f", st.MeanPromptTokens), fmt.Sprintf("%.0f", st.MeanCompletionTokens),
		fmt.Sprintf("%.0f", st.MeanReasoningTokens), fmt.Sprintf("$%.3f", st.CostUSD),
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

// writeSummary renders summary.md: the arm table, the per-case grid, the
// ambiguous bucket, and the error classes.
func writeSummary(
	w io.Writer,
	runID string,
	arms []Arm,
	cases []Case,
	rows []Row,
	errs []ErrRow,
	reps int,
) {
	counts := map[string]int{}
	for _, c := range cases {
		counts[c.Bucket]++
	}
	splits := map[string]int{}
	for _, c := range cases {
		splits[c.Split]++
	}
	fmt.Fprintf(w, "# Judge eval: %s\n\n", runID)
	fmt.Fprintf(w, "Splits: %d train, %d test. ", splits["train"], splits["test"])
	fmt.Fprintf(w, "%d cases (%d scored: %d human, %d ai, %d hard; %d ambiguous, unscored), ",
		len(cases), len(cases)-counts["ambiguous"], counts["human"], counts["ai"], counts["hard"],
		counts["ambiguous"])
	fmt.Fprintf(
		w,
		"%d reps, %d arms, %d rows, %d errors.\n\n",
		reps,
		len(arms),
		len(rows),
		len(errs),
	)

	fmt.Fprintln(w, "## Arms")
	fmt.Fprintln(w)
	fmt.Fprintln(w, armHeader)
	fmt.Fprintln(w, armRule)
	perArm := map[string]map[string]CaseStats{}
	for _, arm := range arms {
		st, cs := summarize(arm, rows, errs)
		perArm[arm.ID] = cs
		fmt.Fprintln(w, armRow(st))
	}
	fmt.Fprintf(w, "\n%s\n\n", metricNotes)

	writeSplitTable(w, arms, rows, errs)

	writeCaseGrid(w, "Per case (scored)", arms, cases, perArm, true)
	writeCaseGrid(w, "Ambiguous bucket (unscored)", arms, cases, perArm, false)
	writeErrors(w, errs)
}

// writeSplitTable repeats the quality columns per split, so a rubric tuned
// on the train split shows its held-out number beside it.
func writeSplitTable(w io.Writer, arms []Arm, rows []Row, errs []ErrRow) {
	fmt.Fprintln(w, "## By split")
	fmt.Fprintln(w)
	fmt.Fprintln(
		w,
		"| arm | split | scored rows | accuracy | recall AI | spec. human | FPR | mixed | flip rate | conf sd |",
	)
	fmt.Fprintln(w, "|---|---|---|---|---|---|---|---|---|---|")
	for _, arm := range arms {
		for _, split := range []string{"train", "test"} {
			var subset []Row
			for _, r := range rows {
				if r.Split == split {
					subset = append(subset, r)
				}
			}
			st, _ := summarize(arm, subset, errs)
			if st.Rows == 0 {
				continue
			}
			fmt.Fprintf(w, "| %s | %s | %d | %s | %s | %s | %s | %s | %s | %s |\n",
				arm.ID, split, st.ScoredRows, pct(st.Accuracy), pct(st.RecallAI),
				pct(st.SpecificityHuman), pct(st.FPR), pct(st.MixedRate), pct(st.FlipRate),
				pts(st.ConfSDPoints))
		}
	}
	fmt.Fprintln(w)
}

// writeCaseGrid prints one row per case with a cell per arm: majority
// verdict, agreeing reps over reps, mean confidence and its spread, and a
// cross when a scored majority misses the label.
func writeCaseGrid(
	w io.Writer,
	title string,
	arms []Arm,
	cases []Case,
	perArm map[string]map[string]CaseStats,
	scored bool,
) {
	fmt.Fprintf(w, "## %s\n\n", title)
	fmt.Fprint(w, "| case | bucket | label |")
	for _, a := range arms {
		fmt.Fprintf(w, " %s |", a.ID)
	}
	fmt.Fprint(w, "\n|---|---|---|")
	for range arms {
		fmt.Fprint(w, "---|")
	}
	fmt.Fprintln(w)
	for _, c := range cases {
		if c.Scored() != scored {
			continue
		}
		fmt.Fprintf(w, "| %s | %s/%s | %s |", c.ID, c.Bucket, c.Split, c.Label)
		for _, a := range arms {
			fmt.Fprintf(w, " %s |", caseCell(perArm[a.ID][c.ID], scored))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
}

func caseCell(s CaseStats, scored bool) string {
	if s.Reps == 0 {
		return "no rows"
	}
	mark := ""
	if scored && !s.Correct() {
		mark = "✗ "
	}
	return fmt.Sprintf("%s%s %d/%d %.0f±%.0f", mark, s.Majority, s.Agree, s.Reps,
		s.ConfMean*100, s.ConfSD*100)
}

func writeErrors(w io.Writer, errs []ErrRow) {
	if len(errs) == 0 {
		return
	}
	fmt.Fprintln(w, "## Errors")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| arm | class | count | example |")
	fmt.Fprintln(w, "|---|---|---|---|")
	type key struct{ arm, class string }
	counts := map[key]int{}
	example := map[key]string{}
	var order []key
	for _, e := range errs {
		k := key{e.Variant, e.Class}
		if counts[k] == 0 {
			order = append(order, k)
			example[k] = e.Error
		}
		counts[k]++
	}
	for _, k := range order {
		fmt.Fprintf(w, "| %s | %s | %d | %s |\n", k.arm, k.class, counts[k], cell(example[k]))
	}
	fmt.Fprintln(w)
}

func tempCell(a Arm) string {
	if a.Temperature == nil {
		return "default"
	}
	return fmt.Sprintf("%g", *a.Temperature)
}

func orDash(s string) string {
	if s == "" {
		return "default"
	}
	return s
}

func pct(x float64) string {
	if math.IsNaN(x) {
		return "n/a"
	}
	return fmt.Sprintf("%.0f%%", x*100)
}

func pts(x float64) string {
	if math.IsNaN(x) {
		return "n/a"
	}
	return fmt.Sprintf("%.1f", x)
}

// String keeps ArmStats readable in test failures.
func (s ArmStats) String() string {
	return strings.TrimSpace(
		fmt.Sprintf("%s acc=%s recall=%s spec=%s fpr=%s flip=%s agree=%s sd=%s",
			s.Arm.ID, pct(s.Accuracy), pct(s.RecallAI), pct(s.SpecificityHuman), pct(s.FPR),
			pct(s.FlipRate), pct(s.Agreement), pts(s.ConfSDPoints)),
	)
}

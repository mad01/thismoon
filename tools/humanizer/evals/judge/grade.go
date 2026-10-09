package main

import (
	"math"
	"slices"
	"strings"
)

// grade scores one verdict against the case label. Scored cases get a
// correct bit; mixed is a miss (the skill treats it as a rewrite target, so
// it must not pass as agreement). Ambiguous cases get no grade.
func grade(c Case, verdict string) map[string]float64 {
	if !c.Scored() {
		return nil
	}
	correct := 0.0
	if verdict == c.Label {
		correct = 1
	}
	return map[string]float64{"correct": correct}
}

// ArmStats is the per-arm headline. Rates are NaN when their denominator
// is empty, and the report prints those as n/a rather than as zero.
type ArmStats struct {
	Arm                  Arm
	Rows                 int
	Errors               int
	ScoredRows           int
	ScoredCases          int
	Accuracy             float64
	RecallAI             float64
	SpecificityHuman     float64
	FPR                  float64
	MixedRate            float64
	FlipRate             float64
	Agreement            float64
	ConfSDPoints         float64
	LatencyP50           int64
	LatencyP95           int64
	MeanPromptTokens     float64
	MeanCompletionTokens float64
	MeanReasoningTokens  float64
	CostUSD              float64
}

// CaseStats is one case on one arm across its reps.
type CaseStats struct {
	ID       string
	Bucket   string
	Label    string
	Reps     int
	Verdicts map[string]int
	Majority string
	Agree    int
	ConfMean float64
	ConfSD   float64
}

// Correct reports whether the majority verdict matches a scored label.
func (s CaseStats) Correct() bool { return s.Label != "unknown" && s.Majority == s.Label }

// summarize computes the arm headline and the per-case breakdown from the
// rows and errors that belong to arm.
func summarize(arm Arm, rows []Row, errs []ErrRow) (ArmStats, map[string]CaseStats) {
	st := ArmStats{Arm: arm}
	byCase := map[string][]Row{}
	var latencies []int64
	var in, out, reasoning float64
	for _, r := range rows {
		if r.Variant != arm.ID {
			continue
		}
		st.Rows++
		byCase[r.ID] = append(byCase[r.ID], r)
		latencies = append(latencies, r.LatencyMS)
		in += float64(r.PromptTokens)
		out += float64(r.CompletionTokens)
		reasoning += float64(r.ReasoningTokens)
		st.CostUSD += r.CostUSD
	}
	for _, e := range errs {
		if e.Variant == arm.ID {
			st.Errors++
		}
	}
	cases := map[string]CaseStats{}
	for id, rs := range byCase {
		cases[id] = caseStats(rs)
	}
	fillRates(&st, rows, cases)
	st.LatencyP50 = percentile(latencies, 0.50)
	st.LatencyP95 = percentile(latencies, 0.95)
	if st.Rows > 0 {
		st.MeanPromptTokens = in / float64(st.Rows)
		st.MeanCompletionTokens = out / float64(st.Rows)
		st.MeanReasoningTokens = reasoning / float64(st.Rows)
	}
	return st, cases
}

func caseStats(rs []Row) CaseStats {
	s := CaseStats{
		ID: rs[0].ID, Bucket: rs[0].Bucket, Label: rs[0].Label,
		Reps: len(rs), Verdicts: map[string]int{},
	}
	var confs []float64
	for _, r := range rs {
		s.Verdicts[r.Verdict]++
		confs = append(confs, r.Confidence)
	}
	verdicts := slices.Sorted(func(yield func(string) bool) {
		for v := range s.Verdicts {
			if !yield(v) {
				return
			}
		}
	})
	for _, v := range verdicts {
		if n := s.Verdicts[v]; n > s.Agree {
			s.Majority, s.Agree = v, n
		}
	}
	s.ConfMean, s.ConfSD = meanSD(confs)
	return s
}

// fillRates computes the row-level rates and the case-level stability
// numbers for one arm. Row-level rates weight every rep equally; the
// stability numbers are per case, which is what a paired comparison between
// arms reads.
func fillRates(st *ArmStats, rows []Row, cases map[string]CaseStats) {
	var scored, correct, ai, aiHit, human, humanHit, humanFP, mixed float64
	for _, r := range rows {
		if r.Variant != st.Arm.ID || r.Grade == nil {
			continue
		}
		scored++
		correct += r.Grade["correct"]
		switch r.Label {
		case "likely_ai":
			ai++
			if r.Verdict == "likely_ai" {
				aiHit++
			}
		case "likely_human":
			human++
			switch r.Verdict {
			case "likely_human":
				humanHit++
			case "likely_ai":
				humanFP++
			}
		}
		if r.Verdict == "mixed" {
			mixed++
		}
	}
	st.ScoredRows = int(scored)
	st.Accuracy = correct / scored
	st.RecallAI = aiHit / ai
	st.SpecificityHuman = humanHit / human
	st.FPR = humanFP / human
	st.MixedRate = mixed / scored

	var n, flips, agree, sd float64
	for _, c := range cases {
		if c.Label == "unknown" || c.Reps < 2 {
			continue
		}
		n++
		if len(c.Verdicts) > 1 {
			flips++
		}
		agree += float64(c.Agree) / float64(c.Reps)
		sd += c.ConfSD * 100
	}
	st.ScoredCases = int(n)
	st.FlipRate = flips / n
	st.Agreement = agree / n
	st.ConfSDPoints = sd / n
}

// meanSD returns the mean and the sample standard deviation.
func meanSD(xs []float64) (mean, sd float64) {
	if len(xs) == 0 {
		return math.NaN(), math.NaN()
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	if len(xs) < 2 {
		return mean, 0
	}
	var ss float64
	for _, x := range xs {
		ss += (x - mean) * (x - mean)
	}
	return mean, math.Sqrt(ss / float64(len(xs)-1))
}

func percentile(xs []int64, p float64) int64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := slices.Clone(xs)
	slices.Sort(sorted)
	idx := int(math.Round(p * float64(len(sorted)-1)))
	return sorted[idx]
}

// sortRows orders rows by arm, case, rep so files and tables are stable
// across runs regardless of worker scheduling.
func sortRows(rows []Row) {
	slices.SortFunc(rows, func(a, b Row) int {
		if c := strings.Compare(a.Variant, b.Variant); c != 0 {
			return c
		}
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return a.Rep - b.Rep
	})
}

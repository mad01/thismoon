package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func renderSummary(dir string, meta runMeta, rows []Row, errs []ErrRow) error {
	f, err := os.Create(filepath.Join(dir, "summary.md"))
	if err != nil {
		return err
	}
	defer f.Close()
	writeSummary(f, meta, rows, errs)
	return f.Close()
}

func writeSummary(w io.Writer, meta runMeta, rows []Row, errs []ErrRow) {
	train, test := 0, 0
	for _, c := range meta.Cases {
		if c.Split == splitTest {
			test++
		} else {
			train++
		}
	}
	fmt.Fprintf(w, "# csl ask eval\n\n")
	fmt.Fprintf(w, "Started %s, %d cases (%d train, %d test), %d reps, via %s, prompt %s.\n\n",
		meta.Started.Format("2006-01-02 15:04"), len(meta.Cases), train, test, meta.Reps,
		orDash(meta.Via), meta.PromptHash)
	if meta.Regraded != nil {
		fmt.Fprintf(
			w,
			"Regraded %s with the cases as edited since; the answers are the original run's.\n\n",
			meta.Regraded.Format("2006-01-02 15:04"),
		)
	}

	fmt.Fprintln(w, "## Arms")
	fmt.Fprintln(w)
	fmt.Fprintln(
		w,
		"| arm | model | effort | rows | errors | pass | mean score | forbidden | over budget | line claims wrong | compression | p50 ms | in tok | out tok | cost |",
	)
	fmt.Fprintln(w, "|---|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|")
	stats := map[string]map[string]CaseStats{}
	for _, arm := range meta.Arms {
		st, cases := summarize(arm, rows, errs)
		stats[arm.ID] = cases
		fmt.Fprintf(
			w,
			"| %s | %s | %s | %d | %d | %s | %.2f | %s | %s | %d/%d | %.3f | %d | %.0f | %.0f | $%.3f |\n",
			st.Arm.ID,
			st.Arm.Model,
			orDash(st.Arm.Effort),
			st.Rows,
			st.Errors,
			pct(st.PassRate),
			st.MeanScore,
			pct(st.ForbiddenRate),
			pct(st.BudgetFailRate),
			st.LineClaimsWrong,
			st.LineClaims,
			st.Compression,
			st.LatencyP50,
			st.MeanPromptTok,
			st.MeanOutputTok,
			st.TotalCost,
		)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(
		w,
		"Pass needs every expected fact, no forbidden match, and an answer inside the budget. Compression is answer chars over corpus chars. Line claims are the path:line references the worker made and how many name an identifier that is not on that line. Cost is the list price of the tokens from the price table, with no cache discount; on the claude backend nothing is charged.",
	)
	fmt.Fprintln(w)

	fmt.Fprintln(w, "## Cases")
	fmt.Fprintln(w)
	fmt.Fprint(w, "| case | split | corpus |")
	for _, arm := range meta.Arms {
		fmt.Fprintf(w, " %s |", arm.ID)
	}
	fmt.Fprintln(w)
	fmt.Fprint(w, "|---|---|---|")
	for range meta.Arms {
		fmt.Fprint(w, "---|")
	}
	fmt.Fprintln(w)
	for _, c := range meta.Cases {
		fmt.Fprintf(
			w,
			"| %s | %s | %d files, %d lines |",
			c.ID,
			c.Split,
			len(c.Corpus.Files),
			c.Corpus.Lines(),
		)
		for _, arm := range meta.Arms {
			fmt.Fprintf(w, " %s |", caseCell(stats[arm.ID][c.ID]))
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "A cell is passes/reps, the mean score, and the compression.")
	fmt.Fprintln(w)

	writeMisses(w, meta, stats)
	writeErrors(w, errs)
}

func caseCell(s CaseStats) string {
	if s.Reps == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%d/%d %.2f c=%.3f", s.Passes, s.Reps, s.MeanScore, s.Compression)
}

// writeMisses lists, per arm and case, the facts any rep missed and the
// forbidden patterns any rep tripped. It is the tuning view: a missed
// pattern is either a worker gap or a pattern written too tightly.
func writeMisses(w io.Writer, meta runMeta, stats map[string]map[string]CaseStats) {
	fmt.Fprintln(w, "## Misses")
	fmt.Fprintln(w)
	found := false
	for _, arm := range meta.Arms {
		for _, c := range meta.Cases {
			s := stats[arm.ID][c.ID]
			if len(s.Missing) == 0 && len(s.Forbidden) == 0 && len(s.WrongClaims) == 0 {
				continue
			}
			found = true
			fmt.Fprintf(w, "- %s / %s (%d/%d pass)\n", arm.ID, c.ID, s.Passes, s.Reps)
			for _, m := range s.Missing {
				fmt.Fprintf(w, "  - missing `%s`\n", m)
			}
			for _, f := range s.Forbidden {
				fmt.Fprintf(w, "  - forbidden `%s`\n", f)
			}
			for _, wc := range s.WrongClaims {
				fmt.Fprintf(w, "  - wrong line claim: %s\n", strings.ReplaceAll(wc, "`", "'"))
			}
		}
	}
	if !found {
		fmt.Fprintln(w, "None.")
	}
	fmt.Fprintln(w)
}

func writeErrors(w io.Writer, errs []ErrRow) {
	if len(errs) == 0 {
		return
	}
	fmt.Fprintln(w, "## Errors")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "| arm | case | rep | class | error |")
	fmt.Fprintln(w, "|---|---|---:|---|---|")
	for _, e := range errs {
		fmt.Fprintf(w, "| %s | %s | %d | %s | %s |\n",
			e.Variant, e.ID, e.Rep, e.Class, strings.ReplaceAll(truncate(e.Error, 120), "|", "\\|"))
	}
	fmt.Fprintln(w)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func pct(x float64) string {
	return fmt.Sprintf("%.0f%%", x*100)
}

package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/mad01/thismoon/tools/factory-eval/internal/cases"
	"github.com/mad01/thismoon/tools/factory-eval/internal/eval"
	"github.com/mad01/thismoon/tools/factory-eval/internal/prereq"
)

// printPlan prints what the batch will do and what stands in its way, so a
// dry run shows every refusal a paid run would hit.
func printPlan(w io.Writer, b batch, dryRun bool) {
	mode := "paid run"
	if dryRun {
		mode = "dry run, no model call"
	}
	tierNames := make([]string, len(b.tiers))
	for i, t := range b.tiers {
		tierNames[i] = fmt.Sprintf(
			"%s (%s, %s effort, $%.2f cap)",
			t.Name,
			t.Model,
			t.Effort,
			t.BudgetUSD,
		)
	}
	fmt.Fprintf(w, "factory-eval %s\n", mode)
	fmt.Fprintf(w, "  source: %s at %s\n", b.source, b.file.Commit)
	fmt.Fprintf(w, "  arms: %s; tiers: %s; runs each: %d; cases: %d\n",
		strings.Join(b.arms, ", "), strings.Join(tierNames, ", "), b.runs, len(b.file.Cases))
	fmt.Fprintf(w, "  worst case: $%.2f against a batch ceiling of $%.2f\n",
		eval.WorstCaseUSD(b.file, len(b.arms), b.tiers, b.runs), b.file.BatchCeilingUSD)
	fmt.Fprintf(w, "  cases file: %s\n", signOffState(b.file))
	fmt.Fprintf(w, "  sign-off digest (sign_off.digest must hold it): %s\n", b.file.Digest())
	fmt.Fprintln(w, "prerequisites:")
	for _, r := range b.prereqs() {
		state := "ok  "
		if !r.OK {
			state = "FAIL"
		}
		fmt.Fprintf(w, "  %s %s: %s\n", state, r.Name, r.Detail)
	}
	if failed := prereq.Failed(b.factory); len(failed) > 0 {
		fmt.Fprintf(w, "%s\n", prereq.Explain(eval.ArmFactory, failed))
		if dryRun {
			fmt.Fprintln(w, "(a dry run rehearses the factory arm anyway; a paid run refuses here)")
		}
	}
	if len(b.unsandboxed) > 0 {
		fmt.Fprintf(w, "unsandboxed: %s, excluded from the sandbox by the user's settings; "+
			"the run cannot deny them\n", strings.Join(b.unsandboxed, ", "))
		switch {
		case b.allowUnsandboxed:
			fmt.Fprintln(w, "(accepted with --allow-unsandboxed)")
		case dryRun:
			fmt.Fprintln(
				w,
				"(a dry run goes on; a paid run refuses here without --allow-unsandboxed)",
			)
		}
	}
	fmt.Fprintln(w)
}

func signOffState(f *cases.File) string {
	if err := f.CheckSignOff(); err != nil {
		return "NOT SIGNED OFF, a paid run refuses: " + strings.ReplaceAll(err.Error(), "\n", "; ")
	}
	return fmt.Sprintf("signed off by %s on %s, digest matches", f.SignOff.By, f.SignOff.Date)
}

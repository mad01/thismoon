// Command ask runs the csl ask worker eval: every case under cases/ has its
// corpus resolved by a csl lexical search, is asked on each configured
// worker model, and is graded on the facts it must carry, the names it
// must not invent, and the size of the answer.
//
//	cd services/csl
//	go run ./evals/ask -list                    # the sign-off document, no network
//	go run ./evals/ask -oracle                  # grading sanity check, no network
//	go run ./evals/ask -arms haiku55 -reps 1 -filter validmode   # one call on the auto-picked transport
//	go run ./evals/ask -via openrouter          # the full matrix, forced onto OpenRouter (paid)
//	go run ./evals/ask                          # the full matrix on the auto-picked transport
//	go run ./evals/ask -regrade evals/ask/runs/<ts>   # re-grade a run after a pattern edit, no network
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mad01/thismoon/kit/progstatus"
)

func main() {
	var (
		casesDir = flag.String("cases", "evals/ask/cases", "directory of case files")
		armIDs   = flag.String("arms", "", "comma-separated arm ids (default: all)")
		via      = flag.String(
			"via",
			"auto",
			"transport: auto (litellm, then claude, then openrouter), litellm, claude, or openrouter",
		)
		reps    = flag.Int("reps", 3, "repetitions per case and arm")
		outDir  = flag.String("out", "", "run directory (default: evals/ask/runs/<timestamp>)")
		workers = flag.Int("workers", 4, "concurrent requests")
		filter  = flag.String("filter", "", "only cases whose id contains this substring")
		split   = flag.String("split", "all", "only cases in this split: train, test, or all")
		list    = flag.Bool("list", false, "print the sign-off document and exit")
		oracle  = flag.Bool(
			"oracle",
			false,
			"run the grading gates (oracle, empty, dump) and exit",
		)
		report  = flag.String("report", "", "re-render summary.md for this run directory and exit")
		regrade = flag.String(
			"regrade",
			"",
			"re-grade this run directory's answers with the current cases and exit (no network)",
		)
	)
	flag.Parse()
	status := progstatus.New("csl-ask-eval")
	err := run(runOptions{
		casesDir: *casesDir, armIDs: *armIDs, via: *via, reps: *reps, outDir: *outDir, workers: *workers,
		filter: *filter, split: *split, list: *list, oracle: *oracle, report: *report,
		regrade: *regrade,
	}, status)
	if err != nil {
		status.Error(err.Error())
		fmt.Fprintln(os.Stderr, "ask eval:", err)
		os.Exit(1)
	}
}

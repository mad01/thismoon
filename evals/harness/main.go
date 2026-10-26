// Command harness runs the agent harness eval: fixed tasks through
// `claude -p` with the user's real settings, hooks, and MCP servers, scored
// on outcome plus cost, turns, tokens, and tool use. Runs are stored so a
// hook, skill, or belt change can be compared before and after.
//
//	go run ./evals/harness -list                       # the cases, no claude
//	go run ./evals/harness -case where-defined         # one case, one rep
//	go run ./evals/harness -case a,b -reps 3           # several cases and reps
//	go run ./evals/harness                             # every case
//	go run ./evals/harness -report <run>               # re-render summary.md
//	go run ./evals/harness -compare <runA> <runB>      # per-case deltas
//
// Run it from the repo root; <run> is a run directory or its id under
// evals/harness/runs.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/mad01/thismoon/kit/progstatus"
)

// defaultMaxBudgetUSD caps one case run so a runaway case cannot spend
// more than this; claude stops the run when it reaches the cap. A fresh
// session on the default model costs about a dollar before it does any
// work, so the cap leaves room for the edit cases.
const defaultMaxBudgetUSD = 5.0

type options struct {
	casesDir     string
	runsDir      string
	list         bool
	caseIDs      string
	reps         int
	model        string
	maxBudgetUSD float64
	roots        workspaceRoots
	scorecardURL string
	report       string
	compare      []string
}

func main() {
	var o options
	flag.StringVar(&o.casesDir, "cases", "evals/harness/cases", "directory of case files")
	flag.StringVar(&o.runsDir, "runs", "evals/harness/runs", "directory runs are written to")
	flag.BoolVar(&o.list, "list", false, "print the cases and exit")
	flag.StringVar(&o.caseIDs, "case", "", "comma-separated case ids (default: all)")
	flag.IntVar(&o.reps, "reps", 1, "repetitions per case")
	flag.StringVar(&o.model, "model", "", "model for the runs (default: the user's setting)")
	flag.Float64Var(&o.maxBudgetUSD, "max-budget-usd", defaultMaxBudgetUSD,
		"hard cost cap per case run, passed to claude (0 for none)")
	flag.StringVar(&o.roots.SrcRoot, "src", "~/code/src/github.com/mad01",
		"directory that bare repo names in workspaces resolve under")
	flag.StringVar(&o.roots.WorktreeRoot, "worktrees", "~/.worktrees",
		"directory worktree workspaces are created under")
	flag.StringVar(&o.scorecardURL, "scorecard-url", "http://127.0.0.1:7433",
		"moondb base URL for session scorecards (empty to skip)")
	flag.StringVar(&o.report, "report", "", "re-render summary.md for this run and exit")
	compareA := flag.String("compare", "", "compare this run to the run named after the flags")
	flag.Parse()
	if *compareA != "" {
		o.compare = append([]string{*compareA}, flag.Args()...)
	}
	status := progstatus.New("harness-eval")
	if err := run(o, status); err != nil {
		status.Error(err.Error())
		fmt.Fprintln(os.Stderr, "harness eval:", err)
		os.Exit(1)
	}
}

func run(o options, status *progstatus.Reporter) error {
	switch {
	case len(o.compare) > 0:
		return compareRuns(o.runsDir, o.compare)
	case o.report != "":
		return rerender(resolveRun(o.runsDir, o.report))
	}
	cases, err := loadCases(o.casesDir)
	if err != nil {
		return err
	}
	if o.list {
		writeList(os.Stdout, cases)
		return nil
	}
	if cases, err = selectCases(cases, o.caseIDs); err != nil {
		return err
	}
	if o.reps < 1 {
		return errors.New("-reps must be at least 1")
	}
	return runEval(o, cases, status)
}

// runEval runs the cases and writes runs/<id>/: run.json, results.jsonl,
// summary.md, and the raw transcripts under streams/.
func runEval(o options, cases []Case, status *progstatus.Reporter) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	bin := claudeBin()
	meta := runMeta{
		RunID:         time.Now().Format("20060102-150405"),
		StartedAt:     time.Now(),
		ClaudeVersion: claudeVersion(ctx, bin),
		Model:         o.model,
		Flags:         flagTemplate(o.model, o.maxBudgetUSD),
		Reps:          o.reps,
		MaxBudgetUSD:  o.maxBudgetUSD,
	}
	for _, c := range cases {
		meta.Cases = append(meta.Cases, c.ID)
	}
	dir := filepath.Join(o.runsDir, meta.RunID)
	streamDir := filepath.Join(dir, "streams")
	if err := os.MkdirAll(streamDir, 0o755); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "run.json"), meta); err != nil {
		return err
	}
	r := &runner{
		bin: bin, model: o.model, maxBudgetUSD: o.maxBudgetUSD, roots: o.roots,
		scorecardURL: o.scorecardURL, client: http.DefaultClient,
		judge: claudeJudge{bin: bin}, streamDir: streamDir, now: time.Now, log: os.Stderr,
	}
	status.Working(fmt.Sprintf("running %d cases, %d reps", len(cases), o.reps))
	rows := r.runAll(ctx, cases, o.reps)
	meta.FinishedAt, meta.Totals = time.Now(), totalsOf(rows)
	if err := writeRun(dir, meta, rows); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\nrun written to %s\n", dir)
	if ctx.Err() != nil {
		status.Idle()
		return nil
	}
	status.Done(fmt.Sprintf("%d of %d passed, $%.2f", meta.Totals.Passed, len(rows),
		meta.Totals.CostUSD+meta.Totals.JudgeCostUSD))
	return nil
}

// writeRun writes run.json, results.jsonl, and summary.md, and prints the
// summary.
func writeRun(dir string, meta runMeta, rows []Row) error {
	if err := writeJSON(filepath.Join(dir, "run.json"), meta); err != nil {
		return err
	}
	if err := writeJSONL(filepath.Join(dir, "results.jsonl"), rows); err != nil {
		return err
	}
	return writeSummaryFile(dir, meta, rows)
}

func writeSummaryFile(dir string, meta runMeta, rows []Row) error {
	var buf bytes.Buffer
	writeSummary(&buf, meta, rows)
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Print(buf.String())
	return nil
}

// rerender rebuilds summary.md for a stored run, so a report change
// applies to past runs too.
func rerender(dir string) error {
	meta, rows, err := readRun(dir)
	if err != nil {
		return err
	}
	return writeSummaryFile(dir, meta, rows)
}

func compareRuns(runsDir string, names []string) error {
	if len(names) != 2 {
		return errors.New("-compare takes two runs: -compare <runA> <runB>")
	}
	dirA, dirB := resolveRun(runsDir, names[0]), resolveRun(runsDir, names[1])
	_, a, err := readRun(dirA)
	if err != nil {
		return err
	}
	_, b, err := readRun(dirB)
	if err != nil {
		return err
	}
	writeCompare(os.Stdout, filepath.Base(dirA), filepath.Base(dirB), a, b)
	return nil
}

// resolveRun accepts a run directory or a run id under runsDir.
func resolveRun(runsDir, name string) string {
	if info, err := os.Stat(name); err == nil && info.IsDir() {
		return name
	}
	return filepath.Join(runsDir, name)
}

func readRun(dir string) (runMeta, []Row, error) {
	var meta runMeta
	data, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return meta, nil, err
	}
	if err := json.Unmarshal(data, &meta); err != nil {
		return meta, nil, fmt.Errorf("%s: %w", dir, err)
	}
	rows, err := readJSONL[Row](filepath.Join(dir, "results.jsonl"))
	return meta, rows, err
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeJSONL[T any](path string, items []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			_ = f.Close()
			return err
		}
	}
	return f.Close()
}

func readJSONL[T any](path string) ([]T, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var out []T
	dec := json.NewDecoder(f)
	for dec.More() {
		var it T
		if err := dec.Decode(&it); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, it)
	}
	return out, nil
}

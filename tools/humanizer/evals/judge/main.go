// Command judge runs the humanizer judge eval: every labeled case under
// cases/ through the production judge path on each configured arm, with
// repetitions, and writes results, errors, traces, and a markdown summary.
//
//	source ~/.secrets.sh                      # OPENROUTER_API_KEY
//	go run ./evals/judge -list                # the sign-off document, no network
//	go run ./evals/judge -oracle              # grading sanity check, no network
//	go run ./evals/judge -null likely_ai      # grading sanity check, no network
//	go run ./evals/judge -arms haiku55 -reps 1 -filter slop   # smoke test
//	go run ./evals/judge                      # the full matrix
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/mad01/thismoon/tools/humanizer/internal/judge"
)

func main() {
	var (
		casesDir = flag.String("cases", "evals/judge/cases", "directory of case files")
		armIDs   = flag.String("arms", "", "comma-separated arm ids (default: all)")
		reps     = flag.Int("reps", 5, "repetitions per case and arm")
		outDir   = flag.String("out", "", "run directory (default: evals/judge/runs/<timestamp>)")
		workers  = flag.Int("workers", 4, "concurrent requests")
		filter   = flag.String("filter", "", "only cases whose id contains this substring")
		split    = flag.String("split", "all", "only cases in this split: train, test, or all")
		list     = flag.Bool("list", false, "print the sign-off document and exit")
		oracle   = flag.Bool(
			"oracle",
			false,
			"grade the labels themselves (must score 100%) and exit",
		)
		null = flag.String(
			"null",
			"",
			"grade this constant verdict (must score poorly) and exit",
		)
		report = flag.String("report", "", "re-render summary.md for this run directory and exit")
	)
	flag.Parse()
	err := run(runOptions{
		casesDir: *casesDir, armIDs: *armIDs, reps: *reps, outDir: *outDir, workers: *workers,
		filter: *filter, split: *split, list: *list, oracle: *oracle, null: *null,
		report: *report,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "judge eval:", err)
		os.Exit(1)
	}
}

type runOptions struct {
	casesDir string
	armIDs   string
	reps     int
	outDir   string
	workers  int
	filter   string
	split    string
	list     bool
	oracle   bool
	null     string
	report   string
}

func run(o runOptions) error {
	cases, err := loadCases(o.casesDir)
	if err != nil {
		return err
	}
	if o.list {
		writeReview(os.Stdout, cases)
		return nil
	}
	if o.report != "" {
		return rerender(o.report, cases)
	}
	cases = filterCases(cases, o.filter, o.split)
	if len(cases) == 0 {
		return errors.New("no cases match the filter and split")
	}
	if o.oracle || o.null != "" {
		return printSynthetic(os.Stdout, cases, o.reps, o.null)
	}
	arms, err := selectArms(o.armIDs)
	if err != nil {
		return err
	}
	reps, outDir, workers := o.reps, o.outDir, o.workers
	key := os.Getenv("OPENROUTER_API_KEY")
	if key == "" {
		return fmt.Errorf("OPENROUTER_API_KEY is not set (source ~/.secrets.sh)")
	}
	runID := time.Now().Format("20060102-150405")
	if outDir == "" {
		outDir = filepath.Join("evals/judge/runs", runID)
	}
	traceDir := filepath.Join(outDir, "traces")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		return err
	}
	if err := writeRunMeta(outDir, runID, arms, cases, reps); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	rows, errs := runAll(ctx, key, buildJobs(cases, arms, reps), traceDir, workers)
	if err := writeJSONL(filepath.Join(outDir, "results.jsonl"), rows); err != nil {
		return err
	}
	if err := writeJSONL(filepath.Join(outDir, "errors.jsonl"), errs); err != nil {
		return err
	}
	var buf bytes.Buffer
	writeSummary(&buf, runID, arms, cases, rows, errs, reps)
	if err := os.WriteFile(filepath.Join(outDir, "summary.md"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Print(buf.String())
	fmt.Fprintf(os.Stderr, "\nrun written to %s\n", outDir)
	return nil
}

// rerender rebuilds summary.md for an existing run from its results and
// errors files, so a report change applies to past runs too. Cases without
// rows (filtered out at run time) are dropped from the grid.
func rerender(dir string, cases []Case) error {
	var meta runMeta
	if data, err := os.ReadFile(filepath.Join(dir, "run.json")); err == nil {
		_ = json.Unmarshal(data, &meta)
	}
	rows, err := readJSONL[Row](filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return err
	}
	errs, err := readJSONL[ErrRow](filepath.Join(dir, "errors.jsonl"))
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.ID] = true
	}
	var ran []Case
	for _, c := range cases {
		if seen[c.ID] {
			ran = append(ran, c)
		}
	}
	arms := meta.Arms
	if len(arms) == 0 {
		arms = allArms()
	}
	var buf bytes.Buffer
	writeSummary(&buf, meta.RunID, arms, ran, rows, errs, meta.Reps)
	if err := os.WriteFile(filepath.Join(dir, "summary.md"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	fmt.Print(buf.String())
	return nil
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

func filterCases(cases []Case, substr, split string) []Case {
	var out []Case
	for _, c := range cases {
		if substr != "" && !strings.Contains(c.ID, substr) {
			continue
		}
		if split != "all" && c.Split != split {
			continue
		}
		out = append(out, c)
	}
	return out
}

func buildJobs(cases []Case, arms []Arm, reps int) []job {
	var jobs []job
	for rep := range reps {
		for _, arm := range arms {
			for _, c := range cases {
				jobs = append(jobs, job{c: c, arm: arm, rep: rep})
			}
		}
	}
	return jobs
}

// printSynthetic runs the grading path without a model: the oracle feeds
// each label back as the verdict and must score 100%; a constant verdict
// must score the base rate. Either failing means the harness is broken.
func printSynthetic(w *os.File, cases []Case, reps int, constant string) error {
	arm := Arm{ID: "oracle", Model: "none", Notes: "labels fed back as verdicts"}
	if constant != "" {
		arm = Arm{ID: "null-" + constant, Model: "none", Notes: "constant verdict " + constant}
	}
	var rows []Row
	for rep := range reps {
		for _, c := range cases {
			verdict := c.Label
			if constant != "" {
				verdict = constant
			}
			if verdict == "unknown" {
				verdict = "mixed"
			}
			rows = append(rows, Row{
				ID: c.ID, Bucket: c.Bucket, Label: c.Label, Variant: arm.ID, Rep: rep,
				Verdict: verdict, Confidence: 0.5, Grade: grade(c, verdict), Model: arm.Model,
			})
		}
	}
	writeSummary(w, "synthetic", []Arm{arm}, cases, rows, nil, reps)
	return nil
}

type runMeta struct {
	RunID              string    `json:"run_id"`
	StartedAt          time.Time `json:"started_at"`
	Arms               []Arm     `json:"arms"`
	Reps               int       `json:"reps"`
	Cases              []string  `json:"cases"`
	SystemPromptSHA    string    `json:"system_prompt_sha256"`
	SystemPrompt       string    `json:"system_prompt"`
	JudgeContractRetry bool      `json:"judge_contract_retry"`
}

func writeRunMeta(dir, runID string, arms []Arm, cases []Case, reps int) error {
	sum := sha256.Sum256([]byte(judge.SystemPrompt))
	m := runMeta{
		RunID: runID, StartedAt: time.Now(), Arms: arms, Reps: reps,
		SystemPromptSHA: hex.EncodeToString(sum[:]), SystemPrompt: judge.SystemPrompt,
		JudgeContractRetry: true,
	}
	for _, c := range cases {
		m.Cases = append(m.Cases, c.ID)
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "run.json"), data, 0o644)
}

func writeJSONL[T any](path string, items []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	enc := json.NewEncoder(f)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}

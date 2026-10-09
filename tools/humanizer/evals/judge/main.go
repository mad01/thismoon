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
	)
	flag.Parse()
	if err := run(*casesDir, *armIDs, *reps, *outDir, *workers, *filter, *list, *oracle, *null); err != nil {
		fmt.Fprintln(os.Stderr, "judge eval:", err)
		os.Exit(1)
	}
}

func run(
	casesDir, armIDs string, reps int, outDir string, workers int,
	filter string, list, oracle bool, null string,
) error {
	cases, err := loadCases(casesDir)
	if err != nil {
		return err
	}
	if list {
		writeReview(os.Stdout, cases)
		return nil
	}
	cases = filterCases(cases, filter)
	if oracle || null != "" {
		return printSynthetic(os.Stdout, cases, reps, null)
	}
	arms, err := selectArms(armIDs)
	if err != nil {
		return err
	}
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

func filterCases(cases []Case, substr string) []Case {
	if substr == "" {
		return cases
	}
	var out []Case
	for _, c := range cases {
		if strings.Contains(c.ID, substr) {
			out = append(out, c)
		}
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

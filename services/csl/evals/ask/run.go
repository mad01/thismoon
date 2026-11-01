package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/kit/progstatus"
	"github.com/mad01/thismoon/services/csl/internal/ask"
)

const (
	// maxAttempts bounds the retries on a transient provider failure.
	maxAttempts = 3
	// callTimeout bounds one attempt. A 2000-line corpus is about 30k
	// tokens in, and a slow provider route can take a minute on it.
	callTimeout = 180 * time.Second
)

// Arm is one worker model at one effort. Every arm gets the same prompt and
// message through the run's backend (kit/llm); Model is the canonical
// Anthropic id and the backend translates it.
type Arm struct {
	ID     string `json:"id"`
	Model  string `json:"model"`
	Effort string `json:"effort,omitempty"`
	Notes  string `json:"notes"`
}

func allArms() []Arm {
	return []Arm{
		{
			ID: "haiku55", Model: "claude-haiku-5-5",
			Notes: "the candidate worker: Haiku 5.5 at the default effort",
		},
		{
			ID: "haiku55-low", Model: "claude-haiku-5-5", Effort: "low",
			Notes: "Haiku 5.5 at low effort: the cheapest and fastest shape",
		},
		{
			ID: "haiku55-medium", Model: "claude-haiku-5-5", Effort: "medium",
			Notes: "Haiku 5.5 at medium effort, the API default, named so a sweep can pin it",
		},
		{
			ID: "haiku55-high", Model: "claude-haiku-5-5", Effort: "high",
			Notes: "Haiku 5.5 at high effort: more thinking on a task the facts already pass",
		},
		{
			ID: "haiku55-xhigh", Model: "claude-haiku-5-5", Effort: "xhigh",
			Notes: "Haiku 5.5 at xhigh effort, between high and max",
		},
		{
			ID: "haiku55-max", Model: "claude-haiku-5-5", Effort: "max",
			Notes: "Haiku 5.5 at max effort: the top of the sweep",
		},
		{
			ID: "sonnet55", Model: "claude-sonnet-5-5",
			Notes: "the ceiling: Sonnet 5.5 at the default effort, to show what Haiku leaves on the table",
		},
		{
			ID: "opus55-low", Model: "claude-opus-5-5", Effort: "low",
			Notes: "Opus 5.5 at low effort: the strongest reader with the thinking turned down",
		},
	}
}

func selectArms(ids string) ([]Arm, error) {
	all := allArms()
	if ids == "" {
		return all, nil
	}
	var out []Arm
	for id := range strings.SplitSeq(ids, ",") {
		id = strings.TrimSpace(id)
		i := -1
		for k, a := range all {
			if a.ID == id {
				i = k
			}
		}
		if i < 0 {
			return nil, fmt.Errorf("unknown arm %q", id)
		}
		out = append(out, all[i])
	}
	return out, nil
}

// Row is one scored (case, arm, rep) result.
type Row struct {
	ID               string  `json:"id"`
	Split            string  `json:"split"`
	Variant          string  `json:"variant"`
	Via              string  `json:"via"`
	Rep              int     `json:"rep"`
	Answer           string  `json:"answer"`
	Grade            Grade   `json:"grade"`
	CorpusFiles      int     `json:"corpus_files"`
	CorpusLines      int     `json:"corpus_lines"`
	CorpusChars      int     `json:"corpus_chars"`
	Model            string  `json:"model"`
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	ReasoningTokens  int     `json:"reasoning_tokens"`
	CostUSD          float64 `json:"cost_usd"`
	LatencyMS        int64   `json:"latency_ms"`
	Attempts         int     `json:"attempts"`
	Trace            string  `json:"trace"`
}

// ErrRow is an attempt that produced no answer. It lives in errors.jsonl so
// plumbing failures never land in the score.
type ErrRow struct {
	ID       string `json:"id"`
	Variant  string `json:"variant"`
	Rep      int    `json:"rep"`
	Class    string `json:"class"`
	Error    string `json:"error"`
	Attempts int    `json:"attempts"`
	Trace    string `json:"trace"`
}

type job struct {
	c      Case
	corpus Corpus
	arm    Arm
	rep    int
}

type trace struct {
	Case     string         `json:"case"`
	Variant  string         `json:"variant"`
	Rep      int            `json:"rep"`
	Arm      Arm            `json:"arm"`
	Question string         `json:"question"`
	Corpus   Corpus         `json:"corpus"`
	Reply    string         `json:"reply"`
	Calls    []llm.CallInfo `json:"calls"`
	Grade    *Grade         `json:"grade,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// runMeta is run.json: what the run asked for, and each case's corpus as
// it was resolved, so a later reader knows what the worker saw.
type runMeta struct {
	Started    time.Time `json:"started"`
	Finished   time.Time `json:"finished"`
	Via        string    `json:"via"`
	Arms       []Arm     `json:"arms"`
	Reps       int       `json:"reps"`
	Split      string    `json:"split"`
	Filter     string    `json:"filter,omitempty"`
	PromptHash string    `json:"prompt_hash"`
	// Regraded is set when -regrade rescored the answers with edited
	// cases; the answers and traces are the original run's.
	Regraded *time.Time `json:"regraded,omitempty"`
	Cases    []caseMeta `json:"cases"`
}

type caseMeta struct {
	ID     string `json:"id"`
	Split  string `json:"split"`
	Budget int    `json:"budget"`
	Corpus Corpus `json:"corpus"`
}

type runOptions struct {
	casesDir string
	armIDs   string
	via      string
	reps     int
	outDir   string
	workers  int
	filter   string
	split    string
	list     bool
	oracle   bool
	report   string
	regrade  string
}

func run(opts runOptions, status *progstatus.Reporter) error {
	if opts.report != "" {
		return rerender(opts.report)
	}
	if opts.regrade != "" {
		return regrade(opts.regrade, opts.casesDir)
	}
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
		syscall.SIGHUP,
	)
	defer stop()

	all, err := loadCases(opts.casesDir)
	if err != nil {
		return err
	}
	cases, err := selectCases(all, opts.filter, opts.split)
	if err != nil {
		return err
	}
	status.Working("resolving corpora")
	corpora := map[string]Corpus{}
	for _, c := range cases {
		corpus, err := resolveCorpus(ctx, c)
		if err != nil {
			return err
		}
		corpora[c.ID] = corpus
	}
	if opts.list {
		return writeList(os.Stdout, cases, corpora)
	}
	if opts.oracle {
		return runGates(os.Stdout, cases, corpora)
	}

	arms, err := selectArms(opts.armIDs)
	if err != nil {
		return err
	}
	if opts.reps < 1 {
		return errors.New("-reps must be at least 1")
	}
	if opts.workers < 1 {
		return errors.New("-workers must be at least 1")
	}
	via, err := resolveVia(opts.via)
	if err != nil {
		return err
	}
	outDir := opts.outDir
	if outDir == "" {
		outDir = filepath.Join(
			filepath.Dir(opts.casesDir),
			"runs",
			time.Now().Format("20060102-150405"),
		)
	}
	traceDir := filepath.Join(outDir, "traces")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		return err
	}

	var jobs []job
	for _, arm := range arms {
		for _, c := range cases {
			for rep := 1; rep <= opts.reps; rep++ {
				jobs = append(jobs, job{c: c, corpus: corpora[c.ID], arm: arm, rep: rep})
			}
		}
	}
	meta := runMeta{
		Started: time.Now(), Via: via, Arms: arms, Reps: opts.reps, Split: opts.split,
		Filter: opts.filter, PromptHash: promptHash(),
	}
	for _, c := range cases {
		meta.Cases = append(meta.Cases, caseMeta{
			ID: c.ID, Split: c.Split, Budget: c.MaxAnswerChars, Corpus: corpora[c.ID],
		})
	}
	status.Working(fmt.Sprintf("%d jobs on %d arms via %s", len(jobs), len(arms), via))
	rows, errs := runAll(ctx, via, jobs, traceDir, opts.workers)
	meta.Finished = time.Now()

	if err := writeJSONL(filepath.Join(outDir, "results.jsonl"), rows); err != nil {
		return err
	}
	if err := writeJSONL(filepath.Join(outDir, "errors.jsonl"), errs); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(outDir, "run.json"), meta); err != nil {
		return err
	}
	if err := renderSummary(outDir, meta, rows, errs); err != nil {
		return err
	}
	fmt.Println(outDir)
	if ctx.Err() != nil {
		status.Idle()
		return fmt.Errorf("interrupted after %d rows", len(rows))
	}
	status.Done(fmt.Sprintf("%d rows, %d errors", len(rows), len(errs)))
	return nil
}

// resolveVia turns the -via flag into the backend name the run records.
// "auto" is kit/llm's empty selector; the probe here fails early on a
// missing backend instead of once per job.
func resolveVia(via string) (string, error) {
	if via == "auto" {
		via = ""
	}
	b, err := llm.Select(llm.Options{Backend: via, AllowCLI: true})
	if err != nil {
		return "", err
	}
	return b.Name(), nil
}

// backendFor builds the backend one job calls: the run's backend on the
// arm's model and effort, reporting every exchange to observe.
func backendFor(via string, arm Arm, observe func(llm.CallInfo)) (llm.Backend, error) {
	return llm.Select(llm.Options{
		Backend: via, Model: arm.Model, Effort: arm.Effort, AllowCLI: true, Observe: observe,
	})
}

// runJob asks one case once on one arm, retrying transient provider
// failures with jittered backoff.
func runJob(ctx context.Context, via string, j job, traceDir string) (*Row, *ErrRow) {
	message := buildMessage(j.corpus, j.c.Question)
	var (
		calls []llm.CallInfo
		reply string
	)
	b, err := backendFor(via, j.arm, func(ci llm.CallInfo) { calls = append(calls, ci) })
	if err != nil {
		return nil, &ErrRow{
			ID: j.c.ID, Variant: j.arm.ID, Rep: j.rep, Class: "backend", Error: err.Error(),
		}
	}
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, callTimeout)
		reply, err = b.Complete(cctx, ask.SystemPrompt, message)
		cancel()
		var info llm.CallInfo
		if len(calls) >= attempt {
			info = calls[len(calls)-1]
		}
		if err == nil || ctx.Err() != nil || !transient(err, info) || attempt == maxAttempts {
			break
		}
		time.Sleep(time.Duration(attempt*attempt)*time.Second + rand.N(time.Second))
	}
	var g *Grade
	if err == nil {
		gg := grade(j.c, j.corpus, reply)
		g = &gg
	}
	tracePath := writeTrace(traceDir, j, reply, calls, g, err)
	if err != nil {
		return nil, &ErrRow{
			ID: j.c.ID, Variant: j.arm.ID, Rep: j.rep,
			Class: classify(err, calls), Error: err.Error(), Attempts: len(calls), Trace: tracePath,
		}
	}
	last := calls[len(calls)-1]
	if !servedMatches(j.arm.Model, last.Model) {
		return nil, &ErrRow{
			ID: j.c.ID, Variant: j.arm.ID, Rep: j.rep, Class: "served-model-mismatch",
			Error:    fmt.Sprintf("asked %q, served %q", j.arm.Model, last.Model),
			Attempts: len(calls), Trace: tracePath,
		}
	}
	var in, out, reasoning int
	for _, c := range calls {
		in += c.PromptTokens
		out += c.CompletionTokens
		reasoning += c.ReasoningTokens
	}
	return &Row{
		ID: j.c.ID, Split: j.c.Split, Variant: j.arm.ID, Via: via, Rep: j.rep,
		Answer: reply, Grade: *g,
		CorpusFiles: len(
			j.corpus.Files,
		), CorpusLines: j.corpus.Lines(), CorpusChars: j.corpus.Bytes(),
		Model: last.Model, PromptTokens: in, CompletionTokens: out, ReasoningTokens: reasoning,
		CostUSD:   ask.CostUSD(j.arm.Model, in, out),
		LatencyMS: last.Latency.Milliseconds(), Attempts: len(calls), Trace: tracePath,
	}, nil
}

// transient reports whether the failure is worth another attempt: a rate
// limit or server error, a timeout, or no response at all (a zero info).
func transient(err error, info llm.CallInfo) bool {
	if errors.Is(err, context.DeadlineExceeded) || info.Status == 0 {
		return true
	}
	return info.Status == http.StatusTooManyRequests || info.Status >= 500
}

func classify(err error, calls []llm.CallInfo) string {
	msg := err.Error()
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case strings.Contains(msg, "no content"):
		return "empty"
	case len(calls) == 0:
		return "network"
	}
	status := calls[len(calls)-1].Status
	switch {
	case status == http.StatusTooManyRequests || status >= 500:
		return "transient-exhausted"
	case status >= 400:
		return fmt.Sprintf("http-%d", status)
	}
	return "other"
}

// servedMatches accepts the model the arm asked for under any id a
// backend reports it as: the canonical id, OpenRouter's dotted id with
// an optional routing suffix, or a dated snapshot of the canonical id.
// Anything else is a substitution.
func servedMatches(asked, served string) bool {
	if served == asked || strings.HasPrefix(served, asked+"-") {
		return true
	}
	if or := llm.OpenRouterID(asked); or != asked {
		return served == or || strings.HasPrefix(served, or+":")
	}
	return false
}

func writeTrace(dir string, j job, reply string, calls []llm.CallInfo, g *Grade, err error) string {
	t := trace{
		Case: j.c.ID, Variant: j.arm.ID, Rep: j.rep, Arm: j.arm,
		Question: j.c.Question, Corpus: j.corpus, Reply: reply, Calls: calls, Grade: g,
	}
	if err != nil {
		t.Error = err.Error()
	}
	path := filepath.Join(dir, fmt.Sprintf("%s__%s__rep%d.json", j.arm.ID, j.c.ID, j.rep))
	if werr := writeJSON(path, t); werr != nil {
		fmt.Fprintf(os.Stderr, "trace %s: %v\n", path, werr)
	}
	return path
}

// runAll drives the jobs through a worker pool and returns rows and errors
// in a stable order.
func runAll(
	ctx context.Context,
	via string,
	jobs []job,
	traceDir string,
	workers int,
) ([]Row, []ErrRow) {
	var (
		mu   sync.Mutex
		rows []Row
		errs []ErrRow
		wg   sync.WaitGroup
	)
	queue := make(chan job)
	for range workers {
		wg.Go(func() {
			for j := range queue {
				if ctx.Err() != nil {
					continue
				}
				row, erow := runJob(ctx, via, j, traceDir)
				mu.Lock()
				if row != nil {
					rows = append(rows, *row)
					fmt.Fprintf(os.Stderr, "%-10s %-30s rep%d %s %d/%d %5dch %6dms\n",
						j.arm.ID, j.c.ID, j.rep, passWord(row.Grade.Pass),
						row.Grade.Found, row.Grade.Total, row.Grade.Chars, row.LatencyMS)
				} else {
					errs = append(errs, *erow)
					fmt.Fprintf(os.Stderr, "%-10s %-30s rep%d ERROR %s: %s\n",
						j.arm.ID, j.c.ID, j.rep, erow.Class, erow.Error)
				}
				mu.Unlock()
			}
		})
	}
	for _, j := range jobs {
		queue <- j
	}
	close(queue)
	wg.Wait()
	sortRows(rows)
	return rows, errs
}

func passWord(pass bool) string {
	if pass {
		return "pass"
	}
	return "FAIL"
}

// runGates checks the grader against each case before anything is paid
// for: the oracle must score full marks, an empty answer none, and the
// corpus echoed back must fail the budget.
func runGates(w *os.File, cases []Case, corpora map[string]Corpus) error {
	failed := 0
	for _, c := range cases {
		corpus := corpora[c.ID]
		for _, gate := range []struct {
			name   string
			answer string
			ok     func(Grade) bool
		}{
			{"oracle", c.Oracle, func(g Grade) bool { return g.Pass && g.LineClaimsWrong == 0 }},
			{"empty", "", func(g Grade) bool { return g.Found == 0 && !g.Pass }},
			{"dump", corpus.dump(), func(g Grade) bool { return !g.Pass && !g.BudgetOK }},
		} {
			g := grade(c, corpus, gate.answer)
			if gate.ok(g) {
				fmt.Fprintf(
					w,
					"  pass  %-8s %-32s %d/%d facts, %d/%d chars, line claims %d wrong of %d\n",
					gate.name,
					c.ID,
					g.Found,
					g.Total,
					g.Chars,
					g.Budget,
					g.LineClaimsWrong,
					g.LineClaims,
				)
				continue
			}
			failed++
			fmt.Fprintf(
				w,
				"  FAIL  %-8s %-32s %d/%d facts, %d/%d chars, line claims %d wrong of %d\n",
				gate.name,
				c.ID,
				g.Found,
				g.Total,
				g.Chars,
				g.Budget,
				g.LineClaimsWrong,
				g.LineClaims,
			)
			if len(g.Missing) > 0 {
				fmt.Fprintf(w, "        missing: %s\n", strings.Join(g.Missing, " | "))
			}
			if len(g.Forbidden) > 0 {
				fmt.Fprintf(w, "        forbidden: %s\n", strings.Join(g.Forbidden, " | "))
			}
		}
	}
	if failed > 0 {
		return fmt.Errorf("%d gate checks failed", failed)
	}
	return nil
}

// writeList prints the sign-off document: each case with the corpus its
// query resolved to, the question, the facts, and the oracle.
func writeList(w *os.File, cases []Case, corpora map[string]Corpus) error {
	train, test := 0, 0
	for _, c := range cases {
		if c.Split == splitTest {
			test++
		} else {
			train++
		}
	}
	fmt.Fprintf(
		w,
		"# csl ask eval: %d cases (%d train, %d test), prompt %s\n",
		len(cases),
		train,
		test,
		promptHash(),
	)
	for _, c := range cases {
		corpus := corpora[c.ID]
		fmt.Fprintf(w, "\n## %s (%s): %s\n\n", c.ID, c.Split, c.Title)
		if c.Query != "" {
			fmt.Fprintf(w, "Query: `%s`", c.Query)
			if c.File != "" {
				fmt.Fprintf(w, " with file `%s`", c.File)
			}
			fmt.Fprintf(w, ", %d of %d matched files kept\n", len(corpus.Files), corpus.Matched)
		} else {
			fmt.Fprintf(w, "Files: given explicitly\n")
		}
		fmt.Fprintf(
			w,
			"Corpus: %d lines, %d bytes, about %dk tokens\n",
			corpus.Lines(),
			corpus.Bytes(),
			corpus.Bytes()/4000,
		)
		for _, f := range corpus.Files {
			fmt.Fprintf(w, "- %s (%d lines)\n", f.Path, f.Lines)
		}
		fmt.Fprintf(w, "\nQuestion: %s\n", c.Question)
		fmt.Fprintf(w, "Budget: %d chars\n", c.MaxAnswerChars)
		fmt.Fprintln(w, "Expected:")
		for _, e := range c.Expected {
			fmt.Fprintf(w, "- `%s`\n", e)
		}
		if len(c.Forbidden) > 0 {
			fmt.Fprintln(w, "Forbidden:")
			for _, f := range c.Forbidden {
				fmt.Fprintf(w, "- `%s`\n", f)
			}
		}
		fmt.Fprintf(w, "Oracle:\n%s", indent(c.Oracle))
		if c.Notes != "" {
			fmt.Fprintf(w, "Notes: %s\n", strings.TrimSpace(c.Notes))
		}
	}
	return nil
}

func indent(s string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(strings.TrimRight(s, "\n"), "\n") {
		b.WriteString("    " + line + "\n")
	}
	return b.String()
}

func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func writeJSONL[T any](path string, rows []T) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range rows {
		if err := enc.Encode(r); err != nil {
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
	defer f.Close()
	var out []T
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var v T
		if err := json.Unmarshal(sc.Bytes(), &v); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		out = append(out, v)
	}
	return out, sc.Err()
}

// regrade rescores an existing run's answers with the cases as they are
// now, so a pattern or budget edit can be judged without paying for the
// answers again. The corpus is re-read from the paths run.json recorded,
// and a file whose size changed since the run is reported.
func regrade(dir, casesDir string) error {
	cases, err := loadCases(casesDir)
	if err != nil {
		return err
	}
	byID := map[string]Case{}
	for _, c := range cases {
		byID[c.ID] = c
	}
	data, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return err
	}
	var meta runMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	corpora := map[string]Corpus{}
	for i, cm := range meta.Cases {
		c, ok := byID[cm.ID]
		if !ok {
			return fmt.Errorf("case %s is in the run but not under %s", cm.ID, casesDir)
		}
		corpus := Corpus{
			Repo: cm.Corpus.Repo, Root: cm.Corpus.Root, Query: cm.Corpus.Query, Matched: cm.Corpus.Matched,
		}
		for _, f := range cm.Corpus.Files {
			cf, err := readCorpusFile(cm.Corpus.Root, f.Path)
			if err != nil {
				return fmt.Errorf("%s: %w", cm.ID, err)
			}
			if cf.Bytes != f.Bytes {
				fmt.Fprintf(os.Stderr, "%s: %s is %d bytes now, was %d at run time\n",
					cm.ID, f.Path, cf.Bytes, f.Bytes)
			}
			corpus.Files = append(corpus.Files, cf)
		}
		corpora[cm.ID] = corpus
		meta.Cases[i].Budget = c.MaxAnswerChars
	}
	rows, err := readJSONL[Row](filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return err
	}
	errs, err := readJSONL[ErrRow](filepath.Join(dir, "errors.jsonl"))
	if err != nil {
		return err
	}
	armModel := map[string]string{}
	for _, a := range meta.Arms {
		armModel[a.ID] = a.Model
	}
	for i := range rows {
		c, ok := byID[rows[i].ID]
		if !ok {
			return fmt.Errorf("row for case %s has no case file", rows[i].ID)
		}
		rows[i].Grade = grade(c, corpora[rows[i].ID], rows[i].Answer)
		rows[i].CostUSD = ask.CostUSD(
			armModel[rows[i].Variant],
			rows[i].PromptTokens,
			rows[i].CompletionTokens,
		)
	}
	now := time.Now()
	meta.Regraded = &now
	if err := writeJSONL(filepath.Join(dir, "results.jsonl"), rows); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "run.json"), meta); err != nil {
		return err
	}
	return renderSummary(dir, meta, rows, errs)
}

// rerender rebuilds summary.md for an existing run directory.
func rerender(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		return err
	}
	var meta runMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return err
	}
	rows, err := readJSONL[Row](filepath.Join(dir, "results.jsonl"))
	if err != nil {
		return err
	}
	errs, err := readJSONL[ErrRow](filepath.Join(dir, "errors.jsonl"))
	if err != nil {
		return err
	}
	return renderSummary(dir, meta, rows, errs)
}

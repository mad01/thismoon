package ask

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/mad01/thismoon/kit/llm"
)

const (
	// callTimeout bounds one worker call. A full corpus is a few hundred
	// thousand tokens in, and a slow route can take minutes on it; a call
	// past this is hung, not slow.
	callTimeout = 5 * time.Minute
	// perRepoWorkers bounds the worker calls a per-repo question runs at
	// once.
	perRepoWorkers = 4
)

// Usage is the tokens the worker calls behind an answer reported, and their
// list price from the price table.
type Usage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	CostUSD          float64 `json:"cost_usd"`
}

func (u Usage) add(o Usage) Usage {
	return Usage{
		PromptTokens:     u.PromptTokens + o.PromptTokens,
		CompletionTokens: u.CompletionTokens + o.CompletionTokens,
		CostUSD:          u.CostUSD + o.CostUSD,
	}
}

// RepoAnswer is one repo's answer in a per-repo question. A failed call
// carries Error, and the other repos keep their answers.
type RepoAnswer struct {
	Repo   string `json:"repo"`
	Answer string `json:"answer,omitempty"`
	Error  string `json:"error,omitempty"`
	Usage  Usage  `json:"usage"`
}

// Relaxed is a query that matched no file as sent: its files come from the
// one rerun without the AND terms that match no file at all, the rerun
// csl_search makes.
type Relaxed struct {
	Query        string   `json:"query"`
	RelaxedQuery string   `json:"relaxed_query"`
	DroppedTerms []string `json:"dropped_terms"`
}

// Result is a question's answer with what it cost and what it read.
type Result struct {
	Answer  string       `json:"answer"`
	Backend string       `json:"backend"`
	Model   string       `json:"model"`
	Corpus  Corpus       `json:"corpus"`
	Relaxed []Relaxed    `json:"relaxed,omitempty"`
	Usage   Usage        `json:"usage"`
	PerRepo []RepoAnswer `json:"per_repo,omitempty"`
}

// Footer is the one line that closes a text answer: the corpus size, the
// backend and model, the list price, each query that was relaxed, and the
// corpus note when there is one.
func (r Result) Footer() string {
	corpus := fmt.Sprintf("%d files, %d lines", len(r.Corpus.Files), r.Corpus.Lines)
	if n := len(r.Corpus.Skipped); n > 0 {
		corpus += fmt.Sprintf(", %d skipped", n)
	}
	if r.Corpus.Truncated {
		corpus += ", truncated"
	}
	footer := fmt.Sprintf(
		"corpus: %s; %s/%s; $%.3f", corpus, r.Backend, r.Model, r.Usage.CostUSD,
	)
	for _, q := range r.Relaxed {
		footer += fmt.Sprintf("; relaxed: %s -> %s", q.Query, q.RelaxedQuery)
	}
	if r.Corpus.Note != "" {
		footer += "; note: " + r.Corpus.Note
	}
	return footer
}

// Text is the answer, a blank line, and the footer.
func (r Result) Text() string {
	return r.Answer + "\n\n" + r.Footer()
}

// WorkerFunc returns a backend whose exchanges go to observe.
type WorkerFunc func(observe func(llm.CallInfo)) (llm.Backend, error)

// Worker is the worker a question goes to, resolved once before the corpus
// is built: its backend name and model, and New, which builds the backend
// for one call. Every call gets its own backend because the observer that
// prices a call is fixed when the backend is built, and the calls of a
// per-repo question run at once.
type Worker struct {
	Name  string
	Model string
	New   WorkerFunc
}

// Request is a question over a built corpus.
type Request struct {
	Question string
	Corpus   Corpus
	// Relaxed names the queries whose files came from a relaxed rerun, so
	// the result can say so.
	Relaxed []Relaxed
	// PerRepo asks once per repo in the corpus, in repo rank order, and
	// concatenates the answers under a heading per repo.
	PerRepo bool
}

// Run answers the question over the corpus with the resolved worker.
func Run(ctx context.Context, req Request, w Worker) (Result, error) {
	if len(req.Corpus.Files) == 0 {
		return Result{}, errors.New("ask: the corpus is empty; nothing to send the worker")
	}
	res := Result{Backend: w.Name, Model: w.Model, Corpus: req.Corpus, Relaxed: req.Relaxed}
	var err error
	if req.PerRepo {
		res.PerRepo, err = askPerRepo(ctx, req.Question, req.Corpus.Files, w.New)
		if err != nil {
			return Result{}, err
		}
		res.Answer, res.Usage = joinRepoAnswers(res.PerRepo)
		return res, nil
	}
	res.Answer, res.Usage, err = askOnce(ctx, req.Question, req.Corpus.Files, w.New)
	if err != nil {
		return Result{}, err
	}
	return res, nil
}

// askOnce sends one worker call over files and prices what it reported.
// The observer runs inside Complete, on this goroutine, so the slice needs
// no lock.
func askOnce(
	ctx context.Context,
	question string,
	files []CorpusFile,
	newWorker WorkerFunc,
) (string, Usage, error) {
	var calls []llm.CallInfo
	w, err := newWorker(func(ci llm.CallInfo) { calls = append(calls, ci) })
	if err != nil {
		return "", Usage{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	answer, err := w.Complete(cctx, SystemPrompt, Message(question, workerFiles(files)))
	usage := priceCalls(w.Model(), calls)
	if err != nil {
		return "", usage, fmt.Errorf("ask: worker %s/%s: %w", w.Name(), w.Model(), err)
	}
	return strings.TrimSpace(answer), usage, nil
}

// priceCalls sums the tokens of every exchange and prices them as the model
// the backend asked for: a provider may report a dated or routed id the
// table does not know.
func priceCalls(model string, calls []llm.CallInfo) Usage {
	var u Usage
	for _, c := range calls {
		u.PromptTokens += c.PromptTokens
		u.CompletionTokens += c.CompletionTokens
	}
	u.CostUSD = CostUSD(model, u.PromptTokens, u.CompletionTokens)
	return u
}

// repoGroup is one repo's share of the corpus.
type repoGroup struct {
	repo  string
	files []CorpusFile
}

// groupByRepo splits files by repo, the repos in the order their first file
// ranks.
func groupByRepo(files []CorpusFile) []repoGroup {
	var groups []repoGroup
	at := map[string]int{}
	for _, f := range files {
		i, ok := at[f.Repo]
		if !ok {
			i = len(groups)
			at[f.Repo] = i
			groups = append(groups, repoGroup{repo: f.Repo})
		}
		groups[i].files = append(groups[i].files, f)
	}
	return groups
}

// askPerRepo asks once per repo, perRepoWorkers at a time. A failed call
// fills its own entry; the question fails only when every call did, since
// the others' answers are already paid for. A panic is recovered into the
// entry too: errgroup does not recover one, and a panic here would take
// csl mcp down.
func askPerRepo(
	ctx context.Context,
	question string,
	files []CorpusFile,
	newWorker WorkerFunc,
) ([]RepoAnswer, error) {
	groups := groupByRepo(files)
	answers := make([]RepoAnswer, len(groups))
	var g errgroup.Group
	g.SetLimit(perRepoWorkers)
	for i, grp := range groups {
		g.Go(func() error {
			answers[i] = RepoAnswer{Repo: grp.repo}
			defer func() {
				if p := recover(); p != nil {
					fmt.Fprintf(
						os.Stderr,
						"csl ask: panic asking %s: %v\n%s",
						grp.repo,
						p,
						debug.Stack(),
					)
					answers[i].Error = fmt.Sprintf("internal error: %v", p)
				}
			}()
			text, usage, err := askOnce(ctx, question, grp.files, newWorker)
			answers[i].Answer, answers[i].Usage = text, usage
			if err != nil {
				answers[i].Error = err.Error()
			}
			return nil
		})
	}
	_ = g.Wait()
	for _, a := range answers {
		if a.Error == "" {
			return answers, nil
		}
	}
	return nil, fmt.Errorf("ask: every per-repo worker call failed; first: %s", answers[0].Error)
}

// joinRepoAnswers concatenates the per-repo answers under an org/repo
// heading each, in repo rank order, and sums their usage. The order and the
// text are deterministic, and no second model call runs.
//
// A merge pass is deliberately absent until the eval harness shows the concatenation falls short.
func joinRepoAnswers(answers []RepoAnswer) (string, Usage) {
	var (
		parts []string
		total Usage
	)
	for _, a := range answers {
		body := a.Answer
		if a.Error != "" {
			body = "error: " + a.Error
		}
		parts = append(parts, "## "+a.Repo+"\n\n"+body)
		total = total.add(a.Usage)
	}
	return strings.Join(parts, "\n\n"), total
}

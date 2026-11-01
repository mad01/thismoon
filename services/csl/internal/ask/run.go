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

// Result is a question's answer with what it cost and what it read.
type Result struct {
	Answer  string       `json:"answer"`
	Backend string       `json:"backend"`
	Model   string       `json:"model"`
	Corpus  Corpus       `json:"corpus"`
	Usage   Usage        `json:"usage"`
	PerRepo []RepoAnswer `json:"per_repo,omitempty"`
}

// Footer is the one line that closes a text answer: the corpus size, the
// backend and model, and the list price.
func (r Result) Footer() string {
	corpus := fmt.Sprintf("%d files, %d lines", len(r.Corpus.Files), r.Corpus.Lines)
	if n := len(r.Corpus.Skipped); n > 0 {
		corpus += fmt.Sprintf(", %d skipped", n)
	}
	return fmt.Sprintf("corpus: %s; %s/%s; $%.3f", corpus, r.Backend, r.Model, r.Usage.CostUSD)
}

// Text is the answer, a blank line, and the footer.
func (r Result) Text() string {
	return r.Answer + "\n\n" + r.Footer()
}

// WorkerFunc returns a backend whose exchanges go to observe. Every worker
// call gets its own, so concurrent calls never share an observer.
type WorkerFunc func(observe func(llm.CallInfo)) (llm.Backend, error)

// Request is a question over a built corpus.
type Request struct {
	Question string
	Corpus   Corpus
	// PerRepo asks once per repo in the corpus, in repo rank order, and
	// concatenates the answers under a heading per repo.
	PerRepo bool
}

// Run answers the question over the corpus. The worker is resolved once
// before any call, so a missing backend fails fast with nothing spent.
func Run(ctx context.Context, req Request, newWorker WorkerFunc) (Result, error) {
	if len(req.Corpus.Files) == 0 {
		return Result{}, errors.New("ask: the corpus is empty; nothing to send the worker")
	}
	probe, err := newWorker(nil)
	if err != nil {
		return Result{}, err
	}
	res := Result{Backend: probe.Name(), Model: probe.Model(), Corpus: req.Corpus}
	if req.PerRepo {
		res.PerRepo, err = askPerRepo(ctx, req.Question, req.Corpus.Files, newWorker)
		if err != nil {
			return Result{}, err
		}
		res.Answer, res.Usage = joinRepoAnswers(res.PerRepo)
		return res, nil
	}
	res.Answer, res.Usage, err = askOnce(ctx, req.Question, req.Corpus.Files, newWorker)
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

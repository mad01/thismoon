package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/llm"
	"github.com/mad01/thismoon/services/csl/internal/ask"
	"github.com/mad01/thismoon/services/csl/internal/lexical"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// csl_ask is the bulk reader: a question over the files a search selects,
// answered by a hosted worker model so the files never enter the caller's
// context. The search runs csl_search_multi's path (runSearchMulti); the
// corpus, the worker and the price live in internal/ask. `csl ask` calls Ask
// with the same request, so the tool and the CLI build one corpus.

// maxAskQueries caps the queries one question may select files with, the
// same cap csl_search_multi has.
const maxAskQueries = maxMultiQueries

// skipReasonsShown is how many skipped files an empty-corpus error names.
const skipReasonsShown = 5

// newAskWorker resolves the worker backend. Tests replace it with a fake, so
// no test spends a model call.
var newAskWorker = ask.NewWorker

// askInput is the typed input for the csl_ask tool.
type askInput struct {
	Question string   `json:"question"            jsonschema:"the question to answer from the selected files"`
	Queries  []string `json:"queries"             jsonschema:"1 to 10 zoekt queries in csl_search syntax; the corpus is the union of the files they match, in rank order"`
	Repo     string   `json:"repo,omitempty"      jsonschema:"restrict every query to repo names matching this case-insensitive regex"`
	Lang     string   `json:"lang,omitempty"      jsonschema:"restrict every query to files of this language"`
	File     string   `json:"file,omitempty"      jsonschema:"restrict every query to file paths matching this regex"`
	MaxFiles int      `json:"max_files,omitempty" jsonschema:"most files the worker reads (default 20, max 100)"`
	MaxLines int      `json:"max_lines,omitempty" jsonschema:"most lines across those files (default 6000, max 20000); each file is read whole or skipped"`
	PerRepo  bool     `json:"per_repo,omitempty"  jsonschema:"ask once per repo and return the answers under an org/repo heading each"`
	formatParam
}

func registerAskTools(s *mcp.Server) {
	addFormattedTool(s, &mcp.Tool{
		Name: "csl_ask",
		Description: "Answer a question over the files a zoekt search selects. A cheap hosted worker model reads the files, so they never enter your context: only its answer comes back, with a footer giving the corpus size, the backend and model, and the list price. " +
			"Use it for a question over many files, or over one large file, when you need only the answer: 'which flags does each command accept', 'how does each service configure retries', 'what does this 3000-line file do'. " +
			"Do not use it for lines you will edit: an edit needs the real content through csl_read or a ranged Read. To locate code, csl_search is cheaper. " +
			"queries takes 1 to 10 zoekt queries in csl_search syntax, and repo, lang and file narrow every one of them. " +
			"The corpus is the union of the matching files in rank order, taking turns across queries, each file read whole and capped at max_files (default 20, max 100) and max_lines (default 6000, max 20000); files that do not fit are listed in corpus.skipped with the reason. " +
			"per_repo asks once per repo and returns the answers under org/repo headings. " +
			"The worker cites path:line, sometimes by the bare file name; corpus.files names the repo and full path for csl_read. " +
			"When no query matches a file, the call fails with each query's zero-result hint and no model call is made.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
		},
	}, handleAsk, ask.Result.Text)
}

func handleAsk(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in askInput,
) (*mcp.CallToolResult, ask.Result, error) {
	res, err := Ask(ctx, AskRequest{
		Question: in.Question,
		Queries:  in.Queries,
		Repo:     in.Repo,
		Lang:     in.Lang,
		File:     in.File,
		Limits:   ask.Limits{MaxFiles: in.MaxFiles, MaxLines: in.MaxLines},
		PerRepo:  in.PerRepo,
	})
	if err != nil {
		return nil, ask.Result{}, err
	}
	return nil, res, nil
}

// AskRequest is one bulk-reader question: the csl_ask tool's input, and
// what `csl ask` builds from its flags.
type AskRequest struct {
	Question string
	// Queries select the corpus by search. FileList names it instead, as
	// org/repo/path lines in the format `csl search` prints. Exactly one of
	// the two is set.
	Queries  []string
	FileList []string
	// Repo, Lang and File narrow every query; a file list takes none.
	Repo string
	Lang string
	File string
	// Limits cap the corpus; zero fields take the defaults.
	Limits  ask.Limits
	PerRepo bool
	// Surface words the malformed-query fix and the zero-result hint for the
	// caller's front end. The zero value is the MCP tool's.
	Surface lexical.Surface
}

// Ask answers a question over the files the request selects. It checks the
// request and resolves the worker before any search runs, so a bad query or
// a missing backend costs nothing, and it never calls the worker over an
// empty corpus.
func Ask(ctx context.Context, req AskRequest) (ask.Result, error) {
	if req.Surface.QueryArg == "" {
		req.Surface = mcpSurface
	}
	lim, err := checkAskRequest(req)
	if err != nil {
		return ask.Result{}, err
	}
	cfg, err := config.Load()
	if err != nil {
		return ask.Result{}, fmt.Errorf("load csl config: %w", err)
	}
	settings := ask.SettingsFrom(cfg)
	if _, err := newAskWorker(settings, nil); err != nil {
		return ask.Result{}, err
	}
	corpus, err := askCorpus(ctx, req, lim)
	if err != nil {
		return ask.Result{}, err
	}
	worker := func(observe func(llm.CallInfo)) (llm.Backend, error) {
		return newAskWorker(settings, observe)
	}
	return ask.Run(
		ctx,
		ask.Request{Question: req.Question, Corpus: corpus, PerRepo: req.PerRepo},
		worker,
	)
}

// checkAskRequest refuses a request no search should run for and resolves
// its limits.
func checkAskRequest(req AskRequest) (ask.Limits, error) {
	if strings.TrimSpace(req.Question) == "" {
		return ask.Limits{}, errors.New("question is empty; ask what the files should answer")
	}
	if len(req.FileList) > 0 {
		if len(req.Queries) > 0 {
			return ask.Limits{}, errors.New("set queries or a file list, not both")
		}
		if req.Repo != "" || req.Lang != "" || req.File != "" {
			return ask.Limits{}, errors.New(
				"repo, lang and file narrow queries; filter the search that made the file list instead",
			)
		}
		return req.Limits.Resolve()
	}
	n := len(req.Queries)
	switch {
	case n == 0:
		return ask.Limits{}, fmt.Errorf(
			"queries is empty; send 1 to %d zoekt queries",
			maxAskQueries,
		)
	case n > maxAskQueries:
		return ask.Limits{}, fmt.Errorf(
			"csl_ask takes at most %d queries per question, got %d", maxAskQueries, n,
		)
	}
	for i, q := range req.Queries {
		if m := lexical.CheckShape(q, req.Surface); m != nil {
			return ask.Limits{}, fmt.Errorf("query %d/%d: %w", i+1, n, m)
		}
	}
	return req.Limits.Resolve()
}

// askCorpus selects the files, by search or from the file list, and reads
// them into a corpus.
func askCorpus(ctx context.Context, req AskRequest, lim ask.Limits) (ask.Corpus, error) {
	if len(req.FileList) > 0 {
		env, err := newSearchEnv()
		if err != nil {
			return ask.Corpus{}, err
		}
		names := slices.Collect(maps.Keys(env.repoPaths))
		hits, err := ask.ParseFileList(req.FileList, names)
		if err != nil {
			return ask.Corpus{}, err
		}
		if len(hits) == 0 {
			return ask.Corpus{}, errors.New(
				"the file list names no file, so nothing was sent to the worker",
			)
		}
		return buildAskCorpus(hits, env.repoPaths, lim)
	}
	env, err := newMultiSearchEnv()
	if err != nil {
		return ask.Corpus{}, err
	}
	env.surface = req.Surface
	lists, err := searchAskQueries(ctx, env, req, lim.MaxFiles)
	if err != nil {
		return ask.Corpus{}, err
	}
	return buildAskCorpus(ask.Union(lists), env.repoPaths, lim)
}

// searchAskQueries runs the queries the way csl_search_multi does, each in
// files mode with limit files, and returns each query's files in rank order.
// No query can put more than the file cap into the corpus, so limit is that
// cap. A failed query fails the question: an answer over the files of the
// queries that worked would read as an answer over all of them.
func searchAskQueries(
	ctx context.Context,
	env searchEnv,
	req AskRequest,
	limit int,
) ([][]ask.Hit, error) {
	queries := make([]searchMultiQuery, len(req.Queries))
	for i, q := range req.Queries {
		queries[i] = searchMultiQuery{
			Query: q, Repo: req.Repo, Lang: req.Lang, File: req.File, Limit: limit,
		}
	}
	out := runSearchMulti(ctx, env, queries)
	lists := make([][]ask.Hit, len(out.Results))
	found := 0
	for i, r := range out.Results {
		if r.Error != "" {
			return nil, fmt.Errorf("query %d/%d %q: %s", i+1, len(out.Results), r.Query, r.Error)
		}
		for _, f := range r.Files {
			lists[i] = append(lists[i], ask.Hit{Repo: f.Repo, Path: f.Path})
		}
		found += len(lists[i])
	}
	if found == 0 {
		return nil, zeroFilesError(out.Results)
	}
	return lists, nil
}

// zeroFilesError carries each query's zero-result hint, the diagnosis
// csl_search gives, so the caller can fix the query without a second call.
func zeroFilesError(results []searchMultiResult) error {
	lines := []string{"no query matched a file, so nothing was sent to the worker"}
	for i, r := range results {
		lines = append(lines, fmt.Sprintf("query %d/%d: %s", i+1, len(results), r.Query))
		if r.searchOutput == nil {
			continue
		}
		for _, l := range r.ZeroHint.Lines() {
			lines = append(lines, "  "+l)
		}
	}
	return errors.New(strings.Join(lines, "\n"))
}

// buildAskCorpus reads the hits into a corpus and refuses one the caps left
// empty, naming why the first files were skipped.
func buildAskCorpus(hits []ask.Hit, roots map[string]string, lim ask.Limits) (ask.Corpus, error) {
	corpus := ask.Build(hits, roots, lim)
	if len(corpus.Files) > 0 {
		return corpus, nil
	}
	lines := []string{fmt.Sprintf(
		"every one of the %d matching files was skipped, so nothing was sent to the worker:",
		corpus.FilesTotal,
	)}
	for _, s := range corpus.Skipped[:min(len(corpus.Skipped), skipReasonsShown)] {
		lines = append(lines, "  "+s.Name()+": "+s.Reason)
	}
	return ask.Corpus{}, errors.New(strings.Join(lines, "\n"))
}

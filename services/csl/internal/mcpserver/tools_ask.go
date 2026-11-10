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
	"github.com/mad01/thismoon/services/csl/internal/worktree"
)

// csl_ask is the bulk reader: a question over the files a search selects,
// answered by a hosted worker model so the files never enter the caller's
// context. The search runs csl_search_multi's path (runSearchMulti); the
// corpus, the worker and the price live in internal/ask. `csl ask` calls Ask
// with the same request, so the tool and the CLI build one corpus.

// maxAskQueries caps the queries one question may select files with, the
// same cap csl_search_multi has.
const maxAskQueries = maxMultiQueries

// askSearchLimit is how many files each query may select, the csl_ask
// eval's search limit. It sits well past the corpus caps, so a file the
// corpus refuses is replaced from further down the ranking, and a query
// that reaches it marks the corpus truncated.
const askSearchLimit = 500

// AskMinLines is the file length from which csl_ask beats a whole-file read
// when only an answer is needed. The csl_ask and csl_read descriptions and
// `csl ask --help` state it. belt's read-size hint fires at the same length
// (readSizeMinLines in tools/belt/internal/hint/readsize.go), so the two
// move together.
const AskMinLines = 300

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
	worktreeParam
	formatParam
}

// registerAskTools registers csl_ask, alwaysLoad when alwaysLoad is set.
// Deferred, csl_ask was never picked over a whole-file read of a named
// file, so its description has to be in view from the start. That is worth
// its schema in every session only where a worker resolves; elsewhere every
// call fails naming what to set, and the tool stays deferred.
func registerAskTools(s *mcp.Server, alwaysLoad bool) {
	tool := &mcp.Tool{
		Name: "csl_ask",
		Description: fmt.Sprintf(
			"Pick this over csl_read or Read when a file runs to %d lines or more, or a question spans several files, and you need only the answer, not the lines. "+
				"Try it before you read: a thin answer costs one cheap call, and the lines are still there to read after. ",
			AskMinLines,
		) +
			"A cheap hosted worker model reads the files a zoekt search selects, so they never enter your context: only its answer comes back, with a footer giving the corpus size, the backend and model, and the list price. " +
			"Examples: 'what does this 3000-line file do', 'which flags does each command accept', 'how does each service configure retries'. " +
			"Not for lines you will edit: an edit needs the real content through csl_read or a ranged Read. To locate code, csl_search is cheaper. " +
			"queries takes 1 to 10 zoekt queries in csl_search syntax, and repo, lang and file narrow every one; a named file or directory is a query like f:^path/to/file\\.go$ or f:^path/to/dir/. " +
			"The corpus is the union of the matching files in rank order, taking turns across queries, each read whole up to max_files (default 20, max 100) and max_lines (default 6000, max 20000). A file that cannot go in is listed in corpus.skipped with the reason and the next takes its place; corpus.truncated is true when matching files were never considered. " +
			"A query whose AND terms partly match nothing is rerun without them, as csl_search does, and listed in relaxed. " +
			"per_repo asks once per repo and returns the answers under org/repo headings. " +
			"The worker cites path:line, sometimes by the bare file name; corpus.files names the repo and full path for csl_read. " +
			"When no query matches a file, the call fails with each query's zero-result hint and makes no model call.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(true),
		},
	}
	if alwaysLoad {
		tool.Meta = mcp.Meta{alwaysLoadKey: true}
	}
	addFormattedTool(s, tool, handleAsk, ask.Result.Text)
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
		Worktree: in.Worktree,
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
	// Worktree names a linked worktree, by path or org/repo@branch name.
	// Queries then search its own index and replace Repo; a file list may
	// name its files beside the main checkouts'.
	Worktree string
	// Surface words the malformed-query fix and the zero-result hint for the
	// caller's front end. The zero value is the MCP tool's.
	Surface lexical.Surface
}

// Ask answers a question over the files the request selects. It checks the
// request and resolves the worker before any search runs, so a bad query or
// a missing backend costs nothing, and it never calls the worker over an
// empty corpus. The config is loaded once, for the worker and the search.
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
	worker, err := resolveAskWorker(ask.SettingsFrom(cfg))
	if err != nil {
		return ask.Result{}, err
	}
	corpus, relaxed, err := askCorpus(ctx, cfg, req, lim)
	if err != nil {
		return ask.Result{}, err
	}
	return ask.Run(ctx, ask.Request{
		Question: req.Question,
		Corpus:   corpus,
		Relaxed:  relaxed,
		PerRepo:  req.PerRepo,
	}, worker)
}

// resolveAskWorker resolves the worker once, naming its backend and model.
// Each call still builds its own backend from the same settings, since the
// observer that prices a call is fixed when the backend is built.
func resolveAskWorker(settings ask.Settings) (ask.Worker, error) {
	b, err := newAskWorker(settings, nil)
	if err != nil {
		return ask.Worker{}, err
	}
	return ask.Worker{
		Name:  b.Name(),
		Model: b.Model(),
		New: func(observe func(llm.CallInfo)) (llm.Backend, error) {
			return newAskWorker(settings, observe)
		},
	}, nil
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
// them into a corpus. It returns the queries whose files came from a
// relaxed rerun beside it.
func askCorpus(
	ctx context.Context,
	cfg *config.Config,
	req AskRequest,
	lim ask.Limits,
) (ask.Corpus, []ask.Relaxed, error) {
	if len(req.FileList) > 0 {
		corpus, err := fileListCorpus(cfg, req.FileList, req.Worktree, lim)
		return corpus, nil, err
	}
	env, err := newMultiSearchEnvIn(cfg, req.Worktree)
	if err != nil {
		return ask.Corpus{}, nil, err
	}
	defer env.close()
	env.surface = req.Surface
	sel, err := searchAskQueries(ctx, env, req, askSearchLimit)
	if err != nil {
		return ask.Corpus{}, nil, err
	}
	corpus, err := buildAskCorpus(ask.Union(sel.lists), env.repoPaths, lim)
	if err != nil {
		return ask.Corpus{}, nil, err
	}
	corpus.Truncated = corpus.Truncated || sel.truncated
	corpus.Note = env.note
	return corpus, sel.relaxed, nil
}

// fileListCorpus reads the files a --files-from list names into a corpus.
// With wt set, the list may also name the worktree's files, under the
// org/repo@branch name a worktree search prints.
func fileListCorpus(
	cfg *config.Config,
	list []string,
	wt string,
	lim ask.Limits,
) (ask.Corpus, error) {
	env, err := newSearchEnvIn(cfg)
	if err != nil {
		return ask.Corpus{}, err
	}
	if wt != "" {
		repo, err := worktree.Resolve(cfg, wt)
		if err != nil {
			return ask.Corpus{}, err
		}
		env.repoPaths[repo.Name] = repo.Path
	}
	names := slices.Collect(maps.Keys(env.repoPaths))
	hits, err := ask.ParseFileList(list, names)
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

// askSelection is what the queries selected: each query's files in rank
// order, the queries that matched only after a relaxed rerun, and whether
// any query's search stopped at its limit.
type askSelection struct {
	lists     [][]ask.Hit
	relaxed   []ask.Relaxed
	truncated bool
}

// searchAskQueries runs the queries the way csl_search_multi does, each in
// files mode with limit files. A query rerun relaxed keeps its files, as on
// csl_search, and is named in the selection. A failed query fails the
// question: an answer over the files of the queries that worked would read
// as an answer over all of them.
func searchAskQueries(
	ctx context.Context,
	env searchEnv,
	req AskRequest,
	limit int,
) (askSelection, error) {
	queries := make([]searchMultiQuery, len(req.Queries))
	for i, q := range req.Queries {
		queries[i] = searchMultiQuery{
			Query: q, Repo: req.Repo, Lang: req.Lang, File: req.File, Limit: limit,
		}
	}
	out := runSearchMulti(ctx, env, queries)
	sel := askSelection{lists: make([][]ask.Hit, len(out.Results)), truncated: out.Truncated}
	found := 0
	for i, r := range out.Results {
		if r.Error != "" {
			return askSelection{}, fmt.Errorf(
				"query %d/%d %q: %s", i+1, len(out.Results), r.Query, r.Error,
			)
		}
		for _, f := range r.Files {
			sel.lists[i] = append(sel.lists[i], ask.Hit{Repo: f.Repo, Path: f.Path})
		}
		found += len(sel.lists[i])
		if r.RelaxedQuery != "" {
			sel.relaxed = append(sel.relaxed, ask.Relaxed{
				Query:        r.Query,
				RelaxedQuery: r.RelaxedQuery,
				DroppedTerms: r.DroppedTerms,
			})
		}
	}
	if found == 0 {
		return askSelection{}, zeroFilesError(out.Results)
	}
	return sel, nil
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

// buildAskCorpus reads the hits into a corpus and refuses an empty one,
// naming why the first files were skipped.
func buildAskCorpus(hits []ask.Hit, roots map[string]string, lim ask.Limits) (ask.Corpus, error) {
	corpus := ask.Build(hits, roots, lim)
	if len(corpus.Files) > 0 {
		return corpus, nil
	}
	lines := []string{fmt.Sprintf(
		"none of the %d matching files could go into the corpus, so nothing was sent to the worker:",
		corpus.Matched,
	)}
	for _, s := range corpus.Skipped[:min(len(corpus.Skipped), skipReasonsShown)] {
		lines = append(lines, "  "+s.Name()+": "+s.Reason)
	}
	return ask.Corpus{}, errors.New(strings.Join(lines, "\n"))
}

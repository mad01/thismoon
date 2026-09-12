package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/services/keeper-of-facts/internal/client"
)

// handlers carries the dependencies shared by all kof tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views assertions in a browser
	checks func(ctx context.Context) []doctor.Check
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_assert",
		Description: "Record a one-sentence assertion about how a system behaves and ground it in evidence pins. " +
			"An assertion is a durable, checkable claim you derived this session (how some code acts, an approach that failed, a settled decision) so a later session can trust it without re-deriving it. " +
			"Each pin captures a line range in a repo working tree, hashed the moment you assert; `kof_check` re-hashes them later and flips the assertion stale if the pinned code changed. " +
			"At least one pin is REQUIRED: pins are what let kof tell whether the claim still holds, so an assertion with no pin is rejected. " +
			"`kind` must be EXACTLY one of: code-behavior | dead-end | preference | decision | machine-state | open-thread. Write \"code-behavior\", not \"behavior\". " +
			"`confidence` must be EXACTLY one of: verified | derived | hint, not high/medium/low. Any other value in either field is rejected. " +
			"Keep the returned id; it is the handle for kof_get / kof_retract / kof_check.",
		// Every call stores one more assertion under a new id and leaves the
		// existing ones untouched, so the write is additive and not idempotent.
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(false),
			IdempotentHint:  false,
			OpenWorldHint:   new(false),
		},
	}, h.handleAssert)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_query",
		Description: "List assertions, newest first, to see what past sessions already established before deriving it again. " +
			"Filter by `subject` (prefix match on the namespaced key), `kind`, and/or `status` (fresh | stale | retracted); omit all to list everything. " +
			"Returns each assertion's id, statement, kind, confidence, and status. Follow up with kof_get for the full record including pins. " +
			"On zero results the response carries zero_result_hint (how many stored subjects the prefix matched, whether other filters excluded everything); read it before assuming nothing is stored.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, h.handleQuery)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_recall",
		Description: "Search the assertion store by meaning for the assertions that bear on a free-form question. " +
			"A one-shot model judge ranks the whole store against the question and returns the relevant assertions in rank order. " +
			"Use this when you don't know the subject key: \"why does the store not lose writes\" finds the single-writer assertion even though no word matches. " +
			"Stale assertions are included and marked (treat them as needing re-verification); retracted ones never appear. " +
			"`question` is the ONLY parameter: there is no subject/kind/status filtering here; those params belong to kof_query. " +
			"Takes a few seconds. If the judge is unavailable the error says so; fall back to kof_query.",
		// The store is only read, but the ranking runs through the `claude -p`
		// judge on serve's host, so this is the one kof tool that depends on a
		// remote model and fails when the machine is offline.
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(true),
			ReadOnlyHint:  true,
		},
	}, h.handleRecall)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kof_get",
		Description: "Get one assertion's full detail by id, including every evidence pin (repo, file, line range, content hash, resolved commit) and its provenance (which session derived it, when, and the token cost).",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, h.handleGet)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_retract",
		Description: "Withdraw an assertion by id, a terminal action that records the claim as no longer holding. " +
			"`note` is REQUIRED: give the reason or the counter-evidence, since the retracted record stays in the store as the explanation of why the claim was dropped. " +
			"Use this instead of leaving a wrong assertion to go stale when you have direct evidence it is false.",
		// Retracting supersedes an existing assertion and there is no
		// un-retract, so the effect is destructive; retracting an
		// already-retracted id is a no-op, so it is idempotent.
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, h.handleRetract)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_check",
		Description: "Re-hash an assertion's evidence pins against the current working tree and report whether it still holds. " +
			"Pass `id` to check one assertion, or omit `id` to re-check every non-retracted assertion. " +
			"Returns counts of how many were checked, how many are fresh, how many are stale, and how many flipped between the two on this run. " +
			"A stale result means the pinned code changed and the claim needs another look, not that it is necessarily wrong.",
		// Checking rewrites the status, stale reason, and checked-at of the
		// assertions it visits, so it is not read-only and not purely
		// additive; re-running it against an unchanged working tree settles on
		// the same statuses, so it is idempotent.
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  true,
			OpenWorldHint:   new(false),
		},
	}, h.handleCheck)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_doctor",
		Description: "Diagnose kof itself: is serve reachable, is the assertion log readable, is the running build " +
			"the installed one. Returns one result per check with `ok` false if any failed. " +
			"Read-only: it probes, it changes nothing. " +
			"This is about the service, not the assertions: use `kof_check` to re-verify the claims in the store, " +
			"and this when a kof tool errors or comes back empty and you need to know whether serve is even up.",
		Annotations: &mcp.ToolAnnotations{
			OpenWorldHint: new(false),
			ReadOnlyHint:  true,
		},
	}, h.handleDoctor)
}

// out is the tool response shape for the single-assertion tools: the assertion
// plus the web URL to view it.
type out struct {
	Assertion client.Assertion `json:"assertion"`
	URL       string           `json:"url"       jsonschema:"web page where the user can view assertions"`
}

func (h *handlers) one(a client.Assertion) out { return out{Assertion: a, URL: h.webURL} }

// ── assert ──

type pinInput struct {
	RepoPath  string `json:"repo_path"  jsonschema:"absolute path to the repo working tree the evidence lives in, e.g. /Users/me/code/src/github.com/mad01/thismoon"`
	File      string `json:"file"       jsonschema:"repo-relative path to the file"`
	StartLine int    `json:"start_line" jsonschema:"first line of the evidence range (1-based, inclusive)"`
	EndLine   int    `json:"end_line"   jsonschema:"last line of the evidence range (1-based, inclusive; >= start_line)"`
}

type assertInput struct {
	Kind       string     `json:"kind"                  jsonschema:"EXACTLY one of: code-behavior | dead-end | preference | decision | machine-state | open-thread. Meanings: code-behavior (how code acts; write code-behavior, not behavior), dead-end (an approach that failed), preference (a stated way of working), decision (a settled choice), machine-state (a fact about the local machine), open-thread (unfinished work worth resuming)"`
	Subject    string     `json:"subject"               jsonschema:"namespaced key the claim is about, e.g. repo:mad01/thismoon/services/events"`
	Statement  string     `json:"statement"             jsonschema:"the claim in one sentence"`
	Confidence string     `json:"confidence"            jsonschema:"EXACTLY one of: verified | derived | hint, not high/medium/low. Meanings: verified (checked against a primary source), derived (reasoned from evidence), hint (a weak signal)"`
	SessionID  string     `json:"session_id"            jsonschema:"identifier of the session deriving this assertion"`
	CostTokens int        `json:"cost_tokens,omitempty" jsonschema:"optional token cost of deriving the assertion"`
	Links      []string   `json:"links,omitempty"       jsonschema:"optional related URLs (tickets, PRs, docs)"`
	Pins       []pinInput `json:"pins"                  jsonschema:"evidence pins grounding the claim in hashed line ranges; at least one is REQUIRED"`
}

func (h *handlers) handleAssert(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in assertInput,
) (*mcp.CallToolResult, out, error) {
	pins := make([]client.PinRef, len(in.Pins))
	for i, p := range in.Pins {
		pins[i] = client.PinRef{
			RepoPath:  p.RepoPath,
			File:      p.File,
			StartLine: p.StartLine,
			EndLine:   p.EndLine,
		}
	}
	a, err := h.client.Assert(client.AssertBody{
		Kind:       in.Kind,
		Subject:    in.Subject,
		Statement:  in.Statement,
		Confidence: in.Confidence,
		SessionID:  in.SessionID,
		CostTokens: in.CostTokens,
		Links:      in.Links,
		Pins:       pins,
	})
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(a), nil
}

// ── recall ──

type recallInput struct {
	Question string `json:"question" jsonschema:"the free-form question to rank the store against, e.g. 'what do we know about JSONL write races'"`
}

func (h *handlers) handleRecall(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in recallInput,
) (*mcp.CallToolResult, queryOutput, error) {
	as, err := h.client.Recall(in.Question)
	if err != nil {
		return nil, queryOutput{}, err
	}
	return nil, queryOutput{Assertions: as, URL: h.webURL}, nil
}

// ── query ──

type queryInput struct {
	Subject string `json:"subject,omitempty" jsonschema:"optional prefix match on the subject key"`
	Kind    string `json:"kind,omitempty"    jsonschema:"optional kind filter: code-behavior | dead-end | preference | decision | machine-state | open-thread"`
	Status  string `json:"status,omitempty"  jsonschema:"optional status filter: fresh | stale | retracted"`
}

type queryOutput struct {
	Assertions []client.Assertion `json:"assertions"`
	URL        string             `json:"url"`
	ZeroHint   *queryZeroHint     `json:"zero_result_hint,omitempty" jsonschema:"set only on zero results: what the filters actually matched against the store, so an empty list is distinguishable from a wrong subject prefix"`
}

// queryZeroHint explains an empty kof_query so an agent can tell "nothing is
// stored about this" from "the subject prefix or filters missed". Additive:
// it appears only when the query returned nothing.
type queryZeroHint struct {
	SubjectPrefix   string   `json:"subject_prefix,omitempty" jsonschema:"the subject prefix that was applied (prefix-from-the-left match)"`
	SubjectsMatched int      `json:"subjects_matched"         jsonschema:"distinct stored subjects the prefix matched; 0 means the prefix is the problem, not the store"`
	SubjectsStored  int      `json:"subjects_stored"          jsonschema:"distinct subjects in the store"`
	StoreAssertions int      `json:"store_assertions"         jsonschema:"total assertions in the store"`
	Notes           []string `json:"notes,omitempty"          jsonschema:"targeted suggestions, e.g. a shorter prefix to try"`
}

func (h *handlers) handleQuery(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in queryInput,
) (*mcp.CallToolResult, queryOutput, error) {
	as, err := h.client.List(in.Subject, in.Kind, in.Status)
	if err != nil {
		return nil, queryOutput{}, err
	}
	outp := queryOutput{Assertions: as, URL: h.webURL}
	if len(as) == 0 {
		outp.ZeroHint = h.buildQueryZeroHint(in)
	}
	return nil, outp, nil
}

// buildQueryZeroHint compares the query's filters against the unfiltered
// store. Best-effort: it returns nil when the extra lookup fails, leaving the
// plain empty result.
func (h *handlers) buildQueryZeroHint(in queryInput) *queryZeroHint {
	all, err := h.client.List("", "", "")
	if err != nil {
		return nil
	}

	subjects := make(map[string]struct{})
	matchedSubjects := make(map[string]struct{})
	matchedAssertions := 0
	for _, a := range all {
		subjects[a.Subject] = struct{}{}
		if in.Subject != "" && strings.HasPrefix(a.Subject, in.Subject) {
			matchedSubjects[a.Subject] = struct{}{}
			matchedAssertions++
		}
	}

	hint := &queryZeroHint{
		SubjectPrefix:   in.Subject,
		SubjectsMatched: len(matchedSubjects),
		SubjectsStored:  len(subjects),
		StoreAssertions: len(all),
	}
	hint.Notes = queryZeroNotes(in, hint, matchedAssertions)
	return hint
}

// queryZeroNotes phrases the one note that names why the query came back
// empty, given the store stats the hint carries.
func queryZeroNotes(in queryInput, hint *queryZeroHint, matchedAssertions int) []string {
	if hint.StoreAssertions == 0 {
		return []string{"the store is empty; nothing has been asserted yet"}
	}
	if in.Subject == "" {
		return []string{fmt.Sprintf(
			"no assertions matched kind=%q status=%q; the store holds %d assertions",
			in.Kind, in.Status, hint.StoreAssertions,
		)}
	}
	if hint.SubjectsMatched == 0 {
		note := fmt.Sprintf(
			"subject prefix %q matched none of the %d stored subjects; matching is prefix-from-the-left, so try a shorter prefix",
			in.Subject,
			hint.SubjectsStored,
		)
		if i := strings.LastIndex(strings.TrimRight(in.Subject, "/"), "/"); i > 0 {
			note += fmt.Sprintf(", e.g. %q", in.Subject[:i])
		}
		return []string{note}
	}
	return []string{fmt.Sprintf(
		"subject prefix matched %d subjects (%d assertions) but the kind/status filters excluded them all",
		hint.SubjectsMatched,
		matchedAssertions,
	)}
}

// ── get ──

type idInput struct {
	ID string `json:"id" jsonschema:"assertion id returned by kof_assert"`
}

func (h *handlers) handleGet(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in idInput,
) (*mcp.CallToolResult, out, error) {
	a, err := h.client.Get(in.ID)
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(a), nil
}

// ── retract ──

type retractInput struct {
	ID   string `json:"id"   jsonschema:"assertion id to retract (required)"`
	Note string `json:"note" jsonschema:"the reason or counter-evidence for withdrawing the assertion (required)"`
}

func (h *handlers) handleRetract(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in retractInput,
) (*mcp.CallToolResult, out, error) {
	a, err := h.client.Retract(in.ID, in.Note)
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(a), nil
}

// ── check ──

type checkInput struct {
	ID string `json:"id,omitempty" jsonschema:"assertion id to re-check; omit to re-check every non-retracted assertion"`
}

type checkOutput struct {
	Checked    int                `json:"checked"`
	Fresh      int                `json:"fresh"`
	Stale      int                `json:"stale"`
	Flipped    int                `json:"flipped"`
	Assertions []client.Assertion `json:"assertions"`
	URL        string             `json:"url"`
}

func (h *handlers) handleCheck(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in checkInput,
) (*mcp.CallToolResult, checkOutput, error) {
	rep, err := h.client.Check(in.ID)
	if err != nil {
		return nil, checkOutput{}, err
	}
	return nil, checkOutput{
		Checked:    rep.Checked,
		Fresh:      rep.Fresh,
		Stale:      rep.Stale,
		Flipped:    rep.Flipped,
		Assertions: rep.Assertions,
		URL:        h.webURL,
	}, nil
}

// ── doctor ──

type doctorInput struct{}

// handleDoctor runs the same checks as `kof doctor` and returns the report.
// A failing check is a result, not a tool error: the caller asked what is
// wrong, and an error would hide the answer behind a transport failure.
func (h *handlers) handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ doctorInput,
) (*mcp.CallToolResult, doctor.Report, error) {
	return nil, doctor.Collect(ctx, h.checks(ctx)), nil
}

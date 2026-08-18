package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/keeper-of-facts/internal/client"
)

// handlers carries the dependencies shared by all keep tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views assertions in a browser
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_assert",
		Description: "Record a one-sentence assertion about how a system behaves and ground it in evidence pins. " +
			"An assertion is a durable, checkable claim you derived this session — how some code acts, an approach that failed, a settled decision — so a later session can trust it without re-deriving it. " +
			"Each pin captures a line range in a repo working tree, hashed the moment you assert; `kof_check` re-hashes them later and flips the assertion stale if the pinned code changed. " +
			"At least one pin is REQUIRED — pins are what let keep tell whether the claim still holds, so an assertion with no pin is rejected. " +
			"Keep the returned id — it is the handle for kof_get / kof_retract / kof_check.",
	}, h.handleAssert)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_query",
		Description: "List assertions, newest first, to see what past sessions already established before deriving it again. " +
			"Filter by `subject` (prefix match on the namespaced key), `kind`, and/or `status` (fresh | stale | retracted); omit all to list everything. " +
			"Returns each assertion's id, statement, kind, confidence, and status — follow up with kof_get for the full record including pins.",
	}, h.handleQuery)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_recall",
		Description: "Ask the keeper what it knows relevant to a free-form question. " +
			"A one-shot model judge ranks the whole store against the question and returns the relevant assertions in rank order — " +
			"use this when you don't know the subject key: \"why does the store not lose writes\" finds the single-writer assertion even though no word matches. " +
			"Stale assertions are included and marked (treat them as needing re-verification); retracted ones never appear. " +
			"Takes a few seconds. If the judge is unavailable the error says so — fall back to kof_query.",
	}, h.handleRecall)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "kof_get",
		Description: "Get one assertion's full detail by id, including every evidence pin (repo, file, line range, content hash, resolved commit) and its provenance (which session derived it, when, and the token cost).",
	}, h.handleGet)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_retract",
		Description: "Withdraw an assertion by id — a terminal action that records the claim as no longer holding. " +
			"`note` is REQUIRED: give the reason or the counter-evidence, since the retracted record stays in the store as the explanation of why the claim was dropped. " +
			"Use this instead of leaving a wrong assertion to go stale when you have direct evidence it is false.",
	}, h.handleRetract)

	mcp.AddTool(s, &mcp.Tool{
		Name: "kof_check",
		Description: "Re-hash an assertion's evidence pins against the current working tree and report whether it still holds. " +
			"Pass `id` to check one assertion, or omit `id` to re-check every non-retracted assertion. " +
			"Returns counts of how many were checked, how many are fresh, how many are stale, and how many flipped between the two on this run. " +
			"A stale result means the pinned code changed and the claim needs another look — not that it is necessarily wrong.",
	}, h.handleCheck)
}

// out is the tool response shape for the single-assertion tools: the assertion
// plus the web URL to view it.
type out struct {
	Assertion client.Assertion `json:"assertion"`
	URL       string           `json:"url"       jsonschema_description:"web page where the user can view assertions"`
}

func (h *handlers) one(a client.Assertion) out { return out{Assertion: a, URL: h.webURL} }

// ── assert ──

type pinInput struct {
	RepoPath  string `json:"repo_path"  jsonschema_description:"absolute path to the repo working tree the evidence lives in, e.g. /Users/me/code/src/github.com/mad01/thismoon"`
	File      string `json:"file"       jsonschema_description:"repo-relative path to the file"`
	StartLine int    `json:"start_line" jsonschema_description:"first line of the evidence range (1-based, inclusive)"`
	EndLine   int    `json:"end_line"   jsonschema_description:"last line of the evidence range (1-based, inclusive; >= start_line)"`
}

type assertInput struct {
	Kind       string     `json:"kind"                  jsonschema_description:"what sort of claim this is; one of: code-behavior (how code acts), dead-end (an approach that failed), preference (a stated way of working), decision (a settled choice), machine-state (a fact about the local machine), open-thread (unfinished work worth resuming)"`
	Subject    string     `json:"subject"               jsonschema_description:"namespaced key the claim is about, e.g. repo:mad01/thismoon/services/events"`
	Statement  string     `json:"statement"             jsonschema_description:"the claim in one sentence"`
	Confidence string     `json:"confidence"            jsonschema_description:"how strongly you believe it: verified (checked against a primary source), derived (reasoned from evidence), or hint (a weak signal)"`
	SessionID  string     `json:"session_id"            jsonschema_description:"identifier of the session deriving this assertion"`
	CostTokens int        `json:"cost_tokens,omitempty" jsonschema_description:"optional token cost of deriving the assertion"`
	Links      []string   `json:"links,omitempty"       jsonschema_description:"optional related URLs (tickets, PRs, docs)"`
	Pins       []pinInput `json:"pins"                  jsonschema_description:"evidence pins grounding the claim in hashed line ranges; at least one is REQUIRED"`
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
	Question string `json:"question" jsonschema_description:"the free-form question to rank the store against, e.g. \"what do we know about JSONL write races\""`
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
	Subject string `json:"subject,omitempty" jsonschema_description:"optional prefix match on the subject key"`
	Kind    string `json:"kind,omitempty"    jsonschema_description:"optional kind filter: code-behavior | dead-end | preference | decision | machine-state | open-thread"`
	Status  string `json:"status,omitempty"  jsonschema_description:"optional status filter: fresh | stale | retracted"`
}

type queryOutput struct {
	Assertions []client.Assertion `json:"assertions"`
	URL        string             `json:"url"`
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
	return nil, queryOutput{Assertions: as, URL: h.webURL}, nil
}

// ── get ──

type idInput struct {
	ID string `json:"id" jsonschema_description:"assertion id returned by kof_assert"`
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
	ID   string `json:"id"   jsonschema_description:"assertion id to retract (required)"`
	Note string `json:"note" jsonschema_description:"the reason or counter-evidence for withdrawing the assertion (required)"`
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
	ID string `json:"id,omitempty" jsonschema_description:"assertion id to re-check; omit to re-check every non-retracted assertion"`
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

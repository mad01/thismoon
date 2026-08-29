package mcpserver

import (
	"context"
	"errors"
	"fmt"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/tools/worklog/internal/config"
	"github.com/mad01/thismoon/tools/worklog/internal/store"
)

type itemView struct {
	Key     string   `json:"key"`
	Status  string   `json:"status"`
	Ticket  string   `json:"ticket,omitempty"`
	Topic   string   `json:"topic,omitempty"`
	Updated string   `json:"updated"`
	Repos   []string `json:"repos,omitempty"`
	Warning string   `json:"warning,omitempty"`
}

// handlers holds what every tool call needs: the config file this server was
// started against. The tools are methods on it rather than package functions
// so the path travels with the server instead of through package state.
type handlers struct{ configPath string }

// store mirrors the CLI's store construction: remote resolved from the config,
// with a bootstrap clone when the store directory is missing. A failed clone
// degrades to the local-only store — a tool call must not break because the
// network is away. A config that exists but cannot be read is a different
// matter and is returned, since it decides where writes get pushed.
func (h *handlers) store() (*store.Store, error) {
	path, err := config.Path(h.configPath)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return nil, err
	}
	s, err := store.NewDefault()
	if err != nil {
		return nil, err
	}
	s.Remote = store.Remote{
		URL:  cfg.Remote.URL,
		Push: cfg.Remote.URL != "" && cfg.Remote.PushEnabled(),
	}
	_ = s.EnsureCloned()
	return s, nil
}

func view(it *store.Item) itemView {
	return itemView{
		Key: it.FM.Key, Status: it.FM.Status, Ticket: it.FM.Ticket, Topic: it.FM.Topic,
		Updated: it.FM.Updated.Format("2006-01-02 15:04"), Repos: it.FM.Repos,
	}
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "worklog_checkpoint",
		Description: "Save resumable work state for a long, cross-repo task so a later session can pick it up. " +
			"Key by ticket id (e.g. ABC-1234) when one exists, else a stable topic slug. Creates the item if missing. " +
			"Pass `cwd` (the user's working directory) so the touched repo is recorded; pass `repo` to override detection. " +
			"Use when the user says checkpoint, save my progress, worklog this, or before a session ends mid-task. " +
			"IMPORTANT: the `where` field is the ONLY context a fresh session gets when resuming. " +
			"Write it for a reader with zero prior knowledge. Include: goal, repo paths (host + local), " +
			"conclusions reached and WHY, working tool/query examples with exact parameters, " +
			"anti-patterns that waste time, links to tickets/docs/PRs, decisions made and their reasoning, " +
			"and concrete next steps. A thin checkpoint forces the next session to re-discover everything.",
	}, h.handleCheckpoint)

	mcp.AddTool(s, &mcp.Tool{
		Name: "worklog_list",
		Description: "List work items, newest first. Filter by status (active|paused|done) or repo. " +
			"Use to answer 'what was I working on' or to find an item to resume. " +
			"On zero results the response carries zero_result_hint (how many items the store holds) — read it to tell a filter miss from an empty or unclonable store.",
	}, h.handleList)

	mcp.AddTool(s, &mcp.Tool{
		Name: "worklog_search",
		Description: "Search work items by key and content (active items first). Use to find a past task by ticket, topic, or keyword. " +
			"On zero results the response carries zero_result_hint (how many items the store holds) — read it to tell a query miss from an empty or unclonable store.",
	}, h.handleSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name: "worklog_show",
		Description: "Read a work item's full CONTEXT.md (goal, where-I-am, log), or a per-repo note with `repo`. " +
			"Use when resuming to restore context before continuing. " +
			"Only explicitly checkpointed tasks have items — unless a prior call already confirmed the key exists, " +
			"run worklog_search first instead of assuming a ticket was checkpointed.",
	}, h.handleShow)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "worklog_status",
		Description: "Set a work item's status to active, paused, or done.",
	}, h.handleStatus)
}

type checkpointInput struct {
	Key    string `json:"key"              jsonschema:"ticket id (ABC-1234) or topic slug"`
	Where  string `json:"where,omitempty"  jsonschema:"current state snapshot; replaces the Where I am section. Write for cold-start: goal, repo paths, all conclusions with reasoning, working query/tool examples, anti-patterns, decisions, next steps. This is the ONLY context a resuming session sees."`
	Note   string `json:"note,omitempty"   jsonschema:"a log entry appended to the item's history. Summarize what changed this session and list remaining work items."`
	Ticket string `json:"ticket,omitempty" jsonschema:"ticket id, set on first creation"`
	Topic  string `json:"topic,omitempty"  jsonschema:"free-text topic, set on first creation"`
	Repo   string `json:"repo,omitempty"   jsonschema:"repo this checkpoint touched; overrides cwd detection"`
	Cwd    string `json:"cwd,omitempty"    jsonschema:"the user's working directory, used to detect the repo and record last_cwd"`
}

func (h *handlers) handleCheckpoint(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in checkpointInput,
) (*mcp.CallToolResult, itemView, error) {
	if in.Where == "" && in.Note == "" {
		return nil, itemView{}, fmt.Errorf("nothing to record: provide where and/or note")
	}
	repo := in.Repo
	if repo == "" {
		repo = store.DetectRepo(in.Cwd)
	}
	s, err := h.store()
	if err != nil {
		return nil, itemView{}, err
	}
	it, err := s.Checkpoint(store.Sanitize(in.Key), store.CheckpointInput{
		Ticket: in.Ticket, Topic: in.Topic, Where: in.Where, Note: in.Note, Repo: repo, Cwd: in.Cwd,
	})
	if errors.Is(err, store.ErrPush) {
		// The checkpoint is written and committed; surface the failed push
		// without failing the tool call.
		v := view(it)
		v.Warning = err.Error()
		return nil, v, nil
	}
	if err != nil {
		return nil, itemView{}, err
	}
	return nil, view(it), nil
}

type listInput struct {
	Status string `json:"status,omitempty" jsonschema:"filter by status: active, paused, or done"`
	Repo   string `json:"repo,omitempty"   jsonschema:"filter to items touching this repo"`
}

type listOutput struct {
	Items    []itemView    `json:"items"`
	ZeroHint *listZeroHint `json:"zero_result_hint,omitempty" jsonschema:"set only on zero results: how many items the store actually holds, so a filter or query miss is distinguishable from an empty or unclonable store"`
}

// listZeroHint explains an empty worklog_list or worklog_search so an agent
// can tell "the filters missed" from "nothing is checkpointed" — the store
// degrades to local-only silently when its clone fails, so an unexpected
// empty store is worth naming. Additive: it appears only on zero results.
type listZeroHint struct {
	ItemsStored int      `json:"items_stored"    jsonschema:"work items in the store regardless of filters"`
	Notes       []string `json:"notes,omitempty" jsonschema:"one note naming why the result is empty"`
}

// buildZeroHint counts the unfiltered store. Best-effort: it returns nil when
// the extra lookup fails, leaving the plain empty result. filteredNote phrases
// the miss for the caller's filter or query.
func buildZeroHint(s *store.Store, filteredNote string) *listZeroHint {
	all, err := s.List("", "")
	if err != nil {
		return nil
	}
	hint := &listZeroHint{ItemsStored: len(all)}
	if len(all) == 0 {
		hint.Notes = []string{
			"the store holds no items: nothing has been checkpointed on this machine, or the store clone failed silently (run 'worklog docs' to debug)",
		}
		return hint
	}
	hint.Notes = []string{fmt.Sprintf("%d items stored; %s", len(all), filteredNote)}
	return hint
}

func (h *handlers) handleList(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in listInput,
) (*mcp.CallToolResult, listOutput, error) {
	s, err := h.store()
	if err != nil {
		return nil, listOutput{}, err
	}
	items, err := s.List(in.Status, in.Repo)
	if err != nil {
		return nil, listOutput{}, err
	}
	out := listOutput{Items: toViews(items)}
	if len(items) == 0 {
		out.ZeroHint = buildZeroHint(s, "the status/repo filter matched none")
	}
	return nil, out, nil
}

type searchInput struct {
	Query string `json:"query" jsonschema:"substring to match against keys and content"`
}

func (h *handlers) handleSearch(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in searchInput,
) (*mcp.CallToolResult, listOutput, error) {
	s, err := h.store()
	if err != nil {
		return nil, listOutput{}, err
	}
	hits, err := s.Search(in.Query)
	if err != nil {
		return nil, listOutput{}, err
	}
	out := listOutput{Items: toViews(hits)}
	if len(hits) == 0 {
		out.ZeroHint = buildZeroHint(s, fmt.Sprintf("no key or content contains %q", in.Query))
	}
	return nil, out, nil
}

type showInput struct {
	Key  string `json:"key"            jsonschema:"ticket id or topic slug"`
	Repo string `json:"repo,omitempty" jsonschema:"print this repo's note file instead of CONTEXT.md"`
}

type showOutput struct {
	Markdown string `json:"markdown"`
}

func (h *handlers) handleShow(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in showInput,
) (*mcp.CallToolResult, showOutput, error) {
	s, err := h.store()
	if err != nil {
		return nil, showOutput{}, err
	}
	key := store.Sanitize(in.Key)
	if in.Repo != "" {
		note, err := s.RepoNote(key, in.Repo)
		if err != nil {
			return nil, showOutput{}, fmt.Errorf("no notes for repo %q on %q", in.Repo, key)
		}
		return nil, showOutput{Markdown: note}, nil
	}
	it, err := s.Load(key)
	if err != nil {
		return nil, showOutput{}, fmt.Errorf(
			"no item %q — only explicitly checkpointed tasks have worklog items; run worklog_search(query: %q) to check what exists before assuming saved state",
			key,
			key,
		)
	}
	b, err := it.Render()
	if err != nil {
		return nil, showOutput{}, err
	}
	return nil, showOutput{Markdown: string(b)}, nil
}

type statusInput struct {
	Key    string `json:"key"    jsonschema:"ticket id or topic slug"`
	Status string `json:"status" jsonschema:"active, paused, or done"`
}

func (h *handlers) handleStatus(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in statusInput,
) (*mcp.CallToolResult, itemView, error) {
	if in.Status != "active" && in.Status != "paused" && in.Status != "done" {
		return nil, itemView{}, fmt.Errorf("status must be active, paused, or done")
	}
	s, err := h.store()
	if err != nil {
		return nil, itemView{}, err
	}
	it, err := s.SetStatus(store.Sanitize(in.Key), in.Status)
	if errors.Is(err, store.ErrPush) {
		v := view(it)
		v.Warning = err.Error()
		return nil, v, nil
	}
	if err != nil {
		return nil, itemView{}, err
	}
	return nil, view(it), nil
}

func toViews(items []*store.Item) []itemView {
	views := make([]itemView, 0, len(items))
	for _, it := range items {
		views = append(views, view(it))
	}
	return views
}

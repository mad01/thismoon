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

// newStore mirrors the CLI's store construction: remote resolved from the
// config for this machine's profile, with a bootstrap clone when the store
// directory is missing. A failed clone degrades to the local-only store — a
// tool call must not break because the network is away.
func newStore() *store.Store {
	s := store.New("")
	rc := config.Load().Remote
	url := rc.ResolveUpstream(config.MachineProfiles())
	s.Remote = store.Remote{URL: url, Push: url != "" && rc.PushEnabled()}
	_ = s.EnsureCloned()
	return s
}

func view(it *store.Item) itemView {
	return itemView{
		Key: it.FM.Key, Status: it.FM.Status, Ticket: it.FM.Ticket, Topic: it.FM.Topic,
		Updated: it.FM.Updated.Format("2006-01-02 15:04"), Repos: it.FM.Repos,
	}
}

func registerTools(s *mcp.Server) {
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
	}, handleCheckpoint)

	mcp.AddTool(s, &mcp.Tool{
		Name: "worklog_list",
		Description: "List work items, newest first. Filter by status (active|paused|done) or repo. " +
			"Use to answer 'what was I working on' or to find an item to resume.",
	}, handleList)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "worklog_search",
		Description: "Search work items by key and content (active items first). Use to find a past task by ticket, topic, or keyword.",
	}, handleSearch)

	mcp.AddTool(s, &mcp.Tool{
		Name: "worklog_show",
		Description: "Read a work item's full CONTEXT.md (goal, where-I-am, log), or a per-repo note with `repo`. " +
			"Use when resuming to restore context before continuing.",
	}, handleShow)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "worklog_status",
		Description: "Set a work item's status to active, paused, or done.",
	}, handleStatus)
}

type checkpointInput struct {
	Key    string `json:"key"              jsonschema_description:"ticket id (ABC-1234) or topic slug"`
	Where  string `json:"where,omitempty"  jsonschema_description:"current state snapshot; replaces the Where I am section. Write for cold-start: goal, repo paths, all conclusions with reasoning, working query/tool examples, anti-patterns, decisions, next steps. This is the ONLY context a resuming session sees."`
	Note   string `json:"note,omitempty"   jsonschema_description:"a log entry appended to the item's history. Summarize what changed this session and list remaining work items."`
	Ticket string `json:"ticket,omitempty" jsonschema_description:"ticket id, set on first creation"`
	Topic  string `json:"topic,omitempty"  jsonschema_description:"free-text topic, set on first creation"`
	Repo   string `json:"repo,omitempty"   jsonschema_description:"repo this checkpoint touched; overrides cwd detection"`
	Cwd    string `json:"cwd,omitempty"    jsonschema_description:"the user's working directory, used to detect the repo and record last_cwd"`
}

func handleCheckpoint(
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
	it, err := newStore().Checkpoint(store.Sanitize(in.Key), store.CheckpointInput{
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
	Status string `json:"status,omitempty" jsonschema_description:"filter by status: active, paused, or done"`
	Repo   string `json:"repo,omitempty"   jsonschema_description:"filter to items touching this repo"`
}

type listOutput struct {
	Items []itemView `json:"items"`
}

func handleList(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in listInput,
) (*mcp.CallToolResult, listOutput, error) {
	items, err := newStore().List(in.Status, in.Repo)
	if err != nil {
		return nil, listOutput{}, err
	}
	return nil, listOutput{Items: toViews(items)}, nil
}

type searchInput struct {
	Query string `json:"query" jsonschema_description:"substring to match against keys and content"`
}

func handleSearch(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in searchInput,
) (*mcp.CallToolResult, listOutput, error) {
	hits, err := newStore().Search(in.Query)
	if err != nil {
		return nil, listOutput{}, err
	}
	return nil, listOutput{Items: toViews(hits)}, nil
}

type showInput struct {
	Key  string `json:"key"            jsonschema_description:"ticket id or topic slug"`
	Repo string `json:"repo,omitempty" jsonschema_description:"print this repo's note file instead of CONTEXT.md"`
}

type showOutput struct {
	Markdown string `json:"markdown"`
}

func handleShow(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in showInput,
) (*mcp.CallToolResult, showOutput, error) {
	s := newStore()
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
		return nil, showOutput{}, fmt.Errorf("no item %q", key)
	}
	b, err := it.Render()
	if err != nil {
		return nil, showOutput{}, err
	}
	return nil, showOutput{Markdown: string(b)}, nil
}

type statusInput struct {
	Key    string `json:"key"    jsonschema_description:"ticket id or topic slug"`
	Status string `json:"status" jsonschema_description:"active, paused, or done"`
}

func handleStatus(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in statusInput,
) (*mcp.CallToolResult, itemView, error) {
	if in.Status != "active" && in.Status != "paused" && in.Status != "done" {
		return nil, itemView{}, fmt.Errorf("status must be active, paused, or done")
	}
	it, err := newStore().SetStatus(store.Sanitize(in.Key), in.Status)
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

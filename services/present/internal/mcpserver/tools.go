package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/notify"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/baseurl"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// openURL launches the system browser for url. Factored as a package var so
// tests can substitute it without spawning a real process.
var openURL = func(url string) error {
	return exec.Command("/usr/bin/open", url).Start()
}

// hint points an error's reader at present's self-diagnosis surface. Tool
// errors pick it up in withHint; server.go calls it directly for the one
// error that never flows through a handler.
func hint(err error) error {
	return agentdoc.Hint(err, present.Facts())
}

// withHint wraps a tool handler so any error it returns carries the hint,
// applied exactly once here at registration instead of at every return site
// inside the handlers. hint is nil-safe, so a clean return passes through.
func withHint[In, Out any](
	h func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error),
) func(context.Context, *mcp.CallToolRequest, In) (*mcp.CallToolResult, Out, error) {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, Out, error) {
		res, out, err := h(ctx, req, in)
		return res, out, hint(err)
	}
}

// handlers carries the dependencies shared by all present tools. baseURL is
// the fixed URL prefix locally and the display override on a shared
// instance, where an empty value means derive from the request.
type handlers struct {
	store   store.Store
	mode    Mode
	baseURL string
	now     func() time.Time
	open    func(url string) error
	checks  func(ctx context.Context) []doctor.Check
	sharer  *sharedclient.Client
}

// createAnnotations is shared by both create tools.
var createAnnotations = &mcp.ToolAnnotations{
	DestructiveHint: new(false),
	IdempotentHint:  false,
	OpenWorldHint:   new(false),
}

func registerTools(s *mcp.Server, h *handlers) {
	if h.mode == ModeShared {
		mcp.AddTool(s, &mcp.Tool{
			Name: "present_create",
			Description: "Create a page on this shared instance and return its id and URL. " +
				"Provide a Doc JSON object as `content`; the server renders it to HTML with the correct CSS classes and structure. " +
				"Pass an optional Graph JSON object as `graph` (structured nodes/edges) and optional `references` (source links displayed at the bottom). " +
				"Set `ephemeral` to have the page expire 30 days after its last update; otherwise it stays until deleted. " +
				"The author key this connection sends as its bearer token becomes the page's author; only it can update the page. " +
				"Keep the returned URL: nothing on this instance lists pages.",
			Annotations: createAnnotations,
		}, withHint(h.handleCreateShared))
	} else {
		mcp.AddTool(s, &mcp.Tool{
			Name: "present_create",
			Description: "Create a new presentation page and return its id and URL. " +
				"Provide a Doc JSON object as `content`; the server renders it to HTML with the correct CSS classes and structure. " +
				"Pass an optional Graph JSON object as `graph` (structured nodes/edges) and optional `references` (source links displayed at the bottom). " +
				"Keep the returned id; it is the handle for present_update/present_read/present_open.",
			Annotations: createAnnotations,
		}, withHint(h.handleCreate))
	}

	mcp.AddTool(s, &mcp.Tool{
		Name:        "present_read",
		Description: "Read a presentation's current title, rendered HTML content, graph JS, version, and URL by id. For editing, prefer present_source: it returns the structured source in the format present_update accepts.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, withHint(h.handleRead))

	mcp.AddTool(s, &mcp.Tool{
		Name: "present_source",
		Description: "Get a presentation's editable source by id: the Doc JSON it was created from (content_format=doc) and the structured graph JSON (graph_format=json), plus references. " +
			"Both come back in exactly the format present_update accepts, so you can modify them and pass them straight back. Use this to mutate a page from a new or restored session. " +
			"Legacy pages return content_format=html (raw HTML) or graph_format=js (raw JS); those can only be edited in that form.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, withHint(h.handleSource))

	mcp.AddTool(s, &mcp.Tool{
		Name: "present_update",
		Description: "Update an existing presentation. The `id` parameter is REQUIRED: it is the page id returned by present_create. " +
			"Provide a Doc JSON object as `content` and/or a Graph JSON object as `graph`. " +
			"Only the fields you provide are changed (omit a field to leave it as-is); pass an empty string to clear the graph. " +
			"Bumps the page version so any open browser tab auto-reloads, so you don't need to call present_open again after an update.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(true),
			IdempotentHint:  false,
			OpenWorldHint:   new(false),
		},
	}, withHint(h.handleUpdate))

	// A shared instance lists nothing (pages are reachable by id alone) and
	// has no browser to open.
	if h.mode == ModeLocal {
		mcp.AddTool(s, &mcp.Tool{
			Name:        "present_list",
			Description: "List all presentations (id, title, URL, version, last updated), newest first.",
			Annotations: &mcp.ToolAnnotations{
				ReadOnlyHint:  true,
				OpenWorldHint: new(false),
			},
		}, withHint(h.handleList))

		mcp.AddTool(s, &mcp.Tool{
			Name: "present_open",
			Description: "Open a presentation in the default browser (macOS `open`). " +
				"Call this AT MOST ONCE per presentation: after the tab is open, present_update triggers an automatic reload, " +
				"so don't call present_open again for subsequent edits. " +
				"If this fails (sandbox or PATH issue), return the URL from present_create to the user instead.",
			Annotations: &mcp.ToolAnnotations{
				DestructiveHint: new(false),
				IdempotentHint:  false,
				OpenWorldHint:   new(false),
			},
		}, withHint(h.handleOpen))

		if h.sharer != nil {
			mcp.AddTool(s, &mcp.Tool{
				Name: "present_share",
				Description: "Push a local page to the configured shared instance and return the link others can open. " +
					"Sharing again replaces the copy under the same link. Set `ephemeral` to have the copy expire 30 days after " +
					"the last share; otherwise it stays until `present unshare`. Only this machine's author key can change the copy.",
				Annotations: &mcp.ToolAnnotations{
					DestructiveHint: new(false),
					IdempotentHint:  true,
					OpenWorldHint:   new(true),
				},
			}, withHint(h.handleShare))
		}
	}

	// Not wrapped in withHint: the hint says to run `present doctor`, which is
	// exactly what this tool already did.
	mcp.AddTool(s, &mcp.Tool{
		Name: "present_doctor",
		Description: "Diagnose present itself: is the page store readable, is serve reachable, is the running " +
			"build the installed one. Returns one result per check with `ok` false if any failed. " +
			"Read-only: it probes, it changes nothing. " +
			"The store check matters most: these tools write the page store directly, so pages can be created " +
			"and updated with serve down; only the URLs stop resolving. " +
			"Call this when a present tool errors or a page URL doesn't load.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleDoctor)
}

// header returns the HTTP headers behind a tool call, which the streamable
// HTTP transport attaches to every request. Nil over stdio and in tests
// that pass no request.
func header(req *mcp.CallToolRequest) http.Header {
	if req == nil || req.Extra == nil {
		return nil
	}
	return req.Extra.Header
}

// url is the public URL of a page for the caller: the fixed prefix locally,
// the origin the request arrived on (or the override) on a shared instance.
func (h *handlers) url(req *mcp.CallToolRequest, id string) string {
	base := h.baseURL
	if h.mode == ModeShared {
		if derived := baseurl.FromHeader(header(req), h.baseURL); derived != "" {
			base = derived
		}
	}
	return base + "/p/" + id
}

// expiry returns when an ephemeral page touched now expires, or nil.
func (h *handlers) expiry(ephemeral bool) *time.Time {
	if !ephemeral {
		return nil
	}
	t := h.now().UTC().Add(present.SharedTTL)
	return &t
}

// resolveContent detects whether s is a Doc JSON object or a legacy HTML
// string and returns rendered HTML either way. When the input is a Doc, it also
// returns the canonical Doc JSON (re-marshaled from the parsed struct) so the
// caller can persist it for future re-renders; docJSON is nil for HTML input.
func resolveContent(s string, title string) (htmlOut string, docJSON []byte, err error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return "", nil, nil
	}
	if s[0] == '{' {
		c, err := render.Compile([]byte(s), title)
		if err != nil {
			return "", nil, err
		}
		return c.HTML, c.JSON, nil
	}
	return s, nil, nil
}

// resolveGraph detects whether s is a GraphInput JSON object or a legacy JS
// string and returns a JS script string either way. When the input is a
// structured graph, it also returns the canonical GraphInput JSON (re-marshaled
// from the parsed struct) so the caller can persist it for future re-renders;
// srcJSON is nil for legacy JS input.
func resolveGraph(s string) (js string, srcJSON []byte, err error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return "", nil, nil
	}
	if s[0] == '{' {
		return parseGraphAndRender([]byte(s))
	}
	return s, nil, nil
}

func parseGraphAndRender(data []byte) (js string, srcJSON []byte, err error) {
	var g render.GraphInput
	if err := json.Unmarshal(data, &g); err != nil {
		return "", nil, fmt.Errorf("parse graph: %w", err)
	}
	out, err := render.RenderGraph(g)
	if err != nil {
		return "", nil, err
	}
	canonical, err := json.Marshal(g)
	if err != nil {
		return "", nil, fmt.Errorf("marshal graph: %w", err)
	}
	return out, canonical, nil
}

// ── create ──

type refInput struct {
	Title string `json:"title" jsonschema:"display label for the link"`
	URL   string `json:"url"   jsonschema:"full URL (https://...) pointing at repos, docs, PRs, or any external material cited in the brief"`
}

type createInput struct {
	Title      string     `json:"title"                jsonschema:"presentation title (shown in the browser tab and page header)"`
	Content    string     `json:"content"              jsonschema:"page content as a Doc JSON string {summary?, meta?, chips?, sections:[{h, blocks:[{t,...}]}]} or a legacy HTML string. Doc block types: p, h3, callout (sev: info|warn), table (cols+rows), kv ([{k,v}]), list (items, ordered?), panel (title, sub?, accent?), progress (pct, label?), graph (placement marker), chart (kind: bar|line|area|sparkline|stacked-bar|horizontal-bar|doughnut|scatter|sankey, title?, unit?, xunit?, series:[{name?, color?, points:[{x,y}]}] for every kind but sankey, flows:[{from,to,value}] for sankey; inline metric chart, many per page), code (text + lang?, verbatim code block with copy button, no inline markdown), html (raw passthrough). Text fields support inline markdown: **bold**, *italic*, backtick-code, [text](url), @chip(style:text). Prefer the Doc format for compact structured input."`
	Graph      string     `json:"graph,omitempty"      jsonschema:"optional Cytoscape graph as a structured JSON string {nodes:[{id,label,type?,color?}], edges:[{from,to,type?,label?,weight?,flow?}], layout?, direction?} or a legacy JS string. Node types: center, module, leaf, registry. Edge types: consumes (solid), publishes (dashed). Edge weight (a number, e.g. requests per second) drives line width and tints the busiest edges; flow: true animates dashes from source to target. Layouts: dagre (default, layered DAG), cose (no hierarchy). Direction (dagre): TB or LR; omit for auto (LR when the graph has few nodes)."`
	References []refInput `json:"references,omitempty" jsonschema:"source links shown in a References section at the bottom of the page: repos, docs, PRs consulted while writing the brief"`
}

// sharedCreateInput is createInput plus the one choice a shared page adds.
type sharedCreateInput struct {
	createInput
	Ephemeral bool `json:"ephemeral,omitempty" jsonschema:"expire the page 30 days after its last update instead of keeping it until deleted"`
}

type pageOutput struct {
	ID        string `json:"id"`
	URL       string `json:"url"`
	Version   int    `json:"version"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func pageOutputFor(p store.Page, url string) pageOutput {
	out := pageOutput{ID: p.ID, URL: url, Version: p.Version, Ephemeral: p.Ephemeral}
	if p.ExpiresAt != nil {
		out.ExpiresAt = p.ExpiresAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (h *handlers) handleCreate(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in createInput,
) (*mcp.CallToolResult, pageOutput, error) {
	return h.create(ctx, req, in, false)
}

func (h *handlers) handleCreateShared(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in sharedCreateInput,
) (*mcp.CallToolResult, pageOutput, error) {
	return h.create(ctx, req, in.createInput, in.Ephemeral)
}

// create renders the input and stores the page. On a shared instance the
// caller's key must be present: it becomes the page's author, and the id
// is a fresh capability id.
func (h *handlers) create(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in createInput,
	ephemeral bool,
) (*mcp.CallToolResult, pageOutput, error) {
	content, docJSON, err := resolveContent(in.Content, in.Title)
	if err != nil {
		return nil, pageOutput{}, fmt.Errorf("content: %w", err)
	}
	graph, graphJSON, err := resolveGraph(in.Graph)
	if err != nil {
		return nil, pageOutput{}, fmt.Errorf("graph: %w", err)
	}
	// The structured sources ride along (nil for legacy HTML/JS input) so a
	// future renderer/template change can re-render the page from source and
	// present_source can hand the source back for edits.
	d := store.Draft{
		Title:       in.Title,
		Content:     content,
		Graph:       graph,
		References:  toStoreRefs(in.References),
		Doc:         docJSON,
		GraphSource: graphJSON,
	}
	if h.mode == ModeShared {
		hash, ok := author.FromHeader(header(req))
		if !ok {
			return nil, pageOutput{}, author.ErrMissing
		}
		d.ID = store.NewSharedID()
		d.Author = hash
		d.Ephemeral = ephemeral
		d.ExpiresAt = h.expiry(ephemeral)
	}
	p, err := h.store.Create(ctx, d)
	if err != nil {
		return nil, pageOutput{}, err
	}
	notify.EmitEvent("present", "info", "page created: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title})
	return nil, pageOutputFor(p, h.url(req, p.ID)), nil
}

func toStoreRefs(in []refInput) []store.Reference {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Reference, len(in))
	for i, r := range in {
		out[i] = store.Reference{Title: r.Title, URL: r.URL}
	}
	return out
}

func fromStoreRefs(in []store.Reference) []refInput {
	if len(in) == 0 {
		return nil
	}
	out := make([]refInput, len(in))
	for i, r := range in {
		out[i] = refInput{Title: r.Title, URL: r.URL}
	}
	return out
}

// ── read ──

type readInput struct {
	ID string `json:"id" jsonschema:"page id returned by present_create"`
}

type readOutput struct {
	ID         string     `json:"id"`
	Title      string     `json:"title"`
	Content    string     `json:"content"`
	Graph      string     `json:"graph"`
	References []refInput `json:"references,omitempty"`
	Version    int        `json:"version"`
	URL        string     `json:"url"`
	UpdatedAt  string     `json:"updated_at"`
}

func (h *handlers) handleRead(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in readInput,
) (*mcp.CallToolResult, readOutput, error) {
	p, err := h.store.Get(ctx, in.ID)
	if err != nil {
		return nil, readOutput{}, err
	}
	return nil, readOutput{
		ID: p.ID, Title: p.Title, Content: p.Content, Graph: p.Graph,
		References: fromStoreRefs(p.References),
		Version:    p.Version, URL: h.url(req, p.ID), UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}, nil
}

// ── source ──

type sourceInput struct {
	ID string `json:"id" jsonschema:"page id returned by present_create"`
}

type sourceOutput struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	ContentFormat string     `json:"content_format"         jsonschema:"doc = structured Doc JSON (editable, pass back to present_update); html = legacy raw HTML (no Doc source persisted)"`
	Content       string     `json:"content"`
	GraphFormat   string     `json:"graph_format,omitempty" jsonschema:"json = structured GraphInput JSON (editable); js = legacy raw JS; empty = page has no graph"`
	Graph         string     `json:"graph,omitempty"`
	References    []refInput `json:"references,omitempty"`
	Version       int        `json:"version"`
	URL           string     `json:"url"`
}

func (h *handlers) handleSource(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in sourceInput,
) (*mcp.CallToolResult, sourceOutput, error) {
	p, err := h.store.Get(ctx, in.ID)
	if err != nil {
		return nil, sourceOutput{}, err
	}
	out := sourceOutput{
		ID: p.ID, Title: p.Title,
		References: fromStoreRefs(p.References),
		Version:    p.Version, URL: h.url(req, p.ID),
	}
	doc, err := h.store.LoadDoc(ctx, in.ID)
	switch {
	case err == nil:
		out.ContentFormat, out.Content = "doc", string(doc)
	case errors.Is(err, store.ErrNotFound):
		out.ContentFormat, out.Content = "html", p.Content
	default:
		return nil, sourceOutput{}, err
	}
	if p.HasGraph {
		src, err := h.store.LoadGraphSource(ctx, in.ID)
		switch {
		case err == nil:
			out.GraphFormat, out.Graph = "json", string(src)
		case errors.Is(err, store.ErrNotFound):
			out.GraphFormat, out.Graph = "js", p.Graph
		default:
			return nil, sourceOutput{}, err
		}
	}
	return nil, out, nil
}

// ── update ──

type updateInput struct {
	ID         string      `json:"id"                   jsonschema:"page id to update"`
	Title      *string     `json:"title,omitempty"      jsonschema:"new title; omit to leave unchanged"`
	Content    *string     `json:"content,omitempty"    jsonschema:"new page content as a Doc JSON string or legacy HTML string; omit to leave unchanged"`
	Graph      *string     `json:"graph,omitempty"      jsonschema:"new graph as structured JSON string or legacy JS string; omit to leave unchanged, empty string to remove"`
	References *[]refInput `json:"references,omitempty" jsonschema:"replace the references list; omit to leave unchanged, empty array to clear"`
}

// sourcePlan is what an update does to the page's persisted sources once
// the page itself is written. A source the update replaced is saved; one
// the update made stale is deleted, so a later re-render never reads a
// source the page no longer came from.
type sourcePlan struct {
	doc        []byte
	clearDoc   bool
	graph      []byte
	clearGraph bool
}

func (h *handlers) handleUpdate(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in updateInput,
) (*mcp.CallToolResult, pageOutput, error) {
	// On a shared instance only the author may write. The check comes
	// first, before any rendering: an update from the wrong key must cost
	// this instance nothing but a store read.
	var cur store.Page
	if h.mode == ModeShared {
		var err error
		if cur, err = h.store.Get(ctx, in.ID); err != nil {
			return nil, pageOutput{}, err
		}
		if err := author.Check(cur.Author, header(req)); err != nil {
			return nil, pageOutput{}, err
		}
	}
	patch, plan, err := resolveUpdate(in)
	if err != nil {
		return nil, pageOutput{}, err
	}
	// An ephemeral page's 30 days start over on every update.
	if h.mode == ModeShared && cur.Ephemeral {
		on := true
		patch.Ephemeral = &on
		patch.ExpiresAt = h.expiry(true)
	}
	p, err := h.store.Update(ctx, in.ID, patch)
	if err != nil {
		return nil, pageOutput{}, err
	}
	if err := h.applySourcePlan(ctx, p.ID, plan); err != nil {
		return nil, pageOutput{}, err
	}
	notify.EmitEvent("present", "info", "page updated: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title})
	return nil, pageOutputFor(p, h.url(req, p.ID)), nil
}

// resolveUpdate renders the inputs an update carries into the patch for the
// page and the plan for its sources. It touches no store, so a malformed
// Doc or Graph fails before anything is written.
func resolveUpdate(in updateInput) (store.Patch, sourcePlan, error) {
	title := ""
	if in.Title != nil {
		title = *in.Title
	}
	patch := store.Patch{Title: in.Title}
	var plan sourcePlan
	if in.Content != nil {
		content, docJSON, err := resolveContent(*in.Content, title)
		if err != nil {
			return store.Patch{}, sourcePlan{}, fmt.Errorf("content: %w", err)
		}
		patch.Content = &content
		switch {
		case docJSON != nil:
			plan.doc = docJSON
		case strings.TrimSpace(*in.Content) != "":
			// Content replaced with legacy HTML: any prior doc.json is now stale.
			plan.clearDoc = true
		}
	}
	if in.Graph != nil {
		graph, graphJSON, err := resolveGraph(*in.Graph)
		if err != nil {
			return store.Patch{}, sourcePlan{}, fmt.Errorf("graph: %w", err)
		}
		patch.Graph = &graph
		// Graph cleared or replaced with legacy JS: any prior graph.json is
		// now stale and would mislead a later re-render.
		plan.graph, plan.clearGraph = graphJSON, graphJSON == nil
	}
	if in.References != nil {
		refs := toStoreRefs(*in.References)
		patch.References = &refs
	}
	return patch, plan, nil
}

// applySourcePlan persists the source changes an update implies. It runs
// after the page is written, so a page that failed to update leaves its
// sources alone.
func (h *handlers) applySourcePlan(ctx context.Context, id string, plan sourcePlan) error {
	switch {
	case plan.doc != nil:
		if err := h.store.SaveDoc(ctx, id, plan.doc); err != nil {
			return fmt.Errorf("save doc: %w", err)
		}
	case plan.clearDoc:
		if err := h.store.DeleteDoc(ctx, id); err != nil {
			return fmt.Errorf("clear doc: %w", err)
		}
	}
	switch {
	case plan.graph != nil:
		if err := h.store.SaveGraphSource(ctx, id, plan.graph); err != nil {
			return fmt.Errorf("save graph source: %w", err)
		}
	case plan.clearGraph:
		if err := h.store.DeleteGraphSource(ctx, id); err != nil {
			return fmt.Errorf("clear graph source: %w", err)
		}
	}
	return nil
}

// ── list ──

type listItem struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
	Version   int    `json:"version"`
	UpdatedAt string `json:"updated_at"`
	HasDoc    bool   `json:"has_doc"    jsonschema:"true when the page has a persisted Doc source; present_source returns it ready for editing"`
}

type listOutput struct {
	Pages []listItem `json:"pages"`
}

func (h *handlers) handleList(
	ctx context.Context,
	req *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, listOutput, error) {
	pages, err := h.store.ListMeta(ctx)
	if err != nil {
		return nil, listOutput{}, err
	}
	out := listOutput{Pages: make([]listItem, 0, len(pages))}
	for _, p := range pages {
		out.Pages = append(out.Pages, listItem{
			ID: p.ID, Title: p.Title, URL: h.url(req, p.ID),
			Version: p.Version, UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			HasDoc: p.HasDoc,
		})
	}
	return nil, out, nil
}

// ── open ──

type openInput struct {
	ID string `json:"id" jsonschema:"page id to open in the browser"`
}

type openOutput struct {
	URL    string `json:"url"`
	Opened bool   `json:"opened"`
}

func (h *handlers) handleOpen(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in openInput,
) (*mcp.CallToolResult, openOutput, error) {
	// Confirm the page exists before launching a browser at a dead URL.
	if _, err := h.store.Get(ctx, in.ID); err != nil {
		return nil, openOutput{}, err
	}
	url := h.url(req, in.ID)
	if err := h.open(url); err != nil {
		return nil, openOutput{URL: url, Opened: false}, fmt.Errorf("open %s: %w", url, err)
	}
	return nil, openOutput{URL: url, Opened: true}, nil
}

// ── doctor ──

type doctorInput struct{}

// handleDoctor runs the same checks as `present doctor` and returns the
// report. A failing check is a result, not a tool error: the caller asked
// what is wrong, and an error would hide the answer behind a transport
// failure.
func (h *handlers) handleDoctor(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	_ doctorInput,
) (*mcp.CallToolResult, doctor.Report, error) {
	return nil, doctor.Collect(ctx, h.checks(ctx)), nil
}

// ── share ──

type shareInput struct {
	ID        string `json:"id"                  jsonschema:"local page id to share"`
	Ephemeral bool   `json:"ephemeral,omitempty" jsonschema:"expire the shared copy 30 days after the last share instead of keeping it until unshared"`
}

type shareOutput struct {
	URL       string `json:"url"`
	Ephemeral bool   `json:"ephemeral"`
	ExpiresAt string `json:"expires_at,omitempty"`
	SharedAt  string `json:"shared_at"`
}

func (h *handlers) handleShare(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in shareInput,
) (*mcp.CallToolResult, shareOutput, error) {
	info, err := sharedclient.Share(ctx, h.store, h.sharer, in.ID, in.Ephemeral, h.now())
	if err != nil {
		return nil, shareOutput{}, err
	}
	out := shareOutput{
		URL:       info.URL,
		Ephemeral: info.Ephemeral,
		SharedAt:  info.SharedAt.UTC().Format(time.RFC3339),
	}
	if info.ExpiresAt != nil {
		out.ExpiresAt = info.ExpiresAt.UTC().Format(time.RFC3339)
	}
	notify.EmitEvent("present", "info", "page shared: "+in.ID, "",
		map[string]string{"id": in.ID, "url": info.URL})
	return nil, out, nil
}

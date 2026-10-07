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
	"github.com/mad01/thismoon/services/present/internal/images"
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
// instance, where an empty value means derive from the request. images is
// where an image block's local file lands; nil on a shared instance, which
// takes image URLs only.
type handlers struct {
	store   store.Store
	mode    Mode
	baseURL string
	now     func() time.Time
	open    func(url string) error
	checks  func(ctx context.Context) []doctor.Check
	sharer  *sharedclient.Client
	images  *images.Store
}

// createAnnotations is shared by both create tools.
var createAnnotations = &mcp.ToolAnnotations{
	DestructiveHint: new(false),
	IdempotentHint:  false,
	OpenWorldHint:   new(false),
}

// deckDescription is the part of the create and update descriptions that
// teaches a client what a deck is; the two tools share it so they cannot
// drift apart.
const deckDescription = "A page can also carry a slide deck beside its content, or instead of it: pass a second Doc JSON object as `deck`. " +
	"The deck is authored on its own, never converted from the content: each section is one slide, the title with the deck's summary, meta, and chips is the title slide, and references make the last slide. " +
	"Write it like a good deck, not a shorter brief: one idea per slide with the heading stating the claim, a list of 3 to 5 short items or one paragraph of at most two sentences per slide, " +
	"exactly one bold phrase or one @chip(stat:...) per slide as the highlight, a chart or the graph alone on its own slide, 5 to 12 slides in all, " +
	"at most one warn callout in the deck, and a closing `Next` slide with at most three actions. Detail belongs in the content; the deck view links to it. " +
	"Which block when: one `stat` or one `quote` alone on a slide for the one figure or the one line to remember; two or three `stat` blocks in a `columns` block for a row of figures; a `chart`, the `graph`, or an `image` beside its caption paragraph in `columns`; a `details` block belongs in the content, not on a slide. " +
	"Deck-level chrome, all optional, read by the deck view only: `logo` (absent = the embedded repo logo, \"none\" hides it, an http(s) URL replaces it), " +
	"`logo_position` (bottom-right default, bottom-left, top-left, top-right), `progress` (dots default, bar, none; dots give way to a bar above 24 slides), " +
	"`presenter` (a byline under the meta line on the title slide and in the footer), and `footer` (footer text; absent = the deck title, \"none\" suppresses it). " +
	"The footer line shows only when `presenter` or `footer` is set. " +
	"`transition` (deck-level: fade default, slide, none) is how the view moves between slides. " +
	"Per slide, a section may set `layout` (default, center, statement for the heading as the slide, section for a divider), `tone` (a palette role that tints the slide), " +
	"`notes` (speaker notes, inline markdown, shown in the deck's drawer on N, never on the slide), and `reveal: true` (list items and top-level blocks appear one per Next). " +
	"The deck is served at the returned deck_url, and present_source returns its Doc for editing. "

func registerTools(s *mcp.Server, h *handlers) {
	if h.mode == ModeShared {
		mcp.AddTool(s, &mcp.Tool{
			Name: "present_create",
			Description: "Create a page on this shared instance and return its id and URL. " +
				"Provide a Doc JSON object as `content`; the server renders it to HTML with the correct CSS classes and structure. " +
				"Pass an optional Graph JSON object as `graph` (structured nodes/edges) and optional `references` (source links displayed at the bottom). " +
				deckDescription + "At least one of `content` and `deck` is required. " +
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
				deckDescription + "At least one of `content` and `deck` is required. " +
				"Keep the returned id; it is the handle for present_update/present_read/present_open.",
			Annotations: createAnnotations,
		}, withHint(h.handleCreate))
	}

	mcp.AddTool(s, &mcp.Tool{
		Name: "present_read",
		Description: "Read a presentation's current title, rendered HTML content, rendered deck HTML, graph JS, version, and URLs by id. " +
			"For editing, prefer present_source: it returns the structured source in the format present_update accepts.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, withHint(h.handleRead))

	mcp.AddTool(s, &mcp.Tool{
		Name: "present_source",
		Description: "Get a presentation's editable source by id: the Doc JSON it was created from (content_format=doc), the structured graph JSON (graph_format=json), the deck's Doc JSON (`deck`, empty when the page has no deck), plus references. " +
			"All come back in exactly the format present_update accepts, so you can modify them and pass them straight back. Use this to mutate a page from a new or restored session. " +
			"Legacy pages return content_format=html (raw HTML) or graph_format=js (raw JS); those can only be edited in that form.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, withHint(h.handleSource))

	mcp.AddTool(s, &mcp.Tool{
		Name: "present_update",
		Description: "Update an existing presentation. The `id` parameter is REQUIRED: it is the page id returned by present_create. " +
			"Provide a Doc JSON object as `content`, a Graph JSON object as `graph`, and/or a Doc JSON object as `deck`. " +
			deckDescription +
			"Only the fields you provide are changed (omit a field to leave it as-is); pass an empty string to clear the graph or to remove the deck. " +
			"A page always keeps at least one of content and deck: an update that would remove the last one is refused. " +
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
				"Set `deck` to open the page's slide deck instead of its content. " +
				"Call this AT MOST ONCE per presentation: after the tab is open, present_update triggers an automatic reload, " +
				"so don't call present_open again for subsequent edits. " +
				"If this fails (sandbox or PATH issue), return the URL from present_create to the user instead.",
			Annotations: &mcp.ToolAnnotations{
				DestructiveHint: new(false),
				IdempotentHint:  false,
				OpenWorldHint:   new(false),
			},
		}, withHint(h.handleOpen))

		// Remote control needs a store that relays commands to open tabs;
		// only the filesystem store does, and only local mode runs on it
		// unwrapped.
		if _, ok := h.deckController(); ok {
			mcp.AddTool(s, &mcp.Tool{
				Name: "present_deck",
				Description: "Drive the slide deck of a page that is open in the browser: `start` and `stop` presenting " +
					"(the chrome hides and one slide fills the window; browser fullscreen needs a click, the F key, or the Present button), " +
					"`next`, `prev`, or `goto` a 1-based `slide`. On a slide with `reveal` or a chart with `steps`, `next` and `prev` walk the items and the chart's steps the way the keys do, and `goto` lands with every step shown. Every open tab of the deck follows within a second. " +
					"The reader can also use the keyboard: Right, Space, or PageDown for the next slide, Left or PageUp for the previous, " +
					"Home and End for the first and last, N for the speaker notes drawer, F or P to start presenting (pressed while presenting they go fullscreen again after a reload, and in fullscreen they stop), Esc to stop. " +
					"Open the deck first with present_open(deck: true) or its deck_url.",
				Annotations: &mcp.ToolAnnotations{
					DestructiveHint: new(false),
					IdempotentHint:  false,
					OpenWorldHint:   new(false),
				},
			}, withHint(h.handleDeck))
		}

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

// deckURL is the public URL of a page's slide deck: the page URL with the
// deck view's suffix.
func deckURL(pageURL string) string {
	return pageURL + "/deck"
}

// deckController returns the store's remote-control surface when it has
// one. The filesystem store does; a wrapped or cluster store does not, and
// then neither present_deck nor its handler exist.
func (h *handlers) deckController() (store.DeckController, bool) {
	dc, ok := h.store.(store.DeckController)
	return dc, ok
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

// resolveDeck compiles a deck's Doc JSON to the HTML fragment deck.html holds
// and returns it with the canonical Doc JSON deck.json holds. Unlike content,
// a deck has no legacy form: anything but a Doc object is refused.
func resolveDeck(s string, title string) (htmlOut string, docJSON []byte, err error) {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return "", nil, nil
	}
	if s[0] != '{' {
		return "", nil, errors.New("deck: want Doc JSON (an object with sections, one per slide)")
	}
	c, err := render.CompileDeck([]byte(s), title)
	if err != nil {
		return "", nil, fmt.Errorf("deck: %w", err)
	}
	return c.HTML, c.JSON, nil
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

// parseGraphAndRender decodes GraphInput JSON and compiles it the way every
// graph write path does, so the MCP tools and the markdown import share one
// implementation.
func parseGraphAndRender(data []byte) (js string, srcJSON []byte, err error) {
	var g render.GraphInput
	if err := json.Unmarshal(data, &g); err != nil {
		return "", nil, fmt.Errorf("parse graph: %w", err)
	}
	return render.CompileGraph(g)
}

// ── create ──

type refInput struct {
	Title string `json:"title" jsonschema:"display label for the link"`
	URL   string `json:"url"   jsonschema:"full URL (https://...) pointing at repos, docs, PRs, or any external material cited in the brief"`
}

type createInput struct {
	Title      string     `json:"title"                jsonschema:"presentation title (shown in the browser tab and page header)"`
	Content    string     `json:"content,omitempty"    jsonschema:"page content as a Doc JSON string {summary?, meta?, chips?, sections:[{h, blocks:[{t,...}]}]} or a legacy HTML string; omit it for a page that is a slide deck only (then deck is required). Doc block types: p, h3, callout (sev: info|warn|ok|error), table (cols+rows), kv ([{k,v}]), list (items, ordered?), panel (title, sub?, accent?), progress (pct, label?), graph (placement marker), chart (kind: bar|line|area|sparkline|stacked-bar|horizontal-bar|doughnut|scatter|sankey, title?, unit?, xunit?, series:[{name?, color?, points:[{x,y}], step?}] for every kind but sankey, flows:[{from,to,value}] for sankey, steps:[{caption}] to walk the chart one step per Next on a deck slide (a series with step k joins at step k, the brief lists the captions under the chart); inline metric chart, many per page), code (text + lang?, verbatim code block with copy button, no inline markdown), html (raw passthrough), columns (cols: 2 or 3 arrays of blocks; equal widths, one column on a narrow screen; three stats in it make a row of figures, a chart beside a p puts the caption next to the chart), stat (value, label, sub?; a large figure over a label, value shown verbatim), quote (text, cite?), details (summary + blocks; collapsible, closed by default, for a long timeline or raw numbers), image (src, alt, caption?; src is an http(s) URL or, on a local instance, an absolute or ~ path to a png, jpeg, gif, or webp file of at most 2 MiB, which the tool copies into the page store and rewrites to the /img/<hash>.<ext> path it is served at, so present_source returns that path; alt is required because read-aloud reads it in the image's place; a shared instance takes image URLs only). A columns or details block holds any block but columns and details; the page's one graph may sit in a column (never in details), and a Doc places it at most once. Section fields beside h and blocks: tone (a palette role; bands the section in the brief, tints the slide in a deck), and deck-only layout (default|center|statement|section), notes (speaker notes), reveal (bool). Text fields support inline markdown: **bold**, *italic*, backtick-code, [text](url), @chip(style:text). Prefer the Doc format for compact structured input."`
	Deck       string     `json:"deck,omitempty"       jsonschema:"optional slide deck as a Doc JSON string with the same shape and block types as content: every section is one slide, summary/meta/chips fill the title slide, references make the last slide. Authored on its own, not converted from content. Keep it minimal: heading = the claim, 3 to 5 short list items or two sentences per slide, one bold phrase or stat chip per slide, one chart or the graph alone per slide, 5 to 12 slides, detail stays in content. Optional deck-level chrome fields beside summary/meta/chips: logo (absent = embedded repo logo, none, or an http(s) URL), logo_position (bottom-right|bottom-left|top-left|top-right), progress (dots|bar|none), presenter, footer (absent = the deck title, none suppresses it; the footer line shows only when presenter or footer is set), transition (fade default|slide|none). Per section: layout (default|center|statement|section), tone (palette role), notes (speaker notes shown in a drawer, N key), reveal (true: items and blocks appear one per Next; a chart with steps walks them right after it appears). A lone stat or quote on a slide is the big-number or quote slide. Served at deck_url; the page's graph is shared with the content."`
	Graph      string     `json:"graph,omitempty"      jsonschema:"optional Cytoscape graph as a structured JSON string {nodes:[{id,label,type?,color?,tone?}], edges:[{from,to,type?,label?,weight?,flow?}], layout?, direction?} or a legacy JS string. Node types: center, module, leaf, registry. Node tones (box background, border, and text as one family, theme-aware): neutral, green, red, blue, amber, purple. Edge types: consumes (solid), publishes (dashed). Edge weight (a number, e.g. requests per second) drives line width and tints the busiest edges; flow: true animates dashes from source to target. Layouts: dagre (default, layered DAG), elk (ELK layered; folds a long chain into rows to fit the container), elk-layered | elk-mrtree | elk-stress | elk-radial | elk-force (other ELK algorithms, no folding), cose (no hierarchy). Direction (dagre and elk): TB or LR; omit for auto (LR when the graph has few nodes)."`
	References []refInput `json:"references,omitempty" jsonschema:"source links shown in a References section at the bottom of the page: repos, docs, PRs consulted while writing the brief"`
}

// errNoRendition is the create-time refusal of a page with nothing to show.
var errNoRendition = errors.New("content or deck is required")

// errLastRendition is the update-time refusal of a patch that would leave
// the page with nothing to show.
var errLastRendition = errors.New("update would leave the page with neither content nor deck")

// sharedCreateInput is createInput plus the one choice a shared page adds.
type sharedCreateInput struct {
	createInput
	Ephemeral bool `json:"ephemeral,omitempty" jsonschema:"expire the page 30 days after its last update instead of keeping it until deleted"`
}

type pageOutput struct {
	ID        string `json:"id"`
	URL       string `json:"url"                  jsonschema:"where the page opens: its content, or its deck when the page has no content"`
	HasDeck   bool   `json:"has_deck"`
	DeckURL   string `json:"deck_url,omitempty"   jsonschema:"where the slide deck opens; set when the page has one"`
	Version   int    `json:"version"`
	Ephemeral bool   `json:"ephemeral,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// pageOutputFor describes p at pageURL. A page without content opens on its
// deck, so its URL is the deck's.
func pageOutputFor(p store.Page, pageURL string) pageOutput {
	out := pageOutput{
		ID: p.ID, URL: pageURL, HasDeck: p.HasDeck, Version: p.Version, Ephemeral: p.Ephemeral,
	}
	if p.HasDeck {
		out.DeckURL = deckURL(pageURL)
		if !p.HasBrief {
			out.URL = out.DeckURL
		}
	}
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
	if strings.TrimSpace(in.Content) == "" && strings.TrimSpace(in.Deck) == "" {
		return nil, pageOutput{}, errNoRendition
	}
	// Image files named by the Docs become stored images first, so the
	// Docs compile against the paths the page will keep; the files are
	// written only once both Docs compiled.
	ing := h.newImageIngest()
	contentSrc, err := ing.doc(in.Content)
	if err != nil {
		return nil, pageOutput{}, fmt.Errorf("content: %w", err)
	}
	deckSrc, err := ing.doc(in.Deck)
	if err != nil {
		return nil, pageOutput{}, fmt.Errorf("deck: %w", err)
	}
	content, docJSON, err := resolveContent(contentSrc, in.Title)
	if err != nil {
		return nil, pageOutput{}, fmt.Errorf("content: %w", err)
	}
	deck, deckJSON, err := resolveDeck(deckSrc, in.Title)
	if err != nil {
		return nil, pageOutput{}, err
	}
	graph, graphJSON, err := resolveGraph(in.Graph)
	if err != nil {
		return nil, pageOutput{}, fmt.Errorf("graph: %w", err)
	}
	if err := ing.commit(); err != nil {
		return nil, pageOutput{}, err
	}
	// The structured sources ride along (nil for legacy HTML/JS input) so a
	// future renderer/template change can re-render the page from source and
	// present_source can hand the source back for edits.
	d := store.Draft{
		Title:       in.Title,
		Content:     content,
		Deck:        deck,
		Graph:       graph,
		References:  toStoreRefs(in.References),
		Doc:         docJSON,
		DeckSource:  deckJSON,
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
	Deck       string     `json:"deck"                 jsonschema:"rendered HTML of the slide deck; empty when the page has none"`
	HasDeck    bool       `json:"has_deck"`
	Graph      string     `json:"graph"`
	References []refInput `json:"references,omitempty"`
	Version    int        `json:"version"`
	URL        string     `json:"url"`
	DeckURL    string     `json:"deck_url,omitempty"`
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
	urls := pageOutputFor(p, h.url(req, p.ID))
	return nil, readOutput{
		ID: p.ID, Title: p.Title, Content: p.Content, Deck: p.Deck, HasDeck: p.HasDeck, Graph: p.Graph,
		References: fromStoreRefs(p.References),
		Version:    p.Version, URL: urls.URL, DeckURL: urls.DeckURL,
		UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
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
	Deck          string     `json:"deck,omitempty"         jsonschema:"the slide deck's Doc JSON (editable, pass back to present_update as deck); empty when the page has no deck"`
	GraphFormat   string     `json:"graph_format,omitempty" jsonschema:"json = structured GraphInput JSON (editable); js = legacy raw JS; empty = page has no graph"`
	Graph         string     `json:"graph,omitempty"`
	References    []refInput `json:"references,omitempty"`
	Version       int        `json:"version"`
	URL           string     `json:"url"`
	DeckURL       string     `json:"deck_url,omitempty"`
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
	urls := pageOutputFor(p, h.url(req, p.ID))
	out := sourceOutput{
		ID: p.ID, Title: p.Title,
		References: fromStoreRefs(p.References),
		Version:    p.Version, URL: urls.URL, DeckURL: urls.DeckURL,
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
	if p.HasDeck {
		// A deck always comes from a Doc, so a missing source is a page
		// written by something other than these tools; hand back nothing
		// rather than the rendered HTML, which present_update would refuse.
		src, err := h.store.LoadDeckSource(ctx, in.ID)
		switch {
		case err == nil:
			out.Deck = string(src)
		case errors.Is(err, store.ErrNotFound):
		default:
			return nil, sourceOutput{}, err
		}
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
	Deck       *string     `json:"deck,omitempty"       jsonschema:"new slide deck as a Doc JSON string (one section per slide, heading = the claim, 3 to 5 short items or two sentences per slide, one highlight per slide, 5 to 12 slides; optional chrome fields logo, logo_position, progress, presenter, footer as in present_create); omit to leave unchanged, empty string to remove the deck"`
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
	deck       []byte
	clearDeck  bool
	graph      []byte
	clearGraph bool
}

func (h *handlers) handleUpdate(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in updateInput,
) (*mcp.CallToolResult, pageOutput, error) {
	// The current page is read first, before any rendering: on a shared
	// instance only the author may write, and an update from the wrong key
	// must cost this instance nothing but a store read; in either mode the
	// patch is judged against what the page holds now.
	cur, err := h.store.Get(ctx, in.ID)
	if err != nil {
		return nil, pageOutput{}, err
	}
	if h.mode == ModeShared {
		if err := author.Check(cur.Author, header(req)); err != nil {
			return nil, pageOutput{}, err
		}
	}
	ing := h.newImageIngest()
	in, err = ing.update(in)
	if err != nil {
		return nil, pageOutput{}, err
	}
	patch, plan, err := resolveUpdate(in, cur.Title)
	if err != nil {
		return nil, pageOutput{}, err
	}
	if err := ing.commit(); err != nil {
		return nil, pageOutput{}, err
	}
	// A page that has something to show keeps something to show. The check
	// runs on a copy: the store applies the real patch. A page from before
	// decks that has neither rendition (empty content was accepted then)
	// still takes title, graph, and reference updates.
	next := cur
	next.Apply(patch)
	if (cur.HasBrief || cur.HasDeck) && !next.HasBrief && !next.HasDeck {
		return nil, pageOutput{}, errLastRendition
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
// Doc or Graph fails before anything is written. curTitle is the page's
// title as stored: a Doc renders its title into the hero, so an update that
// leaves the title alone must still compile under it.
func resolveUpdate(in updateInput, curTitle string) (store.Patch, sourcePlan, error) {
	title := curTitle
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
		// A Doc refreshes doc.json. Legacy HTML, or an empty string that
		// removes the brief, leaves no Doc the content came from, so any
		// prior doc.json is stale and must go, or a later re-render or
		// present_source would bring the old brief back.
		plan.doc, plan.clearDoc = docJSON, docJSON == nil
	}
	if in.Deck != nil {
		deck, deckJSON, err := resolveDeck(*in.Deck, title)
		if err != nil {
			return store.Patch{}, sourcePlan{}, err
		}
		patch.Deck = &deck
		// A deck is replaced from a Doc or removed; either way the old
		// deck.json must not outlive it.
		plan.deck, plan.clearDeck = deckJSON, deckJSON == nil
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
// sources alone. Deletes go first: a store that measures the page on every
// save (the shared instance's size cap) must not count a source the same
// update is removing, or an update that shrinks the page could be refused
// halfway through.
func (h *handlers) applySourcePlan(ctx context.Context, id string, plan sourcePlan) error {
	if plan.clearDoc {
		if err := h.store.DeleteDoc(ctx, id); err != nil {
			return fmt.Errorf("clear doc: %w", err)
		}
	}
	if plan.clearDeck {
		if err := h.store.DeleteDeckSource(ctx, id); err != nil {
			return fmt.Errorf("clear deck source: %w", err)
		}
	}
	if plan.clearGraph {
		if err := h.store.DeleteGraphSource(ctx, id); err != nil {
			return fmt.Errorf("clear graph source: %w", err)
		}
	}
	if plan.doc != nil {
		if err := h.store.SaveDoc(ctx, id, plan.doc); err != nil {
			return fmt.Errorf("save doc: %w", err)
		}
	}
	if plan.deck != nil {
		if err := h.store.SaveDeckSource(ctx, id, plan.deck); err != nil {
			return fmt.Errorf("save deck source: %w", err)
		}
	}
	if plan.graph != nil {
		if err := h.store.SaveGraphSource(ctx, id, plan.graph); err != nil {
			return fmt.Errorf("save graph source: %w", err)
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
	HasDoc    bool   `json:"has_doc"            jsonschema:"true when the page has a persisted Doc source; present_source returns it ready for editing"`
	HasBrief  bool   `json:"has_brief"          jsonschema:"true when the page has content (the scrollable brief)"`
	HasDeck   bool   `json:"has_deck"           jsonschema:"true when the page has a slide deck"`
	DeckURL   string `json:"deck_url,omitempty" jsonschema:"where the slide deck opens; set when the page has one"`
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
		urls := pageOutputFor(p, h.url(req, p.ID))
		out.Pages = append(out.Pages, listItem{
			ID: p.ID, Title: p.Title, URL: urls.URL,
			Version: p.Version, UpdatedAt: p.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			HasDoc: p.HasDoc, HasBrief: p.HasBrief, HasDeck: p.HasDeck, DeckURL: urls.DeckURL,
		})
	}
	return nil, out, nil
}

// ── open ──

type openInput struct {
	ID   string `json:"id"             jsonschema:"page id to open in the browser"`
	Deck bool   `json:"deck,omitempty" jsonschema:"open the page's slide deck instead of its content"`
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
	p, err := h.store.Get(ctx, in.ID)
	if err != nil {
		return nil, openOutput{}, err
	}
	urls := pageOutputFor(p, h.url(req, in.ID))
	url := urls.URL
	if in.Deck {
		if !p.HasDeck {
			return nil, openOutput{}, store.ErrNoDeck
		}
		url = urls.DeckURL
	}
	if err := h.open(url); err != nil {
		return nil, openOutput{URL: url, Opened: false}, fmt.Errorf("open %s: %w", url, err)
	}
	return nil, openOutput{URL: url, Opened: true}, nil
}

// ── deck ──

type deckInput struct {
	ID     string `json:"id"              jsonschema:"page id whose deck is open in the browser"`
	Action string `json:"action"          jsonschema:"start | stop | next | prev | goto"`
	Slide  int    `json:"slide,omitempty" jsonschema:"1-based slide number; required for goto, ignored otherwise"`
}

type deckOutput struct {
	Seq     int64  `json:"seq"             jsonschema:"sequence number of the command; every open deck tab applies commands newer than the one it last saw"`
	Action  string `json:"action"`
	Slide   int    `json:"slide,omitempty"`
	DeckURL string `json:"deck_url"`
}

// handleDeck records a remote command for the page's open deck tabs. The
// tool is registered only when the store relays commands, but the handler
// checks again so a call can never reach a store that does not.
func (h *handlers) handleDeck(
	ctx context.Context,
	req *mcp.CallToolRequest,
	in deckInput,
) (*mcp.CallToolResult, deckOutput, error) {
	dc, ok := h.deckController()
	if !ok {
		return nil, deckOutput{}, errors.New("this store cannot relay deck commands")
	}
	cmd, err := dc.SendDeckCommand(ctx, in.ID, in.Action, in.Slide)
	if err != nil {
		return nil, deckOutput{}, err
	}
	notify.EmitEvent("present", "info", "deck command: "+cmd.Action, "",
		map[string]string{"id": in.ID, "action": cmd.Action})
	return nil, deckOutput{
		Seq: cmd.Seq, Action: cmd.Action, Slide: cmd.Slide,
		DeckURL: deckURL(h.url(req, in.ID)),
	}, nil
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

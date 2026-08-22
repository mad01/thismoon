package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/events/internal/client"
	"github.com/mad01/thismoon/services/events/internal/event"
)

// handlers carries the dependencies shared by all event tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views the event timeline in a browser
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "events_query",
		Description: "Query the local event/audit log, newest first — the primary tool for debugging what happened. " +
			"Optional filters: `source` (e.g. 'deps', 'reminder'), `level` ('info'|'warn'|'error'), " +
			"`q` (case-insensitive substring over title, message, component, and tags), " +
			"`since` (an event id; returns only events newer than it — use it to poll for new activity), " +
			"and `limit` (max events, defaults to the global cap). Use events_sources first to see which sources exist. " +
			"On zero results the response carries zero_result_hint (whether the source filter names a real source, the known sources, the time the since cursor decodes to) — read it before assuming nothing happened.",
	}, h.handleQuery)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "events_sources",
		Description: "List every event source with its current event count, sorted by name. Use it to discover which sources to filter events_query by.",
	}, h.handleSources)

	mcp.AddTool(s, &mcp.Tool{
		Name: "events_emit",
		Description: "Record an event in the local audit log. `source` and `title` are REQUIRED. " +
			"Optional `level` ('info' default | 'warn' | 'error'), `component` (sub-area within the source), " +
			"`message` (longer detail), and `tags` (a flat object of string key/values). Returns the new event id.",
	}, h.handleEmit)
}

// ── query ──

type queryInput struct {
	Source string `json:"source,omitempty" jsonschema:"filter to one source (e.g. 'deps')"`
	Level  string `json:"level,omitempty"  jsonschema:"filter by level: info | warn | error"`
	Q      string `json:"q,omitempty"      jsonschema:"case-insensitive substring over title, message, component, and tags"`
	Since  string `json:"since,omitempty"  jsonschema:"an event id; return only events newer than it (exclusive)"`
	Limit  int    `json:"limit,omitempty"  jsonschema:"max events to return; defaults to the global cap"`
}

type queryOutput struct {
	Events   []client.Event `json:"events"`
	URL      string         `json:"url"`
	ZeroHint *queryZeroHint `json:"zero_result_hint,omitempty" jsonschema:"set only on zero results: what the filters actually applied to, so an empty log is distinguishable from a wrong source name or an over-tight since cursor"`
}

// queryZeroHint explains an empty events_query so an agent can tell "nothing
// happened" from "the source filter names no real source" or "the since
// cursor excluded everything". Additive: it appears only on zero results.
type queryZeroHint struct {
	SourceFilter string   `json:"source_filter,omitempty" jsonschema:"the source filter that was applied"`
	SourceExists bool     `json:"source_exists"           jsonschema:"whether the source filter names a source that exists; only meaningful when source_filter is set"`
	KnownSources []string `json:"known_sources,omitempty" jsonschema:"every source that exists, from events_sources"`
	SinceTime    string   `json:"since_time,omitempty"    jsonschema:"the timestamp the since cursor decodes to (RFC3339); only events newer than this were considered"`
	EventsStored int      `json:"events_stored"           jsonschema:"total events across all sources"`
	Notes        []string `json:"notes,omitempty"         jsonschema:"targeted suggestions naming why the query came back empty"`
}

func (h *handlers) handleQuery(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in queryInput,
) (*mcp.CallToolResult, queryOutput, error) {
	evs, err := h.client.Query(client.QueryFilter{
		Source: in.Source,
		Level:  in.Level,
		Q:      in.Q,
		Since:  in.Since,
		Limit:  in.Limit,
	})
	if err != nil {
		return nil, queryOutput{}, err
	}
	outp := queryOutput{Events: evs, URL: h.webURL}
	if len(evs) == 0 {
		outp.ZeroHint = h.buildQueryZeroHint(in)
	}
	return nil, outp, nil
}

// buildQueryZeroHint compares the query's filters against the live source
// list. Best-effort: it returns nil when the extra lookup fails, leaving the
// plain empty result. The source filter is sanitized the same way the query
// path sanitizes it, so the hint judges the name the store actually matched.
func (h *handlers) buildQueryZeroHint(in queryInput) *queryZeroHint {
	srcs, err := h.client.Sources()
	if err != nil {
		return nil
	}

	want := in.Source
	if in.Source != "" {
		if s, err := event.SanitizeSource(in.Source); err == nil {
			want = s
		}
	}

	hint := &queryZeroHint{SourceFilter: in.Source}
	sourceEvents := 0
	for _, s := range srcs {
		hint.KnownSources = append(hint.KnownSources, s.Source)
		hint.EventsStored += s.Count
		if in.Source != "" && s.Source == want {
			hint.SourceExists = true
			sourceEvents = s.Count
		}
	}
	if since, ok := event.TimeFromID(in.Since); ok {
		hint.SinceTime = since.Format(time.RFC3339)
	}
	hint.Notes = queryZeroNotes(in, hint, sourceEvents)
	return hint
}

// queryZeroNotes phrases the one note that names why the query came back
// empty, given the source stats the hint carries. When several filters are
// active the note names all of them rather than guessing which one excluded
// everything.
func queryZeroNotes(in queryInput, hint *queryZeroHint, sourceEvents int) []string {
	if hint.EventsStored == 0 {
		return []string{"no events are stored; nothing has been emitted yet"}
	}
	if in.Source != "" && !hint.SourceExists {
		return []string{fmt.Sprintf(
			"source %q does not exist; known sources: %s",
			in.Source, strings.Join(hint.KnownSources, ", "),
		)}
	}
	if in.Since != "" && hint.SinceTime == "" {
		return []string{fmt.Sprintf(
			"the since cursor %q does not decode as an event id and may exclude everything; drop it or pass an id returned by a previous query",
			in.Since,
		)}
	}

	scope := fmt.Sprintf("the log holds %d events", hint.EventsStored)
	if in.Source != "" {
		scope = fmt.Sprintf("source %q holds %d events", in.Source, sourceEvents)
	}
	var filters []string
	if in.Since != "" {
		filters = append(filters, "since cursor")
	}
	if in.Level != "" {
		filters = append(filters, "level filter")
	}
	if in.Q != "" {
		filters = append(filters, "q filter")
	}
	if len(filters) == 0 {
		return []string{scope}
	}
	note := fmt.Sprintf("%s; the %s excluded them all", scope, strings.Join(filters, " and "))
	if in.Since != "" {
		note += "; drop since to include older events"
	}
	return []string{note}
}

// ── sources ──

type sourcesOutput struct {
	Sources []client.SourceCount `json:"sources"`
	URL     string               `json:"url"`
}

func (h *handlers) handleSources(
	_ context.Context,
	_ *mcp.CallToolRequest,
	_ struct{},
) (*mcp.CallToolResult, sourcesOutput, error) {
	srcs, err := h.client.Sources()
	if err != nil {
		return nil, sourcesOutput{}, err
	}
	return nil, sourcesOutput{Sources: srcs, URL: h.webURL}, nil
}

// ── emit ──

type emitInput struct {
	Source    string            `json:"source"              jsonschema:"event source, e.g. 'deps' (required)"`
	Title     string            `json:"title"               jsonschema:"short summary of the event (required)"`
	Level     string            `json:"level,omitempty"     jsonschema:"info (default) | warn | error"`
	Component string            `json:"component,omitempty" jsonschema:"optional sub-area within the source"`
	Message   string            `json:"message,omitempty"   jsonschema:"optional longer detail"`
	Tags      map[string]string `json:"tags,omitempty"      jsonschema:"optional flat object of string key/values"`
}

type emitOutput struct {
	ID  string `json:"id"`
	URL string `json:"url" jsonschema:"web page where the user can view the event timeline"`
}

func (h *handlers) handleEmit(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in emitInput,
) (*mcp.CallToolResult, emitOutput, error) {
	id, err := h.client.Emit(client.EmitBody{
		Source:    in.Source,
		Title:     in.Title,
		Level:     in.Level,
		Component: in.Component,
		Message:   in.Message,
		Tags:      in.Tags,
	})
	if err != nil {
		return nil, emitOutput{}, err
	}
	return nil, emitOutput{ID: id, URL: h.webURL}, nil
}

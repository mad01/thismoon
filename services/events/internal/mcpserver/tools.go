package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/kit/doctor"
	"github.com/mad01/thismoon/kit/timespan"
	"github.com/mad01/thismoon/services/events/internal/client"
	"github.com/mad01/thismoon/services/events/internal/event"
)

// handlers carries the dependencies shared by all event tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views the event timeline in a browser
	checks func(ctx context.Context) []doctor.Check
}

func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "events_query",
		Description: "Query the local event/audit log, newest first, the primary tool for debugging what happened. " +
			"Optional filters: `source` (e.g. 'deps', 'reminder'), `level` ('info'|'warn'|'error'), " +
			"`q` (case-insensitive substring over title, message, component, and tags), " +
			"`since` (a time window, the same grammar moondb_sessions takes: a span before now such as 90m, 24h or 7d, or an RFC 3339 time), " +
			"`after_id` (an event id from an earlier result; returns only events newer than it, so use it to poll for new activity), " +
			"and `limit` (max events, defaults to the global cap). Use events_sources first to see which sources exist. " +
			"A since or after_id value of the wrong shape is an error that names the parameter to use. " +
			"On zero results the response carries zero_result_hint (whether the source filter names a real source, the known sources, the times since and after_id resolve to). Read it before assuming nothing happened.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleQuery)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "events_sources",
		Description: "List every event source with its current event count, sorted by name. Use it to discover which sources to filter events_query by.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleSources)

	mcp.AddTool(s, &mcp.Tool{
		Name: "events_emit",
		Description: "Record an event in the local audit log. `source` and `title` are REQUIRED. " +
			"Optional `level` ('info' default | 'warn' | 'error'), `component` (sub-area within the source), " +
			"`message` (longer detail), and `tags` (a flat object of string key/values). Returns the new event id.",
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: new(false),
			IdempotentHint:  false,
			OpenWorldHint:   new(false),
		},
	}, h.handleEmit)

	mcp.AddTool(s, &mcp.Tool{
		Name: "events_doctor",
		Description: "Diagnose events itself: is serve reachable, is the JSONL store readable, is the running " +
			"build the installed one. Returns one result per check with `ok` false if any failed. " +
			"Read-only: it probes, it changes nothing. " +
			"Call this when events_query comes back empty or events_emit errors: it separates a serve that is " +
			"down or on a different port from a log that genuinely has nothing in it.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleDoctor)
}

// ── query ──

type queryInput struct {
	Source  string `json:"source,omitempty"   jsonschema:"filter to one source (e.g. 'deps')"`
	Level   string `json:"level,omitempty"    jsonschema:"filter by level: info | warn | error"`
	Q       string `json:"q,omitempty"        jsonschema:"case-insensitive substring over title, message, component, and tags"`
	Since   string `json:"since,omitempty"    jsonschema:"a time window: a span before now such as 90m, 24h or 7d, or an RFC 3339 time; returns events stamped at or after it"`
	AfterID string `json:"after_id,omitempty" jsonschema:"an event id from an earlier result; return only events newer than it (exclusive), for polling"`
	Limit   int    `json:"limit,omitempty"    jsonschema:"max events to return; defaults to the global cap"`
}

type queryOutput struct {
	Events   []client.Event `json:"events"`
	URL      string         `json:"url"`
	ZeroHint *queryZeroHint `json:"zero_result_hint,omitempty" jsonschema:"set only on zero results: what the filters actually applied to, so an empty log is distinguishable from a wrong source name or an over-tight since window or after_id cursor"`
}

// queryZeroHint explains an empty events_query so an agent can tell "nothing
// happened" from "the source filter names no real source" or "the since
// window or after_id cursor excluded everything". Additive: it appears only
// on zero results.
type queryZeroHint struct {
	SourceFilter string   `json:"source_filter,omitempty" jsonschema:"the source filter this query applied"`
	SourceExists bool     `json:"source_exists"           jsonschema:"whether the source filter names a source that exists; only meaningful when source_filter is set"`
	KnownSources []string `json:"known_sources,omitempty" jsonschema:"every source that exists, from events_sources"`
	SinceTime    string   `json:"since_time,omitempty"    jsonschema:"the time the since window starts at (RFC3339); only events stamped at or after it were considered"`
	AfterIDTime  string   `json:"after_id_time,omitempty" jsonschema:"the time the after_id cursor decodes to (RFC3339); only events newer than it were considered"`
	EventsStored int      `json:"events_stored"           jsonschema:"total events across all sources"`
	Notes        []string `json:"notes,omitempty"         jsonschema:"targeted suggestions naming why the query came back empty"`
}

func (h *handlers) handleQuery(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in queryInput,
) (*mcp.CallToolResult, queryOutput, error) {
	in = legacyCursor(in)
	evs, err := h.client.Query(client.QueryFilter{
		Source:  in.Source,
		Level:   in.Level,
		Q:       in.Q,
		Since:   in.Since,
		AfterID: in.AfterID,
		Limit:   in.Limit,
	})
	if err != nil {
		return nil, queryOutput{}, err
	}
	outp := queryOutput{Events: evs, URL: h.webURL}
	if len(evs) == 0 {
		outp.ZeroHint = h.buildQueryZeroHint(in, time.Now())
	}
	return nil, outp, nil
}

// legacyCursor moves an event id passed as since to after_id, the reading
// serve gives it too, so the zero-result hint describes the cursor the
// query actually applied. since took an id before after_id existed.
func legacyCursor(in queryInput) queryInput {
	if in.AfterID == "" && event.IsID(in.Since) {
		in.AfterID, in.Since = in.Since, ""
	}
	return in
}

// buildQueryZeroHint compares the query's filters against the live source
// list. Best-effort: it returns nil when the extra lookup fails, leaving the
// plain empty result. The source filter is sanitized the same way the query
// path sanitizes it, so the hint judges the name the store actually matched.
func (h *handlers) buildQueryZeroHint(in queryInput, now time.Time) *queryZeroHint {
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
	// serve already rejected a since it cannot parse, so the error is moot.
	if since, err := timespan.Since(in.Since, now); err == nil && !since.IsZero() {
		hint.SinceTime = since.UTC().Format(time.RFC3339)
	}
	if after, ok := event.TimeFromID(in.AfterID); ok {
		hint.AfterIDTime = after.Format(time.RFC3339)
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
	scope := fmt.Sprintf("the log holds %d events", hint.EventsStored)
	if in.Source != "" {
		scope = fmt.Sprintf("source %q holds %d events", in.Source, sourceEvents)
	}
	var filters []string
	if in.Since != "" {
		filters = append(filters, "since window")
	}
	if in.AfterID != "" {
		filters = append(filters, "after_id cursor")
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
	switch {
	case in.Since != "" && in.AfterID != "":
		note += "; widen since or drop after_id to include older events"
	case in.Since != "":
		note += "; widen since (such as 7d) or drop it to include older events"
	case in.AfterID != "":
		note += "; drop after_id to include older events"
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

// ── doctor ──

type doctorInput struct{}

// handleDoctor runs the same checks as `events doctor` and returns the
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

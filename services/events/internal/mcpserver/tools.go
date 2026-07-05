package mcpserver

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/events/internal/client"
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
			"and `limit` (max events, defaults to the global cap). Use events_sources first to see which sources exist.",
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
	Source string `json:"source,omitempty" jsonschema_description:"filter to one source (e.g. 'deps')"`
	Level  string `json:"level,omitempty"  jsonschema_description:"filter by level: info | warn | error"`
	Q      string `json:"q,omitempty"      jsonschema_description:"case-insensitive substring over title, message, component, and tags"`
	Since  string `json:"since,omitempty"  jsonschema_description:"an event id; return only events newer than it (exclusive)"`
	Limit  int    `json:"limit,omitempty"  jsonschema_description:"max events to return; defaults to the global cap"`
}

type queryOutput struct {
	Events []client.Event `json:"events"`
	URL    string         `json:"url"`
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
	return nil, queryOutput{Events: evs, URL: h.webURL}, nil
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
	Source    string            `json:"source"              jsonschema_description:"event source, e.g. 'deps' (required)"`
	Title     string            `json:"title"               jsonschema_description:"short summary of the event (required)"`
	Level     string            `json:"level,omitempty"     jsonschema_description:"info (default) | warn | error"`
	Component string            `json:"component,omitempty" jsonschema_description:"optional sub-area within the source"`
	Message   string            `json:"message,omitempty"   jsonschema_description:"optional longer detail"`
	Tags      map[string]string `json:"tags,omitempty"      jsonschema_description:"optional flat object of string key/values"`
}

type emitOutput struct {
	ID  string `json:"id"`
	URL string `json:"url" jsonschema_description:"web page where the user can view the event timeline"`
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

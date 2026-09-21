# MCP servers

The `modelcontextprotocol/go-sdk` tool-handler shape used by `worklog`, `present`, `events`, and `humanizer`. Backed by the go-sdk and the repo MCP servers.

## A `handlers` struct carries shared dependencies

Group the dependencies every tool needs into one struct, constructed once. From `events/internal/mcpserver`:

```go
// handlers carries the dependencies shared by all event tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views the event timeline in a browser
	checks func(ctx context.Context) []doctor.Check
}
```

In this codebase the MCP server holds no state of its own — it is a thin HTTP client to the running `serve` process (the single writer, see `store.md`), so `handlers` wraps a `*client.Client`. A file-touching MCP (`present`, `worklog`) wraps a `*store.Store` instead. The `checks` field feeds the `events_doctor` tool; every serve-bearing component registers one (ADR-0009).

## Register tools with `mcp.AddTool`

Register each tool with a name and a description written *for the model* — say what it does, when to use it, and what to keep. Annotations tell the client what a call can do: `ReadOnlyHint` for queries, `DestructiveHint: new(false)` for an append, `OpenWorldHint: new(false)` for anything that stays on localhost. Handlers are methods on `handlers`:

```go
func registerTools(s *mcp.Server, h *handlers) {
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
		Name:        "events_sources",
		Description: "List every event source with its current event count, sorted by name. Use it to discover which sources to filter events_query by.",
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:  true,
			OpenWorldHint: new(false),
		},
	}, h.handleSources)
}
```

## Typed input and output structs with schema descriptions

Each tool has an input struct and an output struct. Tag fields with `json` and a `jsonschema` description so the generated schema documents itself to the model — the go-sdk's schema inferrer reads only the `jsonschema` tag, so a `jsonschema_description` tag is silently dropped. Use `omitempty` for optional fields:

```go
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
```

## Handler signature

A handler takes the context and request (both often unused, named `_`) plus the typed input, and returns `(*mcp.CallToolResult, OutputStruct, error)`. Keep it thin — call into the client or store, return the error as-is so the SDK surfaces it:

```go
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
```

Returning the structured output value (not a hand-built `CallToolResult`) lets the SDK encode it against the schema. Every output carries the shared web `URL` from `handlers`, so tool responses stay consistent.

## Wiring

The `mcp` subcommand on the CLI (see `cli.md`) constructs `handlers`, registers the tools, and serves over stdio. MCP binaries that run third-party package code are seatbelt-sandboxed; first-party ones that only HTTP-call localhost (`worklog`, `events`) run unsandboxed. Registration lives in `recipes/claude-mcp/servers.json`.

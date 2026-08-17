# MCP servers

The `modelcontextprotocol/go-sdk` tool-handler shape used by `worklog`, `present`, `reminder`, and `humanizer`. Backed by the go-sdk and the repo MCP servers.

## A `handlers` struct carries shared dependencies

Group the dependencies every tool needs into one struct, constructed once. From `reminder/internal/mcpserver`:

```go
// handlers carries the dependencies shared by all reminder tools.
type handlers struct {
	client *client.Client
	webURL string // where the user views reminders in a browser
}
```

In this codebase the MCP server holds no state of its own — it is a thin HTTP client to the running `serve` process (the single writer, see `store.md`), so `handlers` wraps a `*client.Client`. A file-touching MCP (`present`, `worklog`) wraps a `*store.Store` instead.

## Register tools with `mcp.AddTool`

Register each tool with a name and a description written *for the model* — say what it does, when to use it, and what to keep. Handlers are methods on `handlers`:

```go
func registerTools(s *mcp.Server, h *handlers) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "reminder_create",
		Description: "Create a reminder that fires a macOS notification at its due time. " +
			"Set the time ONE of two ways: `due` as an absolute RFC3339 timestamp, OR " +
			"`in` as a Go duration ('2h30m'). Keep the returned id — it is the handle " +
			"for reminder_get / reminder_edit / reminder_cancel.",
	}, h.handleCreate)

	mcp.AddTool(s, &mcp.Tool{
		Name:        "reminder_get",
		Description: "Get one reminder's full detail by id.",
	}, h.handleGet)
}
```

## Typed input and output structs with schema descriptions

Each tool has an input struct and an output struct. Tag fields with `json` and a `jsonschema_description` so the generated schema documents itself to the model. Use `omitempty` for optional fields:

```go
type createInput struct {
	Title  string `json:"title"            jsonschema_description:"what to be reminded about"`
	Due    string `json:"due,omitempty"    jsonschema_description:"absolute due time as RFC3339. Provide this OR 'in', not both."`
	In     string `json:"in,omitempty"     jsonschema_description:"relative due time as a Go duration from now. Provide this OR 'due', not both."`
	Repeat string `json:"repeat,omitempty" jsonschema_description:"recurrence: 'daily', 'weekly', or a Go duration like '24h'. Omit for a one-shot."`
}

type out struct {
	Reminder client.Reminder `json:"reminder"`
	URL      string          `json:"url" jsonschema_description:"web page where the user can view and manage reminders"`
}
```

## Handler signature

A handler takes the context and request (both often unused, named `_`) plus the typed input, and returns `(*mcp.CallToolResult, OutputStruct, error)`. Keep it thin — call into the client or store, return the error as-is so the SDK surfaces it:

```go
func (h *handlers) handleCreate(_ context.Context, _ *mcp.CallToolRequest, in createInput) (*mcp.CallToolResult, out, error) {
	r, err := h.client.Create(client.CreateBody{Title: in.Title, Due: in.Due, In: in.In, Repeat: in.Repeat})
	if err != nil {
		return nil, out{}, err
	}
	return nil, h.one(r), nil
}
```

Returning the structured output value (not a hand-built `CallToolResult`) lets the SDK encode it against the schema. A small helper like `h.one(r)` attaches shared fields (the web `URL`) so every tool response is consistent.

## Wiring

The `mcp` subcommand on the CLI (see `cli.md`) constructs `handlers`, registers the tools, and serves over stdio. MCP binaries that run third-party package code are seatbelt-sandboxed; first-party ones that only HTTP-call localhost (`worklog`, `reminder`) run unsandboxed. Registration lives in `recipes/claude-mcp/servers.json`.

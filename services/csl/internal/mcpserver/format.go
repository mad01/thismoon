package mcpserver

import (
	"context"
	"fmt"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/mcpformat"
	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

// formatParam is embedded in every tool input so each csl tool accepts a
// response_format parameter. The jsonschema description repeats on every
// tool in every session, so it stays short.
type formatParam struct {
	ResponseFormat string `json:"response_format,omitempty" jsonschema:"output encoding: text | json | jsonl | toon | csv | markdown-kv | xml; omit to use the configured default (config.yaml mcp.response_format, built-in text)"`
}

func (p formatParam) responseFormat() string { return p.ResponseFormat }

// responseFormatter is satisfied by every tool input through the embedded
// formatParam; withFormat reads the requested format through it.
type responseFormatter interface {
	responseFormat() string
}

// withFormat adapts a typed tool handler to the go-sdk boundary with Out
// erased to any, so the encoding of the result is decided per call:
//
//   - json returns the typed output, which the SDK marshals into
//     structuredContent plus a JSON text block, exactly as before;
//   - text uses render when it is non-nil, else the markdown-kv encoding;
//   - every other format goes through mcpformat.Encode.
//
// Non-JSON results carry a single text block and no structured output,
// because Claude Code forwards only structuredContent to the model when a
// result has both. The handler's own error is returned untouched.
func withFormat[In responseFormatter, Out any](
	h mcp.ToolHandlerFor[In, Out],
	render func(Out) string,
) mcp.ToolHandlerFor[In, any] {
	return func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		res, out, err := h(ctx, req, in)
		if err != nil {
			return nil, nil, err
		}
		format, err := resolveFormat(in.responseFormat())
		if err != nil {
			return nil, nil, err
		}
		if format == mcpformat.JSON {
			return res, out, nil
		}
		text, err := renderOutput(format, out, render)
		if err != nil {
			return nil, nil, err
		}
		return textResult(text), nil, nil
	}
}

// resolveFormat picks the effective format from the tool parameter and the
// config default. Config is loaded per call, like the handlers do, so a
// config.yaml edit takes effect without restarting the MCP server.
func resolveFormat(param string) (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", fmt.Errorf("load csl config: %w", err)
	}
	return mcpformat.Resolve(param, cfg.MCPResponseFormat())
}

// renderOutput encodes out in a non-JSON format. Text prefers the tool's
// dedicated renderer and falls back to markdown-kv for tools without one.
func renderOutput[Out any](format string, out Out, render func(Out) string) (string, error) {
	if format == mcpformat.Text {
		if render != nil {
			return render(out), nil
		}
		format = mcpformat.MarkdownKV
	}
	return mcpformat.Encode(format, out)
}

// textResult wraps rendered text as the only content of a tool result.
func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}
}

// addFormattedTool registers a tool through withFormat and advertises the
// response_format values as a schema enum, so clients and models can
// enumerate them instead of parsing the description. The schema is derived
// the same way the SDK would derive it; a derivation error is a programming
// error in the input type and panics at registration, as mcp.AddTool does.
func addFormattedTool[In responseFormatter, Out any](
	s *mcp.Server,
	t *mcp.Tool,
	h mcp.ToolHandlerFor[In, Out],
	render func(Out) string,
) {
	schema, err := jsonschema.For[In](nil)
	if err != nil {
		panic(fmt.Sprintf("mcpserver: input schema for %s: %v", t.Name, err))
	}
	prop, ok := schema.Properties["response_format"]
	if !ok {
		panic(fmt.Sprintf("mcpserver: %s input lacks response_format", t.Name))
	}
	prop.Enum = formatEnum()
	t.InputSchema = schema
	mcp.AddTool(s, t, withFormat(h, render))
}

// formatEnum lists the response formats as schema enum values.
func formatEnum() []any {
	names := mcpformat.Names()
	enum := make([]any, len(names))
	for i, n := range names {
		enum[i] = n
	}
	return enum
}

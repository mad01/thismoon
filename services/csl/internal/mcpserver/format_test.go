package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl/internal/mcpformat"
)

// stubInput and stubOutput exercise withFormat without touching csl state.
type stubInput struct {
	Name string `json:"name"`
	formatParam
}

type stubOutput struct {
	Greeting string `json:"greeting"`
}

func stubHandler(
	_ context.Context,
	_ *mcp.CallToolRequest,
	in stubInput,
) (*mcp.CallToolResult, stubOutput, error) {
	if in.Name == "" {
		return nil, stubOutput{}, errors.New("name is required")
	}
	return nil, stubOutput{Greeting: "hello " + in.Name}, nil
}

func renderStub(out stubOutput) string { return "greeting: " + out.Greeting }

// writeConfig isolates csl config under a temp home and writes body as
// config.yaml. An empty body leaves csl on defaults with no file.
func writeConfig(t *testing.T, body string) {
	t.Helper()
	home := t.TempDir()
	isolateConfigEnv(t, home)
	if body == "" {
		return
	}
	dir := filepath.Join(home, ".config", "csl")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
}

func stubCall(
	t *testing.T,
	format string,
) (*mcp.CallToolResult, any, error) {
	t.Helper()
	h := withFormat(stubHandler, renderStub)
	return h(context.Background(), nil, stubInput{
		Name:        "x",
		formatParam: formatParam{ResponseFormat: format},
	})
}

// wantTyped asserts the json path: no result, the typed output boxed in any.
func wantTyped(t *testing.T, res *mcp.CallToolResult, out any) {
	t.Helper()
	if res != nil {
		t.Errorf("res = %+v, want nil so the SDK builds structured output", res)
	}
	got, ok := out.(stubOutput)
	if !ok || got.Greeting != "hello x" {
		t.Errorf("out = %#v, want stubOutput{Greeting: \"hello x\"}", out)
	}
}

// wantText asserts the text path: one text block, no structured output.
func wantText(t *testing.T, res *mcp.CallToolResult, out any, want string) {
	t.Helper()
	if out != nil {
		t.Errorf("out = %#v, want nil so no structuredContent is emitted", out)
	}
	if res == nil {
		t.Fatal("res = nil, want a text result")
	}
	if res.StructuredContent != nil {
		t.Errorf("StructuredContent = %v, want nil", res.StructuredContent)
	}
	if len(res.Content) != 1 {
		t.Fatalf("Content has %d blocks, want 1", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	if tc.Text != want {
		t.Errorf("text = %q, want %q", tc.Text, want)
	}
}

func TestWithFormat_JSONReturnsTypedOutput(t *testing.T) {
	writeConfig(t, "")
	res, out, err := stubCall(t, "json")
	if err != nil {
		t.Fatalf("withFormat json: %v", err)
	}
	wantTyped(t, res, out)
}

func TestWithFormat_TextReturnsContentOnly(t *testing.T) {
	writeConfig(t, "")
	res, out, err := stubCall(t, "text")
	if err != nil {
		t.Fatalf("withFormat text: %v", err)
	}
	wantText(t, res, out, "greeting: hello x")
}

func TestWithFormat_DefaultIsText(t *testing.T) {
	writeConfig(t, "")
	res, out, err := stubCall(t, "")
	if err != nil {
		t.Fatalf("withFormat default: %v", err)
	}
	wantText(t, res, out, "greeting: hello x")
}

func TestWithFormat_UnknownParamErrors(t *testing.T) {
	writeConfig(t, "")
	_, _, err := stubCall(t, "bogus")
	if err == nil || !strings.Contains(err.Error(), `unknown response_format "bogus"`) {
		t.Errorf("err = %v, want unknown response_format error", err)
	}
}

func TestWithFormat_HandlerErrorWins(t *testing.T) {
	writeConfig(t, "")
	h := withFormat(stubHandler, renderStub)
	_, out, err := h(context.Background(), nil, stubInput{
		formatParam: formatParam{ResponseFormat: "bogus"},
	})
	if err == nil || err.Error() != "name is required" {
		t.Errorf("err = %v, want the handler's own error", err)
	}
	if out != nil {
		t.Errorf("out = %#v, want nil on error", out)
	}
}

func TestWithFormat_ConfigDefaultHonored(t *testing.T) {
	writeConfig(t, "mcp:\n  response_format: json\n")
	res, out, err := stubCall(t, "")
	if err != nil {
		t.Fatalf("withFormat with configured json: %v", err)
	}
	wantTyped(t, res, out)
}

func TestWithFormat_ParamOverridesConfig(t *testing.T) {
	writeConfig(t, "mcp:\n  response_format: json\n")
	res, out, err := stubCall(t, "text")
	if err != nil {
		t.Fatalf("withFormat text over configured json: %v", err)
	}
	wantText(t, res, out, "greeting: hello x")
}

func TestWithFormat_ConfigBogusErrors(t *testing.T) {
	writeConfig(t, "mcp:\n  response_format: bogus\n")
	_, _, err := stubCall(t, "")
	if err == nil || !strings.Contains(err.Error(), "mcp.response_format") {
		t.Errorf("err = %v, want an error naming mcp.response_format", err)
	}
}

// connect serves s over an in-memory transport and returns a client session.
func connect(t *testing.T, s *mcp.Server) (context.Context, *mcp.ClientSession) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := s.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(&mcp.Implementation{Name: "csl-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return ctx, clientSession
}

// schemaProperties decodes a tool's input schema as it arrived over the wire.
func schemaProperties(t *testing.T, schema any) (map[string]json.RawMessage, []string) {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal schema: %v", err)
	}
	var s struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Required   []string                   `json:"required"`
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return s.Properties, s.Required
}

// TestEveryToolAcceptsResponseFormat is the embedding gate: the go-sdk must
// promote formatParam's field into each tool's input schema as an optional
// property, and no tool may advertise an output schema, since a text-only
// result would then violate the MCP structured-content contract.
func TestEveryToolAcceptsResponseFormat(t *testing.T) {
	ctx, session := connect(t, New("test", Options{SemanticEnabled: true}))
	var seen int
	for tool, err := range session.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		seen++
		props, required := schemaProperties(t, tool.InputSchema)
		prop, ok := props["response_format"]
		if !ok {
			t.Errorf("%s: input schema lacks response_format", tool.Name)
			continue
		}
		if !strings.Contains(string(prop), "output encoding") {
			t.Errorf("%s: response_format lost its description: %s", tool.Name, prop)
		}
		var withEnum struct {
			Enum []string `json:"enum"`
		}
		if err := json.Unmarshal(prop, &withEnum); err != nil {
			t.Fatalf("%s: decode response_format schema: %v", tool.Name, err)
		}
		if !slices.Equal(withEnum.Enum, mcpformat.Names()) {
			t.Errorf("%s: response_format enum = %v, want %v", tool.Name, withEnum.Enum, mcpformat.Names())
		}
		for _, r := range required {
			if r == "response_format" {
				t.Errorf("%s: response_format must be optional", tool.Name)
			}
		}
		if tool.OutputSchema != nil {
			t.Errorf("%s: advertises an output schema; text results would violate it", tool.Name)
		}
	}
	if seen == 0 {
		t.Fatal("no tools registered; the gate would pass vacuously")
	}
}

// TestWithFormat_SDKBoundary proves the shape Claude Code sees: text yields a
// single text block and no structuredContent; json yields both, byte-identical.
func TestWithFormat_SDKBoundary(t *testing.T) {
	writeConfig(t, "")
	s := mcp.NewServer(&mcp.Implementation{Name: "stub", Version: "test"}, nil)
	mcp.AddTool(
		s,
		&mcp.Tool{Name: "stub_greet", Description: "test"},
		withFormat(stubHandler, renderStub),
	)
	ctx, session := connect(t, s)

	call := func(args map[string]any) *mcp.CallToolResult {
		t.Helper()
		res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "stub_greet", Arguments: args})
		if err != nil {
			t.Fatalf("call stub_greet %v: %v", args, err)
		}
		return res
	}

	text := call(map[string]any{"name": "x"})
	if text.IsError {
		t.Fatalf("text call errored: %v", text.Content)
	}
	if text.StructuredContent != nil {
		t.Errorf("text: StructuredContent = %v, want none", text.StructuredContent)
	}
	if got := textOf(t, text); got != "greeting: hello x" {
		t.Errorf("text: content = %q", got)
	}

	js := call(map[string]any{"name": "x", "response_format": "json"})
	if js.IsError {
		t.Fatalf("json call errored: %v", js.Content)
	}
	structured, err := json.Marshal(js.StructuredContent)
	if err != nil {
		t.Fatalf("marshal structured: %v", err)
	}
	const wantJSON = `{"greeting":"hello x"}`
	if string(structured) != wantJSON {
		t.Errorf("json: StructuredContent = %s, want %s", structured, wantJSON)
	}
	if got := textOf(t, js); got != wantJSON {
		t.Errorf("json: content = %q, want %q", got, wantJSON)
	}

	bad := call(map[string]any{"name": "x", "response_format": "bogus"})
	if !bad.IsError || !strings.Contains(textOf(t, bad), "unknown response_format") {
		t.Errorf(
			"bogus: IsError=%v content=%q, want a tool error naming the format",
			bad.IsError,
			textOf(t, bad),
		)
	}
}

func textOf(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) != 1 {
		t.Fatalf("Content has %d blocks, want 1", len(res.Content))
	}
	tc, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("Content[0] is %T, want *mcp.TextContent", res.Content[0])
	}
	return tc.Text
}

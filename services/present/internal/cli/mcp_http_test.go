package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/server"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// bearerTransport signs every request with an author key, which is how an
// MCP client registers against a shared instance.
type bearerTransport struct {
	key  string
	base http.RoundTripper
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("Authorization", "Bearer "+b.key)
	return b.base.RoundTrip(clone)
}

// serveSharedMCP builds the shared instance the way runServe does — the
// size-limited, expiry-filtered store behind both the HTTP server and the
// tools at /mcp — and serves it on a loopback port.
func serveSharedMCP(t *testing.T) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	fs, err := store.NewFS(dir)
	if err != nil {
		t.Fatal(err)
	}
	// The flags a shared instance runs with here: no display override, so
	// URLs come from each request's forwarded headers.
	baseURL, port := flagBaseURL, flagPort
	flagBaseURL, flagPort = "", 0
	t.Cleanup(func() { flagBaseURL, flagPort = baseURL, port })

	st := store.WithSizeLimit(store.WithoutExpired(fs, time.Now), 1<<20)
	handler, err := sharedMCP(st)
	if err != nil {
		t.Fatalf("sharedMCP: %v", err)
	}
	ts := httptest.NewServer(server.New(st, server.Options{
		Mode: server.ModeShared, Workdir: dir, MCP: handler,
	}).Handler())
	t.Cleanup(ts.Close)
	return ts
}

// connectMCP speaks the streamable HTTP transport to /mcp with key as the
// bearer token, the way a registered MCP client does.
func connectMCP(t *testing.T, ctx context.Context, url, key string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "present-test", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint: url,
		HTTPClient: &http.Client{
			Transport: bearerTransport{key: key, base: http.DefaultTransport},
		},
		DisableStandaloneSSE: true,
	}, nil)
	if err != nil {
		t.Fatalf("connect to %s: %v", url, err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// callTool runs a tool and fails the test when the transport itself broke.
// A tool error is a result, not a transport failure, so it comes back in
// the result for the caller to assert on.
func callTool(
	t *testing.T,
	ctx context.Context,
	s *mcp.ClientSession,
	name string,
	args map[string]any,
) *mcp.CallToolResult {
	t.Helper()
	res, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	return res
}

// resultText joins a tool result's text content, which is where a tool
// error's message lands.
func resultText(res *mcp.CallToolResult) string {
	var b strings.Builder
	for _, c := range res.Content {
		if text, ok := c.(*mcp.TextContent); ok {
			b.WriteString(text.Text)
		}
	}
	return b.String()
}

// TestSharedMCPOverHTTPAuthenticatesByBearerKey drives /mcp with a real
// streamable client: the key each connection sends becomes the author of
// what it creates, and a second key cannot update that page. It pins that
// the transport carries the Authorization header all the way into the tool
// handler, which is the only thing standing between a shared page and
// anyone who knows its id.
func TestSharedMCPOverHTTPAuthenticatesByBearerKey(t *testing.T) {
	ts := serveSharedMCP(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	owner := connectMCP(t, ctx, ts.URL+"/mcp", author.NewKey())
	res := callTool(t, ctx, owner, "present_create", map[string]any{
		"title": "Shared", "content": "<p>v1</p>",
	})
	if res.IsError {
		t.Fatalf("present_create failed: %s", resultText(res))
	}
	created, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("present_create returned %T, want an object", res.StructuredContent)
	}
	id, _ := created["id"].(string)
	if len(id) != 32 {
		t.Fatalf("id = %q, want a 32-hex capability id", id)
	}
	if url, _ := created["url"].(string); url != ts.URL+"/p/"+id {
		t.Errorf("url = %q, want %q derived from the request", url, ts.URL+"/p/"+id)
	}

	// The page is readable over the same transport by its id alone.
	if got := callTool(t, ctx, owner, "present_read", map[string]any{"id": id}); got.IsError {
		t.Fatalf("present_read failed: %s", resultText(got))
	}

	other := connectMCP(t, ctx, ts.URL+"/mcp", author.NewKey())
	refused := callTool(t, ctx, other, "present_update", map[string]any{
		"id": id, "content": "<p>v2</p>",
	})
	if !refused.IsError {
		t.Fatal("present_update with another key succeeded; want the author check to refuse it")
	}
	if !strings.Contains(resultText(refused), author.ErrMismatch.Error()) {
		t.Errorf("update refusal = %q, want %q", resultText(refused), author.ErrMismatch)
	}

	// The author still owns the page after the refused attempt.
	updated := callTool(t, ctx, owner, "present_update", map[string]any{
		"id": id, "content": "<p>v2</p>",
	})
	if updated.IsError {
		t.Fatalf("present_update by the author failed: %s", resultText(updated))
	}
}

// TestSharedHTTPRefusesAnOversizedPageWith413 pins the size cap on the push
// endpoint with the wiring serve builds: the body is small enough for the
// envelope reader, so the refusal can only come from the store behind it.
func TestSharedHTTPRefusesAnOversizedPageWith413(t *testing.T) {
	ts := serveSharedMCP(t)
	body, err := json.Marshal(map[string]any{
		"title":   "Huge",
		"content": "<p>" + strings.Repeat("x", present.MaxPageBytes) + "</p>",
	})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/pages", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+author.NewKey())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/pages: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized push = %d (%s), want 413", resp.StatusCode, out)
	}
}

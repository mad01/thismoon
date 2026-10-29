package csl_test

import (
	"context"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/mad01/thismoon/services/csl"
	"github.com/mad01/thismoon/services/csl/internal/mcpserver"
)

// claudeMDHeading is the README section that quotes the embedded fragment.
const claudeMDHeading = "### Add this to your CLAUDE.md"

// TestREADMEExcerptIsPartOfClaudeMD is the excerpt-can't-drift gate: the
// fenced block under claudeMDHeading in README.md quotes the routing rule
// users read on GitHub, and claude-md.md is the fragment `csl docs
// --claude-md` prints and the csl recipe links into ~/.claude. The quote must
// appear in the fragment byte for byte.
func TestREADMEExcerptIsPartOfClaudeMD(t *testing.T) {
	readme, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}

	excerpt := fencedBlockAfter(t, string(readme), claudeMDHeading)
	if excerpt == "" {
		t.Fatal("the README block is empty; the gate would pass vacuously")
	}
	if !strings.Contains(csl.ClaudeMD, excerpt) {
		t.Errorf(
			"the README block is not a verbatim part of claude-md.md; edit claude-md.md and copy the section.\n--- README.md ---\n%s",
			excerpt,
		)
	}
}

// TestClaudeMDNamesEveryTool is the tool-list gate: the fragment tells an agent
// which tools exist, so it must name every tool `csl mcp` registers, and must
// not name one that does not exist.
func TestClaudeMDNamesEveryTool(t *testing.T) {
	registered := registeredToolNames(t)
	if len(registered) == 0 {
		t.Fatal("no MCP tools registered; the gate would pass vacuously")
	}

	// A letter must follow csl_, so the mcp__csl__* tool prefix is not read
	// as a tool name.
	inSnippet := make(map[string]bool)
	for _, name := range regexp.MustCompile(`csl_[a-z][a-z_]*`).FindAllString(csl.ClaudeMD, -1) {
		inSnippet[name] = true
	}

	isRegistered := make(map[string]bool, len(registered))
	for _, name := range registered {
		isRegistered[name] = true
		if !inSnippet[name] {
			t.Errorf("tool %s is registered by csl mcp but missing from claude-md.md", name)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(inSnippet)) {
		if !isRegistered[name] {
			t.Errorf("claude-md.md names %s, which csl mcp does not register", name)
		}
	}
}

// registeredToolNames asks a live MCP server for its tools over an in-memory
// transport, so the expected list is the registration itself rather than a
// second hand-typed copy of it.
func registeredToolNames(t *testing.T) []string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	// Semantic enabled: the fragment documents the full tool set, including the
	// two tools a machine with semantic.enabled off never sees.
	opts := mcpserver.Options{SemanticEnabled: true}
	serverSession, err := mcpserver.New("test", opts).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("connect server: %v", err)
	}
	defer func() { _ = serverSession.Close() }()

	client := mcp.NewClient(&mcp.Implementation{Name: "csl-test", Version: "test"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer func() { _ = clientSession.Close() }()

	var names []string
	for tool, err := range clientSession.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("list tools: %v", err)
		}
		names = append(names, tool.Name)
	}
	return names
}

// fencedBlockAfter returns the contents of the first fenced code block that
// follows heading in md, with trailing whitespace trimmed.
func fencedBlockAfter(t *testing.T, md, heading string) string {
	t.Helper()

	_, after, found := strings.Cut(md, heading+"\n")
	if !found {
		t.Fatalf("README.md has no %q heading", heading)
	}
	_, body, found := strings.Cut(after, "\n```markdown\n")
	if !found {
		t.Fatalf("no ```markdown block follows %q in README.md", heading)
	}
	block, _, found := strings.Cut(body, "\n```")
	if !found {
		t.Fatalf("unterminated ```markdown block under %q in README.md", heading)
	}
	return strings.TrimRight(block, " \t\n")
}

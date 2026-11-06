package mcpserver

import (
	"context"
	"hash/fnv"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/brain/internal/semantic"
)

// hashEmbed is a deterministic 16-dim "embedding" used in semantic MCP
// tests. It hashes whitespace-tokenized input into bins and L2-normalizes
// the resulting vector. Notes that share words yield similar vectors —
// good enough to test ranking without an Ollama dependency.
func hashEmbed(_ context.Context, text string) ([]float32, error) {
	const dim = 16
	v := make([]float32, dim)
	for _, w := range strings.Fields(strings.ToLower(text)) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[int(h.Sum32())%dim] += 1
	}
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		v[0] = 1
		return v, nil
	}
	n := math.Sqrt(sum)
	for i := range v {
		v[i] = float32(float64(v[i]) / n)
	}
	return v, nil
}

// setupSemanticFixture builds the standard fixture vault and additionally
// writes a config with semantic.enabled=true (pointing at a temp index dir)
// and swaps the package-level embed function for a deterministic stub.
// Returns the vault root for callers that want to inspect files.
func setupSemanticFixture(t *testing.T) string {
	t.Helper()
	root := setupFixtureVault(t)

	// Rewrite config.yaml to enable semantic and pin index_dir under HOME.
	home, _ := os.UserHomeDir()
	indexDir := filepath.Join(home, ".config", "brain", "semantic")
	cfg := "vault: " + root + "\n" +
		"semantic:\n" +
		"  enabled: true\n" +
		"  ollama_url: http://127.0.0.1:0\n" + // tests never call Ollama
		"  embed_model: stub\n" +
		"  chat_model: stub\n" +
		"  index_dir: " + indexDir + "\n"
	cfgPath := filepath.Join(home, ".config", "brain", "config.yaml")
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}

	return root
}

// semanticServer returns handlers whose semantic layer embeds with
// hashEmbed instead of calling Ollama.
func semanticServer(t *testing.T) *handlers {
	t.Helper()
	h := testServer(t)
	h.embed = semantic.EmbedFunc(hashEmbed)
	return h
}

func TestBrainSemanticSearch_HappyPath(t *testing.T) {
	setupSemanticFixture(t)
	ctx := context.Background()

	_, out, err := semanticServer(t).handleSemanticSearch(ctx, nil, semanticSearchInput{
		Query: "concept alpha foundational ideas",
		K:     5,
	})
	if err != nil {
		t.Fatalf("handleSemanticSearch: %v", err)
	}
	if out.Total == 0 {
		t.Fatalf("got 0 hits, want at least 1")
	}
	// concept-alpha contains the query words; it should rank near the top.
	found := false
	for _, h := range out.Hits {
		if h.Path == "notes/concept-alpha.md" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected notes/concept-alpha.md in results, got: %+v", out.Hits)
	}
}

func TestBrainSemanticSearch_RequiresEnabled(t *testing.T) {
	setupFixtureVault(t) // semantic NOT enabled in this fixture
	_, _, err := testServer(
		t,
	).handleSemanticSearch(context.Background(), nil, semanticSearchInput{Query: "x"})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("want disabled error, got %v", err)
	}
}

func TestBrainSemanticSearch_TierFilter(t *testing.T) {
	setupSemanticFixture(t)
	_, out, err := semanticServer(
		t,
	).handleSemanticSearch(context.Background(), nil, semanticSearchInput{
		Query: "alpha",
		Tier:  "outputs",
		K:     5,
	})
	if err != nil {
		t.Fatalf("handleSemanticSearch: %v", err)
	}
	for _, h := range out.Hits {
		if h.Tier != "outputs" {
			t.Errorf("tier filter leaked: %+v", h)
		}
	}
}

func TestBrainSemanticSearch_PathsMode(t *testing.T) {
	setupSemanticFixture(t)
	_, out, err := semanticServer(
		t,
	).handleSemanticSearch(context.Background(), nil, semanticSearchInput{
		Query:      "concept",
		OutputMode: "paths",
		K:          10,
	})
	if err != nil {
		t.Fatalf("handleSemanticSearch: %v", err)
	}
	// In paths mode, no two hits should share a path (snippets dedupe).
	seen := make(map[string]bool)
	for _, h := range out.Hits {
		if seen[h.Path] {
			t.Errorf("duplicate path in paths mode: %s", h.Path)
		}
		seen[h.Path] = true
		if h.Snippet != "" {
			t.Errorf("paths mode should not include snippet, got %q", h.Snippet)
		}
	}
}

func TestBrainHybridSearch_FusesBothSources(t *testing.T) {
	setupSemanticFixture(t)
	_, out, err := semanticServer(
		t,
	).handleHybridSearch(context.Background(), nil, hybridSearchInput{
		Query: "concept alpha",
		K:     5,
	})
	if err != nil {
		t.Fatalf("handleHybridSearch: %v", err)
	}
	if out.Total == 0 {
		t.Fatalf("got 0 hits")
	}
	// At least one hit should report both retrievers.
	bothFound := false
	for _, h := range out.Hits {
		if len(h.Sources) == 2 {
			bothFound = true
		}
	}
	if !bothFound {
		t.Errorf("expected at least one hit with both sources; got: %+v", out.Hits)
	}
}

func TestBrainHybridSearch_RequiresEnabled(t *testing.T) {
	setupFixtureVault(t)
	_, _, err := testServer(
		t,
	).handleHybridSearch(context.Background(), nil, hybridSearchInput{Query: "x"})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("want disabled error, got %v", err)
	}
}

func TestBrainSemanticSearch_EmptyQueryRejected(t *testing.T) {
	setupSemanticFixture(t)
	_, _, err := semanticServer(
		t,
	).handleSemanticSearch(context.Background(), nil, semanticSearchInput{Query: "  "})
	if err == nil {
		t.Error("want error for empty query")
	}
}

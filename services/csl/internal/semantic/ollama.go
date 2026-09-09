package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/csl/internal/repo/config"
)

const (
	// defaultOllamaEndpoint is the local Ollama server address.
	defaultOllamaEndpoint = "http://localhost:11434"
	// defaultOllamaModel is the embedding model requested from Ollama:
	// jina-code-v2, code-trained, 8k ctx, light enough to bulk-index quickly
	// (MAD-235 benchmarked qwen3-embedding:0.6b at less than half its speed).
	defaultOllamaModel = "unclemusclez/jina-embeddings-v2-base-code:f16"
	// defaultOllamaDim is the embedding dimensionality of the default model.
	defaultOllamaDim = 768
	// ollamaKeepAlive is how long Ollama keeps the model resident after a
	// request. Interactive queries reuse the warm model; bulk indexing calls
	// Unload when done instead of leaving it loaded for this window.
	ollamaKeepAlive = "20m"
	// ollamaNumCtx is the context window requested per embed call. It must
	// exceed the largest chunk EmbedText (maxChunkBodyChars plus breadcrumb,
	// roughly 2k tokens) — Ollama silently truncates past its context.
	// num_batch must be raised to match: Ollama defaults llama-server to a
	// 2048-token physical batch, and an embedding input larger than that
	// crashes the runner (EOF mid-request) instead of erroring.
	ollamaNumCtx = 8192
	// queryInstruction is prefixed to query embeddings only, and only for
	// instruction-tuned models (qwen3-embedding): retrieval improves when the
	// query side states the task while documents are embedded bare. Models
	// without instruction tuning (jina-code, nomic-embed-text) embed the
	// prefix as literal text, which hurts ranking, so they get bare queries.
	queryInstruction = "Instruct: Given a code search query, retrieve the most relevant code\nQuery: "
)

// queryPrefixFor returns the query-side instruction prefix for a model, or ""
// when the model is not instruction-tuned.
func queryPrefixFor(model string) string {
	if strings.HasPrefix(model, "qwen3-embedding") {
		return queryInstruction
	}
	return ""
}

// OllamaEmbedder embeds text via an Ollama server's /api/embed endpoint. It is
// the only embedding backend: csl does not bundle a model, it expects the
// configured model to be pulled into Ollama (`ollama pull` the configured
// or default model).
type OllamaEmbedder struct {
	Endpoint   string
	Model      string
	HTTPClient *http.Client
	dim        int
	// queryPrefix is prepended to query embeddings (never documents); empty
	// for models that are not instruction-tuned.
	queryPrefix string
}

// NewOllamaEmbedder returns an OllamaEmbedder with sane defaults. Empty endpoint
// or model fall back to localhost:11434 and the default model; dim <= 0 falls
// back to the default model's dimensionality.
func NewOllamaEmbedder(endpoint, model string, dim int) *OllamaEmbedder {
	if endpoint == "" {
		endpoint = defaultOllamaEndpoint
	}
	if model == "" {
		model = defaultOllamaModel
	}
	if dim <= 0 {
		dim = defaultOllamaDim
	}
	return &OllamaEmbedder{
		Endpoint:    endpoint,
		Model:       model,
		HTTPClient:  &http.Client{Timeout: 120 * time.Second},
		dim:         dim,
		queryPrefix: queryPrefixFor(model),
	}
}

// NewDefaultEmbedder builds an OllamaEmbedder from the semantic section of
// the config file csl resolves. A missing or unreadable config yields the package defaults,
// so every caller degrades the same way.
func NewDefaultEmbedder() *OllamaEmbedder {
	cfg, err := config.Load()
	if err != nil {
		return NewOllamaEmbedder("", "", 0)
	}
	return NewOllamaEmbedder(cfg.Semantic.OllamaURL, cfg.Semantic.EmbedModel, cfg.Semantic.Dim)
}

type ollamaEmbedRequest struct {
	Model     string         `json:"model"`
	Input     []string       `json:"input"`
	KeepAlive string         `json:"keep_alive,omitempty"`
	Options   map[string]any `json:"options,omitempty"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed returns one vector per input text from the Ollama /api/embed endpoint.
func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	return o.embed(ctx, texts, ollamaKeepAlive)
}

// EmbedQuery embeds a single search query, prefixed with the retrieval
// instruction when the model is instruction-tuned. Documents go through
// Embed bare either way.
func (o *OllamaEmbedder) EmbedQuery(ctx context.Context, query string) ([]float32, error) {
	vecs, err := o.embed(ctx, []string{o.queryPrefix + query}, ollamaKeepAlive)
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("ollama returned no vector for query")
	}
	return vecs[0], nil
}

// Unload asks Ollama to evict the model immediately instead of keeping it
// resident for the keep-alive window. Called after bulk indexing; best-effort.
func (o *OllamaEmbedder) Unload(ctx context.Context) error {
	_, err := o.embed(ctx, []string{"unload"}, "0")
	return err
}

// embed posts one /api/embed request with the given keep-alive.
func (o *OllamaEmbedder) embed(
	ctx context.Context,
	texts []string,
	keepAlive string,
) ([][]float32, error) {
	body, err := json.Marshal(ollamaEmbedRequest{
		Model:     o.Model,
		Input:     texts,
		KeepAlive: keepAlive,
		Options:   map[string]any{"num_ctx": ollamaNumCtx, "num_batch": ollamaNumCtx},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal ollama request: %w", err)
	}

	url := o.Endpoint + "/api/embed"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := o.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call ollama %s (is ollama running?): %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var msg bytes.Buffer
		_, _ = msg.ReadFrom(resp.Body)
		return nil, fmt.Errorf(
			"ollama %s returned status %d: %s (is model %q pulled?)",
			url, resp.StatusCode, bytes.TrimSpace(msg.Bytes()), o.Model,
		)
	}

	var decoded ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	return decoded.Embeddings, nil
}

// Dim returns the configured embedding dimensionality.
func (o *OllamaEmbedder) Dim() int { return o.dim }

// Close satisfies callers that manage the embedder's lifecycle. It does NOT
// unload the model — interactive paths rely on the keep-alive window staying
// warm across queries; bulk indexing calls Unload explicitly.
func (o *OllamaEmbedder) Close() error { return nil }

// ollamaTagsResponse is the subset of GET /api/tags we care about.
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// CheckModel verifies the Ollama server is reachable and the configured model
// is pulled. Returns a nil error when ready; otherwise an error that tells the
// user what to fix (start ollama / ollama pull).
func (o *OllamaEmbedder) CheckModel(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, o.Endpoint+"/api/tags", nil)
	if err != nil {
		return fmt.Errorf("build ollama request: %w", err)
	}
	client := o.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable at %s (is ollama running?): %w", o.Endpoint, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama %s/api/tags returned status %d", o.Endpoint, resp.StatusCode)
	}
	var tags ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return fmt.Errorf("decode ollama tags: %w", err)
	}
	for _, m := range tags.Models {
		if m.Name == o.Model || m.Name == o.Model+":latest" {
			return nil
		}
	}
	return fmt.Errorf("model %q not found in ollama — run: ollama pull %s", o.Model, o.Model)
}

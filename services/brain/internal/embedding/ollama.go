// Package embedding talks to a local Ollama instance to convert markdown
// passages into dense vectors. The Client is intentionally a small typed
// wrapper around Ollama's HTTP API so we can swap implementations in tests
// (httptest.Server) and in future (a different model server) without
// changing call sites.
//
// All vectors returned by EmbedFunc are L2-normalized — chromem-go expects
// unit vectors for its cosine-similarity index.
package embedding

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client embeds text via a local Ollama HTTP API.
type Client struct {
	BaseURL string
	Model   string
	HTTP    *http.Client
}

// New returns a Client with sensible HTTP defaults.
func New(baseURL, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Model:   model,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// embedRequest is the JSON shape posted to /api/embeddings. Ollama also
// supports /api/embed (batched), but /api/embeddings is the stable single-input
// endpoint and is sufficient for our chunk-at-a-time ingestion.
type embedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

type embedResponse struct {
	Embedding []float32 `json:"embedding"`
}

// Embed returns a single L2-normalized embedding vector for the given text.
// Returns an error if Ollama is unreachable, the model is not pulled, or the
// response is empty.
func (c *Client) Embed(ctx context.Context, text string) ([]float32, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("embed: text is empty")
	}
	body, err := json.Marshal(embedRequest{Model: c.Model, Prompt: text})
	if err != nil {
		return nil, fmt.Errorf("embed: marshal request: %w", err)
	}
	endpoint, err := url.JoinPath(c.BaseURL, "/api/embeddings")
	if err != nil {
		return nil, fmt.Errorf("embed: build url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("embed: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embed: ollama unreachable at %s: %w", c.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		var msg bytes.Buffer
		_, _ = msg.ReadFrom(resp.Body)
		return nil, fmt.Errorf("embed: ollama %s: %s", resp.Status, strings.TrimSpace(msg.String()))
	}

	var out embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("embed: decode response: %w", err)
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("embed: empty embedding returned (is model %q pulled?)", c.Model)
	}
	return Normalize(out.Embedding), nil
}

// EmbedFunc returns a chromem-go-compatible EmbeddingFunc bound to this client.
// The returned function captures the client and must not outlive the client's
// HTTP transport.
func (c *Client) EmbedFunc() func(ctx context.Context, text string) ([]float32, error) {
	return func(ctx context.Context, text string) ([]float32, error) {
		return c.Embed(ctx, text)
	}
}

// Health probes /api/tags. Returns nil if Ollama responds; an error otherwise.
// Used at server startup to fail fast (or fall back to "semantic disabled")
// when the user's Ollama isn't running.
func (c *Client) Health(ctx context.Context) error {
	endpoint, err := url.JoinPath(c.BaseURL, "/api/tags")
	if err != nil {
		return fmt.Errorf("health: build url: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("health: new request: %w", err)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("health: ollama unreachable at %s: %w", c.BaseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health: ollama returned %s", resp.Status)
	}
	return nil
}

// Normalize returns v scaled to unit L2 norm. Returns the input unchanged
// when it is already (numerically) unit length or all-zero, so we never
// produce NaN.
func Normalize(v []float32) []float32 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum == 0 {
		return v
	}
	norm := math.Sqrt(sum)
	if math.Abs(norm-1.0) < 1e-6 {
		return v
	}
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = float32(float64(x) / norm)
	}
	return out
}

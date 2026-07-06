package semantic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	// defaultOllamaEndpoint is the local Ollama server address.
	defaultOllamaEndpoint = "http://localhost:11434"
	// defaultOllamaModel is the embedding model requested from Ollama.
	defaultOllamaModel = "nomic-embed-text"
	// defaultOllamaDim is the embedding dimensionality of the default model.
	defaultOllamaDim = 768
)

// OllamaEmbedder embeds text via a local Ollama server's /api/embed endpoint.
// It is the documented fallback for hosts without the hugot model; it is not
// wired into the index path yet.
type OllamaEmbedder struct {
	Endpoint   string
	Model      string
	HTTPClient *http.Client
	dim        int
}

// NewOllamaEmbedder returns an OllamaEmbedder with sane defaults. Empty endpoint
// or model fall back to localhost:11434 and nomic-embed-text; dim <= 0 falls
// back to 768.
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
		Endpoint:   endpoint,
		Model:      model,
		HTTPClient: &http.Client{Timeout: 60 * time.Second},
		dim:        dim,
	}
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed returns one vector per input text from the Ollama /api/embed endpoint.
func (o *OllamaEmbedder) Embed(ctx context.Context, texts []string) ([][]float32, error) {
	if len(texts) == 0 {
		return nil, nil
	}

	body, err := json.Marshal(ollamaEmbedRequest{Model: o.Model, Input: texts})
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
		return nil, fmt.Errorf("call ollama %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama %s returned status %d", url, resp.StatusCode)
	}

	var decoded ollamaEmbedResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("decode ollama response: %w", err)
	}
	return decoded.Embeddings, nil
}

// Dim returns the configured embedding dimensionality.
func (o *OllamaEmbedder) Dim() int { return o.dim }

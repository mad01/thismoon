package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	openRouterName           = "openrouter"
	openRouterDefaultModel   = "anthropic/claude-haiku-4.5"
	openRouterDefaultBaseURL = "https://openrouter.ai/api"
	openRouterTimeout        = 60 * time.Second
)

// errRedirectRefused stops the HTTP client before it can re-send the request
// (and its Authorization header) to a redirect target.
var errRedirectRefused = errors.New("backend: HTTP redirect refused")

// OpenRouterConfig configures an OpenRouter backend. Zero-value Model,
// BaseURL, and Client fall back to the provider defaults.
type OpenRouterConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

// OpenRouter completes against the OpenRouter chat-completions API
// (an OpenAI-compatible surface routing to anthropic/claude-haiku-4.5
// by default).
type OpenRouter struct {
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// NewOpenRouter returns an OpenRouter backend. It fails when APIKey is empty;
// everything else defaults.
func NewOpenRouter(cfg OpenRouterConfig) (*OpenRouter, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("backend: OPENROUTER_API_KEY is not set")
	}
	if cfg.Model == "" {
		cfg.Model = openRouterDefaultModel
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = openRouterDefaultBaseURL
	}
	if cfg.Client == nil {
		cfg.Client = &http.Client{
			Timeout: openRouterTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirectRefused
			},
		}
	}
	return &OpenRouter{
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		client:  cfg.Client,
	}, nil
}

func newOpenRouterFromEnv(model string) (*OpenRouter, error) {
	if model == "" {
		model = os.Getenv("HUMANIZER_MODEL")
	}
	return NewOpenRouter(OpenRouterConfig{
		APIKey: os.Getenv("OPENROUTER_API_KEY"),
		Model:  model,
	})
}

// Name implements Backend.
func (o *OpenRouter) Name() string { return openRouterName }

// Model implements Backend.
func (o *OpenRouter) Model() string { return o.model }

type chatRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete implements Backend with one chat-completions call at temperature 0.
func (o *OpenRouter) Complete(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: o.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("backend: marshal request: %w", err)
	}
	endpoint := o.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("backend: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("backend: openrouter call: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("backend: read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("backend: openrouter HTTP %d: %s", resp.StatusCode, snippet(data))
	}
	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("backend: decode response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("backend: openrouter error: %s", out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", errors.New("backend: openrouter returned no content")
	}
	return out.Choices[0].Message.Content, nil
}

// snippet trims an error body to something loggable.
func snippet(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const chatTimeout = 60 * time.Second

// errRedirectRefused stops the HTTP client before it can re-send the request
// (and its Authorization header) to a redirect target.
var errRedirectRefused = errors.New("backend: HTTP redirect refused")

// ChatConfig configures an OpenAI-compatible chat-completions backend. The
// provider constructors (NewOpenRouter, NewLiteLLM) fill the defaults each
// provider needs; a zero-value Client gets the shared timeout and redirect
// refusal.
type ChatConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client
}

// ChatClient completes against an OpenAI-compatible chat-completions
// endpoint (POST {base}/v1/chat/completions). OpenRouter and the LiteLLM
// proxy both serve that surface, so they share this one client and differ
// only in name, env, and defaults.
type ChatClient struct {
	name    string
	apiKey  string
	model   string
	baseURL string
	client  *http.Client
}

// newChatClient builds the client behind every provider constructor. It
// trusts cfg: the provider constructor has already validated and defaulted
// it. A trailing /v1 on BaseURL is trimmed because the endpoint path adds
// it, and pasting a "…/v1" base is the common mistake.
func newChatClient(name string, cfg ChatConfig) *ChatClient {
	if cfg.Client == nil {
		cfg.Client = &http.Client{
			Timeout: chatTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errRedirectRefused
			},
		}
	}
	base := strings.TrimRight(cfg.BaseURL, "/")
	base = strings.TrimSuffix(base, "/v1")
	return &ChatClient{
		name:    name,
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		baseURL: base,
		client:  cfg.Client,
	}
}

// Name implements Backend.
func (c *ChatClient) Name() string { return c.name }

// Model implements Backend.
func (c *ChatClient) Model() string { return c.model }

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

// Complete implements Backend with one chat-completions call at temperature
// 0. The Authorization header is sent only when a key is configured, so a
// proxy without auth (a local LiteLLM) works too.
func (c *ChatClient) Complete(ctx context.Context, system, user string) (string, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature: 0,
	})
	if err != nil {
		return "", fmt.Errorf("backend: marshal request: %w", err)
	}
	endpoint := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("backend: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("backend: %s call: %w", c.name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("backend: read response: %w", err)
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("backend: %s HTTP %d: %s", c.name, resp.StatusCode, snippet(data))
	}
	var out chatResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return "", fmt.Errorf("backend: decode response: %w", err)
	}
	if out.Error != nil {
		return "", fmt.Errorf("backend: %s error: %s", c.name, out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("backend: %s returned no content", c.name)
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

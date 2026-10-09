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
// refusal. The judge sets only the first four fields. The rest exist for the
// judge eval harness, which needs to reproduce older request shapes and read
// what the provider reported; production leaves them at their zero values.
type ChatConfig struct {
	APIKey  string
	Model   string
	BaseURL string
	Client  *http.Client

	// Temperature, when set, is sent as the sampling temperature. The judge
	// leaves it nil: the Claude 5.5 generation rejects any value other than
	// the default with a 400, so pinning 0 would break the default model
	// (MAD-391). The eval harness sets it to run the earlier temperature-0
	// baseline on models that still accept one.
	Temperature *float64
	// ReasoningEffort, when non-empty, is sent as reasoning_effort, the
	// OpenAI-compatible knob OpenRouter and LiteLLM forward to the model's
	// effort setting. Empty keeps the provider default.
	ReasoningEffort string
	// Observe, when set, receives one CallInfo per HTTP exchange that got a
	// response, whatever its status. The eval harness reads the served
	// model, token usage, and latency from it.
	Observe func(CallInfo)
}

// CallInfo is what the provider reported about one chat-completions
// exchange: the model that served it, the token usage, the HTTP status, and
// the latency of the HTTP exchange alone (request sent to body read).
type CallInfo struct {
	Model            string
	PromptTokens     int
	CompletionTokens int
	// ReasoningTokens is the thinking share of CompletionTokens when the
	// provider breaks it out (OpenRouter does); zero otherwise.
	ReasoningTokens int
	Status          int
	Latency         time.Duration
}

// ChatClient completes against an OpenAI-compatible chat-completions
// endpoint (POST {base}/v1/chat/completions). OpenRouter and the LiteLLM
// proxy both serve that surface, so they share this one client and differ
// only in name, env, and defaults.
type ChatClient struct {
	name            string
	apiKey          string
	model           string
	baseURL         string
	client          *http.Client
	temperature     *float64
	reasoningEffort string
	observe         func(CallInfo)
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
		name:            name,
		apiKey:          cfg.APIKey,
		model:           cfg.Model,
		baseURL:         base,
		client:          cfg.Client,
		temperature:     cfg.Temperature,
		reasoningEffort: cfg.ReasoningEffort,
		observe:         cfg.Observe,
	}
}

// Name implements Backend.
func (c *ChatClient) Name() string { return c.name }

// Model implements Backend.
func (c *ChatClient) Model() string { return c.model }

// chatRequest is the wire shape of one completion. Temperature and
// ReasoningEffort are omitted unless configured, so the default request
// carries only model and messages and works on every Claude generation.
type chatRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
	Temperature     *float64      `json:"temperature,omitempty"`
	ReasoningEffort string        `json:"reasoning_effort,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		Details          struct {
			ReasoningTokens int `json:"reasoning_tokens"`
		} `json:"completion_tokens_details"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// Complete implements Backend with one chat-completions call. The
// Authorization header is sent only when a key is configured, so a proxy
// without auth (a local LiteLLM) works too.
func (c *ChatClient) Complete(ctx context.Context, system, user string) (string, error) {
	req, err := c.newRequest(ctx, system, user)
	if err != nil {
		return "", err
	}
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("backend: %s call: %w", c.name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("backend: read response: %w", err)
	}
	latency := time.Since(start)
	var out chatResponse
	decodeErr := json.Unmarshal(data, &out)
	c.report(out, resp.StatusCode, latency)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("backend: %s HTTP %d: %s", c.name, resp.StatusCode, snippet(data))
	}
	if decodeErr != nil {
		return "", fmt.Errorf("backend: decode response: %w", decodeErr)
	}
	if out.Error != nil {
		return "", fmt.Errorf("backend: %s error: %s", c.name, out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("backend: %s returned no content", c.name)
	}
	return out.Choices[0].Message.Content, nil
}

// newRequest marshals the completion body and builds the POST.
func (c *ChatClient) newRequest(ctx context.Context, system, user string) (*http.Request, error) {
	body, err := json.Marshal(chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		Temperature:     c.temperature,
		ReasoningEffort: c.reasoningEffort,
	})
	if err != nil {
		return nil, fmt.Errorf("backend: marshal request: %w", err)
	}
	endpoint := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("backend: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	return req, nil
}

// report hands the provider's view of one exchange to the observer, if any.
func (c *ChatClient) report(out chatResponse, status int, latency time.Duration) {
	if c.observe == nil {
		return
	}
	c.observe(CallInfo{
		Model:            out.Model,
		PromptTokens:     out.Usage.PromptTokens,
		CompletionTokens: out.Usage.CompletionTokens,
		ReasoningTokens:  out.Usage.Details.ReasoningTokens,
		Status:           status,
		Latency:          latency,
	})
}

// snippet trims an error body to something loggable.
func snippet(data []byte) string {
	s := strings.TrimSpace(string(data))
	if len(s) > 300 {
		s = s[:300] + "..."
	}
	return s
}

package llm

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

const (
	// chatTimeout is the HTTP client's safety ceiling on one exchange, a
	// guard against a connection that never answers, not the per-call
	// bound. Callers bound a call with their context: the csl_ask eval gives
	// each attempt 180 s, and csl_ask gives a call its callTimeout. A full
	// corpus through a slow route can take minutes, so the ceiling sits
	// above every caller's own bound.
	chatTimeout  = 10 * time.Minute
	maxBodyBytes = 4 << 20

	openRouterDefaultBaseURL = "https://openrouter.ai/api"
)

// errRedirectRefused stops the HTTP client before it can re-send the
// request, and its bearer key, to a redirect target.
var errRedirectRefused = errors.New("llm: HTTP redirect refused")

// ChatConfig configures an OpenAI-compatible chat-completions backend. The
// provider constructors (NewLiteLLM, NewOpenRouter) fill the defaults each
// provider needs; a nil Client gets the shared timeout and redirect
// refusal.
type ChatConfig struct {
	APIKey string
	// Model is the canonical Anthropic id or a provider-specific one; empty
	// is DefaultModel. NewOpenRouter translates a canonical id.
	Model   string
	BaseURL string
	Client  *http.Client
	// Effort, when non-empty, is sent as reasoning_effort, the
	// OpenAI-compatible knob OpenRouter and LiteLLM forward to the model's
	// effort setting.
	Effort string
	// Temperature, when set, is sent as the sampling temperature. The
	// Claude 5.5 generation rejects any value other than the default with
	// a 400, so production leaves it nil; an eval that reproduces an older
	// request shape sets it.
	Temperature *float64
	// Observe, when set, receives one CallInfo per HTTP exchange that got
	// a response, whatever its status.
	Observe func(CallInfo)
}

// ChatClient completes against an OpenAI-compatible chat-completions
// endpoint (POST {base}/v1/chat/completions). OpenRouter and a LiteLLM
// proxy both serve that surface, so they share this one client and differ
// only in name, env, and the id they know a model by.
type ChatClient struct {
	name        string
	apiKey      string
	model       string
	baseURL     string
	client      *http.Client
	effort      string
	temperature *float64
	observe     func(CallInfo)
}

// NewLiteLLM returns a backend for a LiteLLM proxy (self-hosted or a
// company gateway). BaseURL is required, without the /v1 suffix; APIKey is
// optional because a proxy may run without a master key. The model id goes
// to the proxy as given: a proxy serves whatever aliases its model_list
// declares, and the canonical id is the alias to declare.
func NewLiteLLM(cfg ChatConfig) (*ChatClient, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("llm: LITELLM_BASE_URL is not set")
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	return newChatClient(LiteLLM, cfg), nil
}

// NewOpenRouter returns a backend for the OpenRouter chat-completions API.
// It fails when APIKey is empty; BaseURL defaults, and a canonical model id
// is translated to the id OpenRouter serves it under.
func NewOpenRouter(cfg ChatConfig) (*ChatClient, error) {
	if cfg.APIKey == "" {
		return nil, errors.New("llm: OPENROUTER_API_KEY is not set")
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel
	}
	cfg.Model = OpenRouterID(cfg.Model)
	if cfg.BaseURL == "" {
		cfg.BaseURL = openRouterDefaultBaseURL
	}
	return newChatClient(OpenRouter, cfg), nil
}

// newChatClient builds the client behind both provider constructors. It
// trusts cfg: the constructor has validated and defaulted it. A trailing
// /v1 on BaseURL is trimmed because the endpoint path adds it, and pasting
// a "…/v1" base is the common mistake.
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
		name:        name,
		apiKey:      cfg.APIKey,
		model:       cfg.Model,
		baseURL:     base,
		client:      cfg.Client,
		effort:      cfg.Effort,
		temperature: cfg.Temperature,
		observe:     cfg.Observe,
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
// without auth (a local LiteLLM) works too. The observer sees every
// exchange that got a response, error or not, so a caller can classify
// the failure.
func (c *ChatClient) Complete(ctx context.Context, system, user string) (string, error) {
	req, err := c.newRequest(ctx, system, user)
	if err != nil {
		return "", err
	}
	start := time.Now()
	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("llm: %s call: %w", c.name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return "", fmt.Errorf("llm: read response: %w", err)
	}
	latency := time.Since(start)
	var out chatResponse
	decodeErr := json.Unmarshal(data, &out)
	c.report(out, resp.StatusCode, latency)
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf(
			"llm: %s HTTP %d: %s",
			c.name,
			resp.StatusCode,
			truncate(string(data), 300),
		)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("llm: decode response: %w", decodeErr)
	}
	if out.Error != nil {
		return "", fmt.Errorf("llm: %s error: %s", c.name, out.Error.Message)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("llm: %s returned no content", c.name)
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
		ReasoningEffort: c.effort,
	})
	if err != nil {
		return nil, fmt.Errorf("llm: marshal request: %w", err)
	}
	endpoint := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("llm: build request: %w", err)
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

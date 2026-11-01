package main

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
	openRouterBase = "https://openrouter.ai/api"
	// callTimeout bounds one completion. A 2000-line corpus is about 30k
	// tokens in, and a slow provider route can take a minute on it.
	callTimeout  = 180 * time.Second
	maxBodyBytes = 4 << 20
)

// errRedirectRefused stops the client before it re-sends the request, and
// its bearer key, to a redirect target.
var errRedirectRefused = errors.New("http redirect refused")

var httpClient = &http.Client{
	Timeout: callTimeout,
	CheckRedirect: func(*http.Request, []*http.Request) error {
		return errRedirectRefused
	},
}

// CallInfo is what the backend reported about one exchange.
type CallInfo struct {
	Model            string        `json:"model"`
	PromptTokens     int           `json:"prompt_tokens"`
	CompletionTokens int           `json:"completion_tokens"`
	ReasoningTokens  int           `json:"reasoning_tokens"`
	Status           int           `json:"status"`
	Latency          time.Duration `json:"latency"`
	// CostUSD is set by a transport that prices the call itself (the
	// claude CLI reports total_cost_usd); zero means the price table applies.
	CostUSD float64 `json:"cost_usd,omitempty"`
}

// openRouterIDs maps the canonical Anthropic model ids the arms use to the
// ids OpenRouter serves them under.
var openRouterIDs = map[string]string{
	"claude-haiku-5-5":  "anthropic/claude-haiku-5.5",
	"claude-sonnet-5-5": "anthropic/claude-sonnet-5.5",
	"claude-opus-5-5":   "anthropic/claude-opus-5.5",
}

// chatClient completes against an OpenAI-compatible chat-completions
// endpoint (POST {base}/v1/chat/completions). OpenRouter and a LiteLLM
// proxy both serve that surface and differ in base URL, key, and the model
// id they know the Anthropic models by.
type chatClient struct {
	id      string
	baseURL string
	apiKey  string
	modelID func(canonical string) (string, error)
}

func newOpenRouter(key string) *chatClient {
	return &chatClient{
		id: viaOpenRouter, baseURL: openRouterBase, apiKey: key,
		modelID: func(canonical string) (string, error) {
			id, ok := openRouterIDs[canonical]
			if !ok {
				return "", fmt.Errorf("no OpenRouter id for %s", canonical)
			}
			return id, nil
		},
	}
}

// newLiteLLM targets a LiteLLM proxy by its base URL. The canonical id is
// sent as the model alias, which is what humanizer's judge defaults to on
// the same proxies; a trailing /v1 on the base is trimmed because the
// endpoint path adds it.
func newLiteLLM(base, key string) *chatClient {
	base = strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
	return &chatClient{
		id: viaLiteLLM, baseURL: base, apiKey: key,
		modelID: func(canonical string) (string, error) { return canonical, nil },
	}
}

func (c *chatClient) name() string { return c.id }

type chatRequest struct {
	Model           string        `json:"model"`
	Messages        []chatMessage `json:"messages"`
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

// complete runs one chat completion. No temperature goes on the wire: the
// Claude 5.5 generation rejects any value but the default. reasoning_effort
// is sent only when the arm sets it. CallInfo is filled whenever a response
// arrived, error or not, so the caller can classify the failure.
func (c *chatClient) complete(
	ctx context.Context, model, effort, system, user string,
) (string, CallInfo, error) {
	id, err := c.modelID(model)
	if err != nil {
		return "", CallInfo{}, err
	}
	body, err := json.Marshal(chatRequest{
		Model: id,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		ReasoningEffort: effort,
	})
	if err != nil {
		return "", CallInfo{}, err
	}
	endpoint := c.baseURL + "/v1/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", CallInfo{}, err
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	start := time.Now()
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", CallInfo{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	info := CallInfo{Status: resp.StatusCode, Latency: time.Since(start)}
	if err != nil {
		return "", info, fmt.Errorf("read response: %w", err)
	}
	var out chatResponse
	if jerr := json.Unmarshal(data, &out); jerr != nil {
		if resp.StatusCode != http.StatusOK {
			return "", info, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(data), 200))
		}
		return "", info, fmt.Errorf("decode response: %w", jerr)
	}
	info.Model = out.Model
	info.PromptTokens = out.Usage.PromptTokens
	info.CompletionTokens = out.Usage.CompletionTokens
	info.ReasoningTokens = out.Usage.Details.ReasoningTokens
	if out.Error != nil {
		return "", info, fmt.Errorf("http %d: %s", resp.StatusCode, out.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", info, fmt.Errorf("http %d", resp.StatusCode)
	}
	if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
		return "", info, errors.New("no content in response")
	}
	return out.Choices[0].Message.Content, info, nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

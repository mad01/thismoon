package rewrite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var loopbackHosts = map[string]struct{}{
	"localhost": {}, "127.0.0.1": {}, "::1": {},
}

// errRedirectRefused stops the HTTP client before it can re-send the request
// (and its Authorization header) to a redirect target.
var errRedirectRefused = errors.New("rewrite: HTTP redirect refused")

// CheckRemote enforces the endpoint allowlist. It returns a non-empty warning
// when a non-loopback host is explicitly allowed, and an error when the
// endpoint is denied (non-loopback without opt-in, or a non-http(s) scheme).
func CheckRemote(baseURL string, allowRemote bool) (warning string, err error) {
	u, perr := url.Parse(baseURL)
	if perr != nil {
		return "", fmt.Errorf("rewrite: invalid base URL %q: %w", baseURL, perr)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("rewrite: base URL must be http(s), got scheme %q: %s", u.Scheme, baseURL)
	}
	host := u.Hostname()
	if _, ok := loopbackHosts[host]; ok {
		return "", nil
	}
	if !allowRemote {
		return "", fmt.Errorf(
			"rewrite: base URL host is not loopback (%q); refusing to send content off-machine. "+
				"Set WATERMARKS_REWRITE_ALLOW_REMOTE=1 or pass --allow-remote to override",
			host,
		)
	}
	return fmt.Sprintf("warning: rewrite base URL host is %q (not localhost); content will leave this machine", host), nil
}

// httpJSON POSTs a JSON payload and decodes a JSON response. Redirects are
// refused so the Authorization header is never forwarded to another host.
func httpJSON(endpoint string, payload any, headers map[string]string, timeout time.Duration) (map[string]any, error) {
	u, err := url.Parse(endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("rewrite: refusing non-http(s) endpoint: %s", endpoint)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("rewrite: marshal payload: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errRedirectRefused
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("rewrite: endpoint returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("rewrite: decode response: %w", err)
	}
	return out, nil
}

func callOllama(baseURL, model, prompt string, timeout time.Duration, temperature float64) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/chat"
	data, err := httpJSON(endpoint, map[string]any{
		"model":    model,
		"stream":   false,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
		"options":  map[string]any{"temperature": temperature},
	}, nil, timeout)
	if err != nil {
		return "", err
	}
	msg, _ := data["message"].(map[string]any)
	content, _ := msg["content"].(string)
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("rewrite: ollama empty response")
	}
	return strings.TrimSpace(content), nil
}

func callOpenAICompatible(baseURL, model, prompt, apiKey string, timeout time.Duration, temperature float64) (string, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/chat/completions"
	headers := map[string]string{}
	if apiKey != "" {
		headers["Authorization"] = "Bearer " + apiKey
	}
	data, err := httpJSON(endpoint, map[string]any{
		"model":       model,
		"messages":    []map[string]string{{"role": "user", "content": prompt}},
		"temperature": temperature,
	}, headers, timeout)
	if err != nil {
		return "", err
	}
	choices, _ := data["choices"].([]any)
	if len(choices) == 0 {
		return "", fmt.Errorf("rewrite: openai-compatible empty choices")
	}
	first, _ := choices[0].(map[string]any)
	msg, _ := first["message"].(map[string]any)
	content, _ := msg["content"].(string)
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("rewrite: openai-compatible empty content")
	}
	return strings.TrimSpace(content), nil
}

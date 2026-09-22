// Package sharedclient talks to a shared present instance from a local one:
// push a page, replace it, remove it, and confirm the author key. The CLI,
// the local serve's share endpoint, and the local MCP's present_share tool
// all go through it, so the wire shape lives in one place.
package sharedclient

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

// Errors mapped from the shared instance's status codes.
var (
	ErrUnauthorized = errors.New("shared instance rejected the author key (401)")
	ErrForbidden    = errors.New("page belongs to another author on the shared instance (403)")
	ErrNotFound     = errors.New("page not found on the shared instance (404)")
)

// requestTimeout bounds one call; a shared instance that hangs must not
// hang the share button or the CLI with it.
const requestTimeout = 10 * time.Second

// Client is a shared instance plus the author key writes are signed with.
type Client struct {
	BaseURL string
	Key     string
	HTTP    *http.Client
}

// New returns a Client for baseURL (no trailing slash needed) using key.
func New(baseURL, key string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Key:     key,
		HTTP:    &http.Client{Timeout: requestTimeout},
	}
}

// Reference is a page reference on the wire.
type Reference struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Bundle is the page a client pushes: the rendered artifacts plus the
// canonical sources, exactly what the local store holds for it.
type Bundle struct {
	Title       string          `json:"title"`
	Content     string          `json:"content"`
	Graph       string          `json:"graph"`
	References  []Reference     `json:"references"`
	Doc         json.RawMessage `json:"doc,omitempty"`
	GraphSource json.RawMessage `json:"graph_source,omitempty"`
	Ephemeral   bool            `json:"ephemeral"`
}

// Result is where a pushed page lives on the shared instance.
type Result struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Version   int        `json:"version"`
	Ephemeral bool       `json:"ephemeral"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Create pushes a new page and returns where it landed.
func (c *Client) Create(ctx context.Context, b Bundle) (Result, error) {
	var out Result
	err := c.do(ctx, http.MethodPost, "/api/pages", b, &out)
	return out, err
}

// Replace overwrites the shared page id with b. Only the author's key may.
func (c *Client) Replace(ctx context.Context, id string, b Bundle) (Result, error) {
	var out Result
	err := c.do(ctx, http.MethodPut, "/api/p/"+id, b, &out)
	return out, err
}

// Delete removes the shared page id. Only the author's key may.
func (c *Client) Delete(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, "/p/"+id, nil, nil)
}

// WhoAmI returns the author hash the instance derives from the key: the
// cheapest proof that the instance is reachable and the key arrives intact.
func (c *Client) WhoAmI(ctx context.Context) (string, error) {
	var out struct {
		Author string `json:"author"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/whoami", nil, &out); err != nil {
		return "", err
	}
	return out.Author, nil
}

// do sends one request with the bearer key and decodes a JSON reply into
// out when out is non-nil.
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var payload io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		payload = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, payload)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	req.Header.Set("Accept", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, c.BaseURL+path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := statusError(resp); err != nil {
		return err
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode reply: %w", err)
	}
	return nil
}

// statusError maps a failed response onto the package errors, carrying the
// instance's own message for everything else.
func statusError(resp *http.Response) error {
	switch {
	case resp.StatusCode < 300:
		return nil
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode == http.StatusForbidden:
		return ErrForbidden
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("shared instance returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
}

// Package client is the HTTP client for a running `events serve`. The MCP server
// and the CLI commands use it, so neither writes the JSONL store directly — serve
// stays the single writer.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/events"
)

// Event mirrors the JSON an event record carries over the API.
type Event struct {
	ID        string            `json:"id"`
	Time      time.Time         `json:"time"`
	Source    string            `json:"source"`
	Component string            `json:"component,omitempty"`
	Level     string            `json:"level"`
	Title     string            `json:"title"`
	Message   string            `json:"message,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	Data      json.RawMessage   `json:"data,omitempty"`
}

// SourceCount is one source and its event count.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// EmitBody is the POST /api/events payload. The server stamps id and time.
type EmitBody struct {
	Source    string            `json:"source"`
	Component string            `json:"component,omitempty"`
	Level     string            `json:"level,omitempty"`
	Title     string            `json:"title"`
	Message   string            `json:"message,omitempty"`
	Tags      map[string]string `json:"tags,omitempty"`
	Data      json.RawMessage   `json:"data,omitempty"`
}

// QueryFilter is the GET /api/events query, mapped to URL params.
type QueryFilter struct {
	Source string
	Level  string
	Q      string
	Since  string
	Limit  int
}

// Client talks to a running `events serve` over its localhost HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the serve instance at baseURL.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 5 * time.Second}}
}

// Emit records an event and returns its generated id.
func (c *Client) Emit(b EmitBody) (string, error) {
	var out struct {
		ID string `json:"id"`
	}
	if err := c.do(http.MethodPost, "/api/events", b, &out); err != nil {
		return "", err
	}
	return out.ID, nil
}

// Query returns events newest-first matching the filter.
func (c *Client) Query(f QueryFilter) ([]Event, error) {
	q := url.Values{}
	if f.Source != "" {
		q.Set("source", f.Source)
	}
	if f.Level != "" {
		q.Set("level", f.Level)
	}
	if f.Q != "" {
		q.Set("q", f.Q)
	}
	if f.Since != "" {
		q.Set("since", f.Since)
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	path := "/api/events"
	if enc := q.Encode(); enc != "" {
		path += "?" + enc
	}
	var out []Event
	return out, c.do(http.MethodGet, path, nil, &out)
}

// Purge drops events from one source (all of them, or only those with ID <=
// before) and returns how many were removed.
func (c *Client) Purge(source, before string) (int, error) {
	q := url.Values{}
	q.Set("source", source)
	if before != "" {
		q.Set("before", before)
	}
	var out struct {
		Purged int `json:"purged"`
	}
	if err := c.do(http.MethodDelete, "/api/events?"+q.Encode(), nil, &out); err != nil {
		return 0, err
	}
	return out.Purged, nil
}

// Sources lists each source with its event count.
func (c *Client) Sources() ([]SourceCount, error) {
	var out []SourceCount
	return out, c.do(http.MethodGet, "/api/sources", nil, &out)
}

// do issues a request and decodes a JSON response. A transport error (serve not
// running) and an API error ({"error": ...}) both come back as descriptive
// errors callers can relay to the user.
func (c *Client) do(method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		// The one transport chokepoint every CLI command and MCP tool goes
		// through; Hint points the reader at the operating doc from here.
		return agentdoc.Hint(fmt.Errorf(
			"events serve not reachable at %s — is the t-man agent running? (t-man status events): %w",
			c.baseURL,
			err,
		), events.Facts())
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(res.Body)
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Error != "" {
			// The serve API already package-prefixes its errors (e.g.
			// "event: source is required"); surface them as-is.
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("events: serve returned %d", res.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Package client is the HTTP client for a running `prs serve`. Both the MCP
// server and the CLI commands use it, so neither talks to GitHub or touches
// the cache directly — serve stays the single writer.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	prs "github.com/mad01/thismoon/services/prs"
	"github.com/mad01/thismoon/services/prs/internal/store"
)

// ListResult mirrors the GET /api/prs response: the filtered PRs, the
// dropdown facets over the whole cache, and the cache status.
type ListResult struct {
	PRs    []store.PR `json:"prs"`
	Facets struct {
		Repos   []string `json:"repos"`
		Authors []string `json:"authors"`
	} `json:"facets"`
	Status store.Status `json:"status"`
}

// ServiceStatus mirrors the GET /api/status response: the cache status plus
// the config the poller runs with.
type ServiceStatus struct {
	store.Status
	PollInterval string   `json:"poll_interval"`
	Dirs         []string `json:"dirs,omitempty"`
	HostsAllowed []string `json:"hosts_allowed,omitempty"`
	Exclude      []string `json:"exclude,omitempty"`
	ConfigPath   string   `json:"config_path"`
	ConfigLoaded bool     `json:"config_loaded"`
}

// RefreshSummary mirrors the POST /api/refresh response.
type RefreshSummary struct {
	Repos      int       `json:"repos"`
	OpenPRs    int       `json:"open_prs"`
	Errors     int       `json:"errors"`
	PolledAt   time.Time `json:"polled_at"`
	DurationMS int64     `json:"duration_ms"`
}

// ListFilter carries the GET /api/prs query params.
type ListFilter struct {
	Repo   string
	Author string
	Review string
	Sort   string
}

// Client talks to a running `prs serve` over its localhost HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
	// slow covers the refresh, which fans a full poll cycle over every
	// discovered repo; every other call reads the local cache and keeps the
	// short timeout.
	slow *http.Client
}

// New returns a Client for the serve instance at baseURL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
		slow:    &http.Client{Timeout: 5 * time.Minute},
	}
}

// List returns the PRs matching f.
func (c *Client) List(f ListFilter) (ListResult, error) {
	q := url.Values{}
	if f.Repo != "" {
		q.Set("repo", f.Repo)
	}
	if f.Author != "" {
		q.Set("author", f.Author)
	}
	if f.Review != "" {
		q.Set("review", f.Review)
	}
	if f.Sort != "" {
		q.Set("sort", f.Sort)
	}
	path := "/api/prs"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out ListResult
	return out, c.do(c.http, http.MethodGet, path, nil, &out)
}

// Status returns the service status.
func (c *Client) Status() (ServiceStatus, error) {
	var out ServiceStatus
	return out, c.do(c.http, http.MethodGet, "/api/status", nil, &out)
}

// Refresh forces one synchronous poll cycle and reports it. A cycle walks
// every discovered repo, so this rides the slow client.
func (c *Client) Refresh() (RefreshSummary, error) {
	var out RefreshSummary
	return out, c.do(c.slow, http.MethodPost, "/api/refresh", nil, &out)
}

// do issues a request and decodes a JSON response. A transport error (serve
// not running) and an API error ({"error": ...}) both come back as
// descriptive errors callers can relay to the user.
func (c *Client) do(hc *http.Client, method, path string, body, out any) error {
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
	res, err := hc.Do(req)
	if err != nil {
		// The one transport chokepoint every CLI command and MCP tool goes
		// through; Hint points the reader at the operating doc from here.
		return agentdoc.Hint(fmt.Errorf(
			"prs serve not reachable at %s — is the t-man agent running? (t-man status prs): %w",
			c.baseURL,
			err,
		), prs.Facts())
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode >= 400 {
		var apiErr struct {
			Error string `json:"error"`
		}
		raw, _ := io.ReadAll(res.Body)
		_ = json.Unmarshal(raw, &apiErr)
		if apiErr.Error != "" {
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("prs: serve returned %d", res.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

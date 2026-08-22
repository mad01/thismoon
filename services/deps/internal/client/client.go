// Package client is the HTTP client for a running `deps serve`. The CLI scan/
// check/notify commands and the MCP server use it, so neither runs a scan nor
// writes the store directly — serve stays the single writer and the only process
// that reaches the network.
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
	deps "github.com/mad01/thismoon/services/deps"
	"github.com/mad01/thismoon/services/deps/internal/api"
)

// Client talks to a running `deps serve` over its localhost HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the serve instance at baseURL. The timeout is
// generous: a full check fans out to OSV across every dependency.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 2 * time.Minute}}
}

// ScanResult summarizes a discovery-only scan.
type ScanResult struct {
	ScannedAt   time.Time      `json:"scanned_at"`
	Total       int            `json:"total"`
	ByEcosystem map[string]int `json:"by_ecosystem"`
}

// CheckResult is the outcome of a discover + OSV check.
type CheckResult struct {
	ScannedAt    time.Time        `json:"scanned_at"`
	Total        int              `json:"total"`
	FlaggedCount int              `json:"flagged_count"`
	Flagged      []api.Dependency `json:"flagged"`
}

// NotifyResult reports how many flags were delivered.
type NotifyResult struct {
	Delivered int `json:"delivered"`
}

// ResolveResult reports how many advisories were newly acknowledged.
type ResolveResult struct {
	Resolved int `json:"resolved"`
}

// Snapshot is the current persisted scan (GET /api/deps).
type Snapshot struct {
	ScannedAt    time.Time        `json:"scanned_at"`
	Total        int              `json:"total"`
	FlaggedCount int              `json:"flagged_count"`
	Deps         []api.Dependency `json:"deps"`
}

func (c *Client) Scan() (ScanResult, error) {
	var out ScanResult
	return out, c.do(http.MethodPost, "/api/scan", nil, &out)
}

func (c *Client) Check() (CheckResult, error) {
	var out CheckResult
	return out, c.do(http.MethodPost, "/api/check", nil, &out)
}

// CheckRepo rescans a single repo (by absolute path or basename).
func (c *Client) CheckRepo(repo string) (CheckResult, error) {
	var out CheckResult
	return out, c.do(http.MethodPost, "/api/check?repo="+url.QueryEscape(repo), nil, &out)
}

func (c *Client) Notify() (NotifyResult, error) {
	var out NotifyResult
	return out, c.do(http.MethodPost, "/api/notify", nil, &out)
}

// Resolve acknowledges the given advisory keys.
func (c *Client) Resolve(keys []string) (ResolveResult, error) {
	var out ResolveResult
	return out, c.do(http.MethodPost, "/api/resolve", map[string][]string{"keys": keys}, &out)
}

func (c *Client) Flagged() (CheckResult, error) {
	var out CheckResult
	return out, c.do(http.MethodGet, "/api/flagged", nil, &out)
}

func (c *Client) Deps() (Snapshot, error) {
	var out Snapshot
	return out, c.do(http.MethodGet, "/api/deps", nil, &out)
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
			"deps serve not reachable at %s — is the t-man agent running? (t-man status deps): %w",
			c.baseURL, err,
		), deps.Facts())
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
		return fmt.Errorf("deps: serve returned %d", res.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Package client is the HTTP client for a running `kof serve`. Both the MCP
// server and the CLI management commands use it, so neither writes the JSON
// store or resolves pins directly — serve stays the single writer.
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
)

// Pin mirrors a resolved evidence pin as the serve API returns it.
type Pin struct {
	RepoPath      string    `json:"repo_path"`
	Repo          string    `json:"repo,omitempty"`
	File          string    `json:"file"`
	StartLine     int       `json:"start_line"`
	EndLine       int       `json:"end_line"`
	ContentSHA256 string    `json:"content_sha256"`
	HeadCommit    string    `json:"head_commit"`
	ResolvedAt    time.Time `json:"resolved_at"`
}

// Provenance mirrors where an assertion came from.
type Provenance struct {
	Author     string    `json:"author,omitempty"`
	SessionID  string    `json:"session_id"`
	DerivedAt  time.Time `json:"derived_at"`
	CostTokens int       `json:"cost_tokens,omitempty"`
}

// Assertion mirrors the JSON the serve API returns for a single assertion.
type Assertion struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	Subject     string     `json:"subject"`
	Statement   string     `json:"statement"`
	Pins        []Pin      `json:"pins"`
	Confidence  string     `json:"confidence"`
	Provenance  Provenance `json:"provenance"`
	Status      string     `json:"status"`
	StaleReason string     `json:"stale_reason,omitempty"`
	RetractNote string     `json:"retract_note,omitempty"`
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
	Links       []string   `json:"links,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	RetractedAt *time.Time `json:"retracted_at,omitempty"`
}

// CheckReport mirrors the POST /api/check response: how many assertions were
// re-hashed and how the counts broke down, plus the affected assertions.
type CheckReport struct {
	Checked    int         `json:"checked"`
	Fresh      int         `json:"fresh"`
	Stale      int         `json:"stale"`
	Flipped    int         `json:"flipped"`
	Assertions []Assertion `json:"assertions"`
}

// PinRef is an unresolved pin request: the server resolves and hashes it.
type PinRef struct {
	RepoPath  string `json:"repo_path"`
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
}

// AssertBody is the POST /api/assertions payload. At least one pin is required;
// the server rejects an assertion it cannot ground in evidence.
type AssertBody struct {
	Kind       string   `json:"kind"`
	Subject    string   `json:"subject"`
	Statement  string   `json:"statement"`
	Confidence string   `json:"confidence"`
	SessionID  string   `json:"session_id"`
	CostTokens int      `json:"cost_tokens,omitempty"`
	Links      []string `json:"links,omitempty"`
	Pins       []PinRef `json:"pins"`
}

// Client talks to a running `kof serve` over its localhost HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
	// slow covers requests that wait on the recall model judge; every other
	// call is a local store operation and keeps the short timeout.
	slow *http.Client
}

// New returns a Client for the serve instance at baseURL.
func New(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 5 * time.Second},
		slow:    &http.Client{Timeout: 45 * time.Second},
	}
}

func (c *Client) Assert(in AssertBody) (Assertion, error) {
	var out Assertion
	return out, c.do(http.MethodPost, "/api/assertions", in, &out)
}

func (c *Client) List(subject, kind, status string) ([]Assertion, error) {
	q := url.Values{}
	if subject != "" {
		q.Set("subject", subject)
	}
	if kind != "" {
		q.Set("kind", kind)
	}
	if status != "" {
		q.Set("status", status)
	}
	path := "/api/assertions"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out struct {
		Assertions []Assertion `json:"assertions"`
	}
	return out.Assertions, c.do(http.MethodGet, path, nil, &out)
}

// Recall asks serve to rank the store against a free-form question with the
// model judge and returns the relevant assertions in rank order. The judge
// takes seconds, so this rides the slow client.
func (c *Client) Recall(question string) ([]Assertion, error) {
	var out struct {
		Assertions []Assertion `json:"assertions"`
	}
	body := map[string]string{"question": question}
	return out.Assertions, c.doWith(c.slow, http.MethodPost, "/api/recall", body, &out)
}

func (c *Client) Get(id string) (Assertion, error) {
	var out Assertion
	return out, c.do(http.MethodGet, "/api/assertions/"+id, nil, &out)
}

// Retract withdraws an assertion with a counter-evidence note; the returned
// assertion is the terminal retracted record.
func (c *Client) Retract(id, note string) (Assertion, error) {
	var out Assertion
	return out, c.do(
		http.MethodPost,
		"/api/assertions/"+id+"/retract",
		retractBody{Note: note},
		&out,
	)
}

// Check re-hashes an assertion's pins, or every non-retracted assertion when id
// is empty, and reports how many flipped between fresh and stale.
func (c *Client) Check(id string) (CheckReport, error) {
	var out CheckReport
	return out, c.do(http.MethodPost, "/api/check", checkBody{ID: id}, &out)
}

type retractBody struct {
	Note string `json:"note"`
}

// checkBody omits id when empty so the request body is `{}` — the server reads
// an absent id as "check every non-retracted assertion".
type checkBody struct {
	ID string `json:"id,omitempty"`
}

// do issues a request and decodes a JSON response. A transport error (serve not
// running) and an API error ({"error": ...}) both come back as descriptive
// errors callers can relay to the user.
func (c *Client) do(method, path string, body, out any) error {
	return c.doWith(c.http, method, path, body, out)
}

// doWith is do with an explicit http client, so slow judge-backed requests
// can outlive the default timeout without loosening it for everything.
func (c *Client) doWith(hc *http.Client, method, path string, body, out any) error {
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
		return fmt.Errorf(
			"kof serve not reachable at %s — is the t-man agent running? (t-man status keeper-of-facts): %w",
			c.baseURL,
			err,
		)
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
			// "kof: not found"); surface them as-is to avoid double prefixing.
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("kof: serve returned %d", res.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

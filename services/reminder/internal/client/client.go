// Package client is the HTTP client for a running `reminder serve`. Both the
// MCP server and the CLI management commands use it, so neither writes the JSON
// store directly — serve stays the single writer.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mad01/thismoon/kit/agentdoc"
	"github.com/mad01/thismoon/services/reminder"
)

// Reminder mirrors the JSON the serve API returns.
type Reminder struct {
	ID        string     `json:"id"`
	Title     string     `json:"title"`
	Body      string     `json:"body,omitempty"`
	Due       time.Time  `json:"due"`
	Repeat    string     `json:"repeat,omitempty"`
	Status    string     `json:"status"`
	Overdue   bool       `json:"overdue"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	FiredAt   *time.Time `json:"fired_at,omitempty"`
}

// Client talks to a running `reminder serve` over its localhost HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the serve instance at baseURL.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 5 * time.Second}}
}

// CreateBody is the POST /api/reminders payload (set Due OR In).
type CreateBody struct {
	Title  string `json:"title"`
	Body   string `json:"body,omitempty"`
	Due    string `json:"due,omitempty"`
	In     string `json:"in,omitempty"`
	Repeat string `json:"repeat,omitempty"`
}

// UpdateBody is the PUT /api/reminders/{id} payload; nil fields are omitted.
type UpdateBody struct {
	Title  *string `json:"title,omitempty"`
	Body   *string `json:"body,omitempty"`
	Due    *string `json:"due,omitempty"`
	Repeat *string `json:"repeat,omitempty"`
}

func (c *Client) Create(in CreateBody) (Reminder, error) {
	var out Reminder
	return out, c.do(http.MethodPost, "/api/reminders", in, &out)
}

func (c *Client) Get(id string) (Reminder, error) {
	var out Reminder
	return out, c.do(http.MethodGet, "/api/reminders/"+id, nil, &out)
}

func (c *Client) List(status string) ([]Reminder, error) {
	path := "/api/reminders"
	if status != "" {
		path += "?status=" + status
	}
	var out struct {
		Reminders []Reminder `json:"reminders"`
	}
	return out.Reminders, c.do(http.MethodGet, path, nil, &out)
}

func (c *Client) Update(id string, in UpdateBody) (Reminder, error) {
	var out Reminder
	return out, c.do(http.MethodPut, "/api/reminders/"+id, in, &out)
}

func (c *Client) Cancel(id string) (Reminder, error) {
	var out Reminder
	return out, c.do(http.MethodPost, "/api/reminders/"+id+"/cancel", nil, &out)
}

// Test delivers a reminder's notification now without changing its state — a
// dry run to confirm notifications work. The returned reminder is unchanged.
func (c *Client) Test(id string) (Reminder, error) {
	var out Reminder
	return out, c.do(http.MethodPost, "/api/reminders/"+id+"/test", nil, &out)
}

// Fire fires a reminder for real now (recurring reschedules, one-shot → fired),
// returning the updated reminder.
func (c *Client) Fire(id string) (Reminder, error) {
	var out Reminder
	return out, c.do(http.MethodPost, "/api/reminders/"+id+"/fire", nil, &out)
}

// TestNotification sends a generic test notification with no reminder attached —
// a first-run check that notifications work at all.
func (c *Client) TestNotification() error {
	return c.do(http.MethodPost, "/api/test", nil, nil)
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
			"reminder serve not reachable at %s — is the t-man agent running? (t-man status reminder): %w",
			c.baseURL,
			err,
		), reminder.Facts())
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
			// "reminder: not found"); surface them as-is to avoid double prefixing.
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("reminder: serve returned %d", res.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

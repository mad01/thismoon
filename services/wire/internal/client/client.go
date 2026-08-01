// Package client is the HTTP client for a running `wire serve`. Both the MCP
// server and the CLI use it, so neither writes the JSONL logs directly and
// serve stays the single writer. It is also where a blocking read gets its
// timeout: a read that parks server-side for a minute needs a client that will
// wait a minute for it.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/mad01/thismoon/services/wire/internal/ref"
)

// requestTimeout bounds an ordinary call. A blocking read gets its own,
// derived from how long the caller asked the server to wait.
const requestTimeout = 10 * time.Second

// Channel mirrors a conversation as the serve API returns it.
type Channel struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Topic       string     `json:"topic,omitempty"`
	OpenedBy    string     `json:"opened_by,omitempty"`
	Conventions string     `json:"conventions,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CreatedAt   time.Time  `json:"created_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
	CloseNote   string     `json:"close_note,omitempty"`
}

// Closed reports whether the conversation takes no more messages.
func (c Channel) Closed() bool { return c.ClosedAt != nil }

// Summary is a channel plus the derived counts the list view shows and the
// connection string to hand another session.
type Summary struct {
	Channel
	Connect                string             `json:"connect"`
	Messages               int                `json:"messages"`
	Cursor                 int64              `json:"cursor"`
	Participants           []string           `json:"participants"`
	Members                []string           `json:"members,omitempty"`
	AwaitingReply          []int64            `json:"awaiting_reply,omitempty"`
	AwaitingReplyBy        map[string][]int64 `json:"awaiting_reply_by,omitempty"`
	AwaitingReplyOffRoster []string           `json:"awaiting_reply_off_roster,omitempty"`
	LastFrom               string             `json:"last_from,omitempty"`
	LastBody               string             `json:"last_body,omitempty"`
	LastAt                 *time.Time         `json:"last_at,omitempty"`
}

// Message mirrors one turn in a channel.
type Message struct {
	ChannelID   string    `json:"channel_id"`
	Seq         int64     `json:"seq"`
	From        string    `json:"from"`
	To          string    `json:"to,omitempty"`
	Body        string    `json:"body"`
	Kind        string    `json:"kind,omitempty"`
	ReplyTo     int64     `json:"reply_to,omitempty"`
	ReplyNeeded bool      `json:"reply_needed,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// Batch is one read's result: the channel, the messages after the cursor the
// reader gave, the cursor to resume from, and the seqs still owed an answer.
type Batch struct {
	Channel                Channel            `json:"channel"`
	Messages               []Message          `json:"messages"`
	Cursor                 int64              `json:"cursor"`
	Members                []string           `json:"members,omitempty"`
	AwaitingReply          []int64            `json:"awaiting_reply,omitempty"`
	AwaitingReplyBy        map[string][]int64 `json:"awaiting_reply_by,omitempty"`
	AwaitingReplyOffRoster []string           `json:"awaiting_reply_off_roster,omitempty"`
}

// OpenBody is the POST /api/channels payload; every field is optional.
type OpenBody struct {
	Name        string `json:"name,omitempty"`
	Topic       string `json:"topic,omitempty"`
	From        string `json:"from,omitempty"`
	Conventions string `json:"conventions,omitempty"`
}

// PostBody is one message. From and Body are required; the rest are the
// optional protocol fields.
type PostBody struct {
	From        string `json:"from"`
	To          string `json:"to,omitempty"`
	Body        string `json:"body"`
	Kind        string `json:"kind,omitempty"`
	ReplyTo     int64  `json:"reply_to,omitempty"`
	ReplyNeeded bool   `json:"reply_needed,omitempty"`
}

// ReadOptions selects what a read returns. Wait is how many seconds the server
// may park before answering with an empty batch; zero returns immediately.
type ReadOptions struct {
	Since int64
	Limit int
	Wait  int
}

// Client talks to a running `wire serve` over its localhost HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client for the serve instance at baseURL.
func New(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: requestTimeout}}
}

// Open creates a channel. An empty name gets a generated one.
func (c *Client) Open(ctx context.Context, in OpenBody) (Summary, error) {
	var out Summary
	return out, c.do(ctx, http.MethodPost, "/api/channels", in, requestTimeout, &out)
}

// List returns the channels, most recently active first; closed ones only when
// includeClosed is set.
func (c *Client) List(ctx context.Context, includeClosed bool) ([]Summary, error) {
	path := "/api/channels"
	if includeClosed {
		path += "?all=1"
	}
	var out struct {
		Channels []Summary `json:"channels"`
	}
	return out.Channels, c.do(ctx, http.MethodGet, path, nil, requestTimeout, &out)
}

// Get returns one channel by id, name, or connection string.
func (c *Client) Get(ctx context.Context, channel string) (Summary, error) {
	path, err := channelPath(channel)
	if err != nil {
		return Summary{}, err
	}
	var out Summary
	return out, c.do(ctx, http.MethodGet, path, nil, requestTimeout, &out)
}

// Post appends a message to a channel.
func (c *Client) Post(ctx context.Context, channel string, in PostBody) (Message, error) {
	path, err := channelPath(channel)
	if err != nil {
		return Message{}, err
	}
	var out Message
	return out, c.do(ctx, http.MethodPost, path+"/messages", in, requestTimeout, &out)
}

// Read returns the messages after opts.Since. With opts.Wait set the call
// blocks server-side until a message arrives or the wait expires, so the local
// timeout has to outlast it — otherwise the client gives up on a request the
// server is still holding open for good reason.
func (c *Client) Read(ctx context.Context, channel string, opts ReadOptions) (Batch, error) {
	path, err := channelPath(channel)
	if err != nil {
		return Batch{}, err
	}
	q := url.Values{}
	if opts.Since > 0 {
		q.Set("since", strconv.FormatInt(opts.Since, 10))
	}
	if opts.Limit > 0 {
		q.Set("limit", strconv.Itoa(opts.Limit))
	}
	if opts.Wait > 0 {
		q.Set("wait", strconv.Itoa(opts.Wait))
	}
	path += "/messages"
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	var out Batch
	return out, c.do(ctx, http.MethodGet, path, nil, readTimeout(opts.Wait), &out)
}

// readTimeout is how long to wait on a blocking read: the seconds the server
// was asked to park, plus the ordinary request budget on top. Without the
// margin the client would abandon a request the server is still holding open
// for exactly the reason it was asked to.
func readTimeout(wait int) time.Duration {
	if wait <= 0 {
		return requestTimeout
	}
	return requestTimeout + time.Duration(wait)*time.Second
}

// Join puts an agent on a channel's roster and returns the summary — the
// one-call briefing: conventions, members, and open obligations. Idempotent.
func (c *Client) Join(ctx context.Context, channel, from, note string) (Summary, error) {
	return c.roster(ctx, channel, "/join", from, note)
}

// Leave takes an agent off a channel's roster. Idempotent.
func (c *Client) Leave(ctx context.Context, channel, from, note string) (Summary, error) {
	return c.roster(ctx, channel, "/leave", from, note)
}

func (c *Client) roster(ctx context.Context, channel, op, from, note string) (Summary, error) {
	path, err := channelPath(channel)
	if err != nil {
		return Summary{}, err
	}
	var out Summary
	return out, c.do(ctx, http.MethodPost, path+op, joinBody{From: from, Note: note},
		requestTimeout, &out)
}

type joinBody struct {
	From string `json:"from"`
	Note string `json:"note,omitempty"`
}

// Close ends a conversation with an optional parting note.
func (c *Client) Close(ctx context.Context, channel, note string) (Summary, error) {
	path, err := channelPath(channel)
	if err != nil {
		return Summary{}, err
	}
	var out Summary
	return out, c.do(
		ctx,
		http.MethodPost,
		path+"/close",
		closeBody{Note: note},
		requestTimeout,
		&out,
	)
}

type closeBody struct {
	Note string `json:"note,omitempty"`
}

// channelPath turns a channel reference into an API path. A connection string
// is reduced to its channel first — its slashes would otherwise be escaped
// into a path segment no route matches — so every caller can paste the token
// the server handed them wherever a channel is expected.
func channelPath(channel string) (string, error) {
	name, err := ref.Parse(channel)
	if err != nil {
		return "", err
	}
	return "/api/channels/" + url.PathEscape(name), nil
}

// do issues a request and decodes a JSON response. A transport error (serve
// not running) and an API error ({"error": ...}) both come back as descriptive
// errors callers can relay to the user. ctx cancels a request in flight, which
// matters most for a blocking read: an interrupted caller stops waiting
// immediately instead of holding the connection for its full timeout.
func (c *Client) do(
	ctx context.Context,
	method, path string,
	body any,
	timeout time.Duration,
	out any,
) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	hc := c.http
	if timeout != hc.Timeout {
		clone := *hc
		clone.Timeout = timeout
		hc = &clone
	}
	res, err := hc.Do(req)
	if err != nil {
		return fmt.Errorf(
			"wire serve not reachable at %s — is the t-man agent running? (t-man status wire): %w",
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
			// Surface the serve error verbatim. It is unprefixed by
			// design — the CLI's main adds the "wire:" prefix once, and
			// double-prefixing every message is the alternative.
			return errors.New(apiErr.Error)
		}
		return fmt.Errorf("serve returned %d", res.StatusCode)
	}
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

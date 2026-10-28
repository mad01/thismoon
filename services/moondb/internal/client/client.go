// Package client is the HTTP client for a running `moondb serve`. The CLI and
// the MCP server read through it, so neither opens the store: serve is its
// single writer and the one process that reads it. The records come back as
// the store and query types themselves, so the wire shape has one definition.
package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/kit/agentdoc"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/query"
	"github.com/mad01/thismoon/services/moondb/internal/scorecard"
	"github.com/mad01/thismoon/services/moondb/internal/server"
	"github.com/mad01/thismoon/services/moondb/internal/store"
	"github.com/mad01/thismoon/services/moondb/internal/trace"
)

const (
	// requestTimeout bounds a store read: serve answers from its index.
	requestTimeout = 5 * time.Second
	// queryTimeout bounds why, history and annotate, which may run git in a
	// worktree, and a session trace or scorecard, which may read every
	// turn's transcript.
	queryTimeout = 30 * time.Second
)

// Client talks to a running `moondb serve` over its localhost HTTP API.
type Client struct {
	baseURL string
	http    *http.Client
	// slow carries why, history, annotate, session traces and scorecards;
	// every other call keeps the short timeout.
	slow *http.Client
}

// New returns a Client for the serve instance at baseURL, e.g.
// http://localhost:7433.
func New(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: requestTimeout},
		slow:    &http.Client{Timeout: queryTimeout},
	}
}

// SessionQuery narrows a Sessions listing. Zero values mean no filter, and a
// zero Limit takes the server's default.
type SessionQuery struct {
	// Repo keeps sessions with at least one edit in the repository.
	Repo string
	// Since is an RFC 3339 time or a duration before now: 90m, 24h, 7d.
	Since string
	Limit int
}

// EditQuery narrows an Edits listing. Zero values mean no filter, and a zero
// Limit takes the server's default.
type EditQuery struct {
	Repo     string
	Path     string
	Worktree string
	Session  string
	Turn     string
	// Blob matches edits whose after-content has this git blob id.
	Blob string
	// Since is an RFC 3339 time or a duration before now: 90m, 24h, 7d.
	Since string
	Limit int
}

// Sessions lists recorded sessions most recently active first, each with what it touched.
func (c *Client) Sessions(ctx context.Context, q SessionQuery) ([]server.SessionSummary, error) {
	v := url.Values{}
	set(v, "repo", q.Repo)
	set(v, "since", q.Since)
	setInt(v, "limit", q.Limit)
	var out struct {
		Sessions []server.SessionSummary `json:"sessions"`
	}
	return out.Sessions, c.getJSON(ctx, c.http, "/api/sessions", v, &out)
}

// Session loads one session with its turns, its listing row and its
// agents. A missing id matches store.ErrNotFound.
func (c *Client) Session(ctx context.Context, id string) (server.SessionView, error) {
	var out server.SessionView
	return out, c.getJSON(ctx, c.http, "/api/sessions/"+url.PathEscape(id), nil, &out)
}

// Turn loads one turn with everything recorded under it and its trace
// totals. A missing id matches store.ErrNotFound.
func (c *Client) Turn(ctx context.Context, id string) (server.TurnView, error) {
	var out server.TurnView
	return out, c.getJSON(ctx, c.http, "/api/turns/"+url.PathEscape(id), nil, &out)
}

// SessionTrace loads what took time in a session: every turn's spans and
// totals, and the user time between turns. It rides the slow client: serve
// reads every turn's transcript chunk that is not cached. A missing id
// matches store.ErrNotFound.
func (c *Client) SessionTrace(ctx context.Context, id string) (trace.SessionTrace, error) {
	var out trace.SessionTrace
	path := "/api/sessions/" + url.PathEscape(id) + "/trace"
	return out, c.getJSON(ctx, c.slow, path, nil, &out)
}

// SessionScorecard loads how a session spent its context. It rides the
// slow client: serve reads every turn's and agent's transcript chunk and
// asks the events service and keeper-of-facts. A missing id matches
// store.ErrNotFound.
func (c *Client) SessionScorecard(ctx context.Context, id string) (scorecard.Scorecard, error) {
	var out scorecard.Scorecard
	path := "/api/sessions/" + url.PathEscape(id) + "/scorecard"
	return out, c.getJSON(ctx, c.slow, path, nil, &out)
}

// Edits lists recorded edits newest first.
func (c *Client) Edits(ctx context.Context, q EditQuery) ([]store.Edit, error) {
	v := url.Values{}
	set(v, "repo", q.Repo)
	set(v, "path", q.Path)
	set(v, "worktree", q.Worktree)
	set(v, "session", q.Session)
	set(v, "turn", q.Turn)
	set(v, "blob", q.Blob)
	set(v, "since", q.Since)
	setInt(v, "limit", q.Limit)
	var out struct {
		Edits []store.Edit `json:"edits"`
	}
	return out.Edits, c.getJSON(ctx, c.http, "/api/edits", v, &out)
}

// InflightQuery asks for one checkout's in-flight files. A zero Since or
// Limit takes the server's default, moondb.InflightWindow or
// moondb.InflightLimit.
type InflightQuery struct {
	// Worktree is the absolute checkout root; required.
	Worktree string
	// ExcludeSession is the asking session, whose own edits never count.
	ExcludeSession string
	// Since is an RFC 3339 time or a duration before now: 90m, 24h, 7d.
	Since string
	Limit int
}

// Inflight lists the files of one checkout with edits no commit has taken
// yet, made by sessions other than q.ExcludeSession, newest edit first.
func (c *Client) Inflight(ctx context.Context, q InflightQuery) (server.InflightView, error) {
	v := url.Values{}
	set(v, "worktree", q.Worktree)
	set(v, "exclude_session", q.ExcludeSession)
	set(v, "since", q.Since)
	setInt(v, "limit", q.Limit)
	var out server.InflightView
	return out, c.getJSON(ctx, c.http, "/api/inflight", v, &out)
}

// Why asks which recorded edit produced one line. It rides the slow client
// because serve may run git blame to answer.
func (c *Client) Why(ctx context.Context, req query.WhyRequest) (query.WhyResult, error) {
	v := url.Values{}
	set(v, "worktree", req.Worktree)
	set(v, "repo", req.Repo)
	set(v, "path", req.Path)
	setInt(v, "line", req.Line)
	set(v, "text", req.Text)
	var out query.WhyResult
	return out, c.getJSON(ctx, c.slow, "/api/why", v, &out)
}

// History lists the recorded edits of one path, newest first.
func (c *Client) History(ctx context.Context, req query.HistoryRequest) ([]store.Edit, error) {
	v := url.Values{}
	set(v, "worktree", req.Worktree)
	set(v, "repo", req.Repo)
	set(v, "path", req.Path)
	setInt(v, "limit", req.Limit)
	var out struct {
		Edits []store.Edit `json:"edits"`
	}
	return out.Edits, c.getJSON(ctx, c.slow, "/api/history", v, &out)
}

// Annotate explains every line of one file on disk in a worktree. It rides
// the slow client because serve runs git blame to answer.
func (c *Client) Annotate(
	ctx context.Context,
	req query.AnnotateRequest,
) (query.AnnotateResult, error) {
	v := url.Values{}
	set(v, "worktree", req.Worktree)
	set(v, "repo", req.Repo)
	set(v, "path", req.Path)
	var out query.AnnotateResult
	return out, c.getJSON(ctx, c.slow, "/api/annotate", v, &out)
}

// Chunk returns the bytes stored under a chunk key. A missing key matches
// store.ErrNotFound.
func (c *Client) Chunk(ctx context.Context, sha string) ([]byte, error) {
	return c.get(ctx, c.http, "/api/chunks/"+url.PathEscape(sha), nil)
}

// Health reports whether serve answers and its store answers Ping. Its
// signature fits a kit/doctor Check's Run.
func (c *Client) Health(ctx context.Context) error {
	_, err := c.get(ctx, c.http, "/healthz", nil)
	return err
}

// Version returns the build metadata serve reports on /version.
func (c *Client) Version(ctx context.Context) (buildinfo.Info, error) {
	var out buildinfo.Info
	return out, c.getJSON(ctx, c.http, "/version", nil, &out)
}

func set(v url.Values, key, value string) {
	if value != "" {
		v.Set(key, value)
	}
}

func setInt(v url.Values, key string, n int) {
	if n != 0 {
		v.Set(key, strconv.Itoa(n))
	}
}

// getJSON issues a GET and decodes the JSON body into out.
func (c *Client) getJSON(
	ctx context.Context, hc *http.Client, path string, q url.Values, out any,
) error {
	raw, err := c.get(ctx, hc, path, q)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("moondb: decode %s: %w", path, err)
	}
	return nil
}

// get issues a GET and returns the body of a 2xx answer. A transport failure
// comes back with the doctor hint; a non-2xx answer as an *APIError.
func (c *Client) get(
	ctx context.Context,
	hc *http.Client,
	path string,
	q url.Values,
) ([]byte, error) {
	target := c.baseURL + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("moondb: build request: %w", err)
	}
	res, err := hc.Do(req)
	if err != nil {
		return nil, c.transportErr(ctx, path, hc.Timeout, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("moondb: read %s: %w", path, err)
	}
	if res.StatusCode >= http.StatusBadRequest {
		return nil, apiError(res.StatusCode, raw)
	}
	return raw, nil
}

// transportErr names why a request got no answer. The caller cancelling is
// passed through; a timeout says so; anything else means serve is not
// listening, and the hint points the reader at `moondb doctor`.
func (c *Client) transportErr(
	ctx context.Context,
	path string,
	limit time.Duration,
	err error,
) error {
	if ctx.Err() != nil {
		return fmt.Errorf("moondb: %s: %w", path, ctx.Err())
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return agentdoc.Hint(fmt.Errorf(
			"moondb: serve at %s did not answer %s within %s: %w", c.baseURL, path, limit, err,
		), moondb.Facts())
	}
	return agentdoc.Hint(fmt.Errorf(
		"moondb: serve not reachable at %s (is 'moondb serve' running?): %w", c.baseURL, err,
	), moondb.Facts())
}

// APIError is a non-2xx answer from serve: the status and the server's
// message. It matches store.ErrNotFound on a 404 and query.ErrNotImplemented
// on a 501, so callers test the outcome the way they would against the store.
type APIError struct {
	Status  int
	Message string
}

// Error returns the server's message, already prefixed "moondb: ".
func (e *APIError) Error() string { return e.Message }

// Is maps the status back to the sentinel the server mapped it from.
func (e *APIError) Is(target error) bool {
	switch e.Status {
	case http.StatusNotFound:
		return target == store.ErrNotFound
	case http.StatusNotImplemented:
		return target == query.ErrNotImplemented
	}
	return false
}

// apiError reads the {"error": ...} body; a body without one, such as a proxy
// page, falls back to the status line.
func apiError(status int, raw []byte) *APIError {
	var body struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(raw, &body)
	if body.Error == "" {
		body.Error = fmt.Sprintf("moondb: serve returned %d %s", status, http.StatusText(status))
	}
	return &APIError{Status: status, Message: body.Error}
}

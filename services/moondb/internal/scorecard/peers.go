package scorecard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/moondb"
)

// Peers reads the two loopback services a scorecard joins. *HTTPPeers is
// the real one; tests pass a fake or point it at an httptest server.
type Peers interface {
	// Events lists entries of the events service, newest first.
	Events(ctx context.Context, q EventQuery) ([]Event, error)
	// Assertions lists every keeper-of-facts assertion.
	Assertions(ctx context.Context) ([]Assertion, error)
}

// EventQuery narrows an events listing. Q is a substring of the title,
// message or tags; Before keeps the entries whose id sorts before it.
type EventQuery struct {
	Source string
	Q      string
	Before string
	Limit  int
}

// Event is one events-service entry, the fields a scorecard reads.
type Event struct {
	ID      string            `json:"id"`
	Time    time.Time         `json:"time"`
	Source  string            `json:"source"`
	Title   string            `json:"title"`
	Message string            `json:"message"`
	Tags    map[string]string `json:"tags"`
}

// Assertion is a keeper-of-facts assertion, the fields a scorecard reads.
type Assertion struct {
	ID   string `json:"id"`
	Pins []Pin  `json:"pins"`
}

// Pin is one file an assertion is pinned to: its path in the repository,
// and the repository when known (github.com/owner/name).
type Pin struct {
	Repo string `json:"repo"`
	File string `json:"file"`
}

// HTTPPeers reads the events service and keeper-of-facts over HTTP.
type HTTPPeers struct {
	eventsURL string
	kofURL    string
	http      *http.Client
}

// NewHTTPPeers returns Peers for the events service at eventsURL and
// keeper-of-facts at kofURL, each request bounded by moondb.PeerTimeout.
// A scorecard asks keeper-of-facts once, so PeerTimeout is its whole
// budget there.
func NewHTTPPeers(eventsURL, kofURL string) *HTTPPeers {
	return &HTTPPeers{
		eventsURL: strings.TrimRight(eventsURL, "/"),
		kofURL:    strings.TrimRight(kofURL, "/"),
		http:      &http.Client{Timeout: moondb.PeerTimeout},
	}
}

// Events asks GET /api/events.
func (p *HTTPPeers) Events(ctx context.Context, q EventQuery) ([]Event, error) {
	v := url.Values{}
	for key, value := range map[string]string{
		"source": q.Source, "q": q.Q, "before": q.Before,
	} {
		if value != "" {
			v.Set(key, value)
		}
	}
	if q.Limit > 0 {
		v.Set("limit", strconv.Itoa(q.Limit))
	}
	var out []Event
	err := p.getJSON(ctx, p.eventsURL+"/api/events?"+v.Encode(), &out)
	if err != nil {
		return nil, fmt.Errorf("events: %w", err)
	}
	return out, nil
}

// Assertions asks GET /api/assertions, which lists every assertion with
// no filter.
func (p *HTTPPeers) Assertions(ctx context.Context) ([]Assertion, error) {
	var out struct {
		Assertions []Assertion `json:"assertions"`
	}
	if err := p.getJSON(ctx, p.kofURL+"/api/assertions", &out); err != nil {
		return nil, fmt.Errorf("kof: %w", err)
	}
	return out.Assertions, nil
}

// maxPeerError caps the body an error message quotes.
const maxPeerError = 200

// getJSON fetches u and decodes a 200 answer into out.
func (p *HTTPPeers) getJSON(ctx context.Context, u string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := p.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusOK {
		return json.NewDecoder(res.Body).Decode(out)
	}
	body, _ := io.ReadAll(io.LimitReader(res.Body, maxPeerError))
	return fmt.Errorf("GET %s: %s: %s", u, res.Status, strings.TrimSpace(string(body)))
}

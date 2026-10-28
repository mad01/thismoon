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

// Peers reads the loopback service a scorecard joins, the events service.
// *HTTPPeers is the real one; tests pass a fake or point it at an httptest
// server.
type Peers interface {
	// Events lists entries of the events service, newest first.
	Events(ctx context.Context, q EventQuery) ([]Event, error)
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

// HTTPPeers reads the events service over HTTP.
type HTTPPeers struct {
	eventsURL string
	http      *http.Client
}

// NewHTTPPeers returns Peers for the events service at eventsURL, each
// request bounded by moondb.PeerTimeout.
func NewHTTPPeers(eventsURL string) *HTTPPeers {
	return &HTTPPeers{
		eventsURL: strings.TrimRight(eventsURL, "/"),
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

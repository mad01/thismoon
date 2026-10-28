// Package jsdelivr looks up a file on jsDelivr by its sha256 and reports the
// published package version it is a copy of. Discovery uses it to name
// vendored JavaScript files; apart from the OSV check and go list's module
// downloads, it is the only way deps reaches the network. Lookups are cached
// in memory: a hit for the life of the process, since a hash always names the
// same bytes, and a miss for a day, since jsDelivr indexes a file only after
// serving it once.
package jsdelivr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// defaultBaseURL is the public jsDelivr data API.
const defaultBaseURL = "https://data.jsdelivr.com"

// missTTL is how long a miss stays cached. It matches the API's own
// Cache-Control: a day for a miss, a year for a hit.
const missTTL = 24 * time.Hour

// File is the published file jsDelivr matched to a hash.
type File struct {
	// Type is the registry the file was published to: "npm", or "gh" for a
	// GitHub repository.
	Type    string `json:"type"`
	Name    string `json:"name"`
	Version string `json:"version"`
	// Path is the file's path inside the package, such as /dist/cytoscape.min.js.
	Path string `json:"file"`
}

// Client looks up files on the jsDelivr data API. It is safe for concurrent
// use: the serve loop and a manual check may discover at the same time.
type Client struct {
	baseURL string
	http    *http.Client
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]entry
}

// entry is one cached lookup result.
type entry struct {
	file  File
	found bool
	at    time.Time
}

// New returns a Client against the public jsDelivr data API.
func New() *Client { return NewWithURL(defaultBaseURL) }

// NewWithURL returns a Client against baseURL (used by tests with an httptest
// server).
func NewWithURL(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 15 * time.Second},
		now:     time.Now,
		cache:   map[string]entry{},
	}
}

// Lookup returns the published file whose contents hash to sum, a hex-encoded
// sha256. found is false when jsDelivr knows no such file. An error means the
// lookup itself failed, so the caller cannot tell a match from a miss; errors
// are never cached.
func (c *Client) Lookup(ctx context.Context, sum string) (File, bool, error) {
	if e, ok := c.cached(sum); ok {
		return e.file, e.found, nil
	}
	file, found, err := c.fetch(ctx, sum)
	if err != nil {
		return File{}, false, err
	}
	c.mu.Lock()
	c.cache[sum] = entry{file: file, found: found, at: c.now()}
	c.mu.Unlock()
	return file, found, nil
}

// cached returns the cached result for sum while it is still fresh.
func (c *Client) cached(sum string) (entry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[sum]
	if !ok || (!e.found && c.now().Sub(e.at) >= missTTL) {
		return entry{}, false
	}
	return e, true
}

// fetch asks the API once: 200 is a match, 404 a miss, anything else an error.
func (c *Client) fetch(ctx context.Context, sum string) (File, bool, error) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		c.baseURL+"/v1/lookup/hash/"+sum,
		nil,
	)
	if err != nil {
		return File{}, false, fmt.Errorf("jsdelivr: build request: %w", err)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return File{}, false, fmt.Errorf("jsdelivr: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusNotFound {
		return File{}, false, nil
	}
	if res.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return File{}, false, fmt.Errorf(
			"jsdelivr: lookup returned %d: %s",
			res.StatusCode,
			bytes.TrimSpace(raw),
		)
	}
	var file File
	if err := json.NewDecoder(res.Body).Decode(&file); err != nil {
		return File{}, false, fmt.Errorf("jsdelivr: decode lookup: %w", err)
	}
	if file.Type == "" || file.Name == "" || file.Version == "" {
		return File{}, false, errors.New("jsdelivr: lookup answer lacks type, name or version")
	}
	return file, true, nil
}

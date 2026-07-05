// Package osv checks dependency versions against the OSV.dev advisory database.
// One HTTP API (api.osv.dev) covers every ecosystem we discover — Go, npm,
// PyPI — so the same batch query path serves all of them. It is the only part
// of deps that talks to the network.
package osv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// defaultBaseURL is the public OSV.dev API.
const defaultBaseURL = "https://api.osv.dev"

// batchSize is OSV's documented querybatch ceiling.
const batchSize = 1000

// Client queries the OSV.dev API.
type Client struct {
	baseURL string
	http    *http.Client
}

// New returns a Client against the public OSV.dev API.
func New() *Client { return NewWithURL(defaultBaseURL) }

// NewWithURL returns a Client against baseURL (used by tests with an httptest
// server).
func NewWithURL(baseURL string) *Client {
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
}

// Query identifies one package version to check.
type Query struct {
	Name      string
	Ecosystem string
	Version   string
}

// Check returns the advisories for each query, aligned by index (queries[i] →
// result[i]). It batches the lookups, then fetches each unique advisory once.
func (c *Client) Check(ctx context.Context, queries []Query) ([][]store.Advisory, error) {
	results := make([][]store.Advisory, len(queries))

	// idsPerQuery[i] = advisory ids affecting queries[i].
	idsPerQuery := make([][]string, len(queries))
	for start := 0; start < len(queries); start += batchSize {
		end := min(start+batchSize, len(queries))
		ids, err := c.queryBatch(ctx, queries[start:end])
		if err != nil {
			return nil, err
		}
		copy(idsPerQuery[start:end], ids)
	}

	// Fetch each distinct advisory once.
	cache := map[string]*vuln{}
	for _, ids := range idsPerQuery {
		for _, id := range ids {
			if _, ok := cache[id]; ok {
				continue
			}
			v, err := c.fetchVuln(ctx, id)
			if err != nil {
				return nil, err
			}
			cache[id] = v
		}
	}

	for i, ids := range idsPerQuery {
		for _, id := range ids {
			v := cache[id]
			if v == nil {
				continue
			}
			results[i] = append(results[i], store.Advisory{
				ID:           v.ID,
				Summary:      v.summary(),
				Severity:     v.severity(),
				FixedVersion: v.fixedFor(queries[i]),
			})
		}
	}
	return results, nil
}

// batchRequest / batchResponse model POST /v1/querybatch.
type batchRequest struct {
	Queries []batchQuery `json:"queries"`
}

type batchQuery struct {
	Package batchPackage `json:"package"`
	Version string       `json:"version"`
}

type batchPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

type batchResponse struct {
	Results []struct {
		Vulns []struct {
			ID string `json:"id"`
		} `json:"vulns"`
	} `json:"results"`
}

// queryBatch returns the advisory ids for each query in the batch, index-aligned.
func (c *Client) queryBatch(ctx context.Context, queries []Query) ([][]string, error) {
	req := batchRequest{Queries: make([]batchQuery, len(queries))}
	for i, q := range queries {
		req.Queries[i] = batchQuery{
			Package: batchPackage{Name: q.Name, Ecosystem: q.Ecosystem},
			Version: q.Version,
		}
	}
	var resp batchResponse
	if err := c.post(ctx, "/v1/querybatch", req, &resp); err != nil {
		return nil, err
	}
	out := make([][]string, len(queries))
	for i := range queries {
		if i >= len(resp.Results) {
			break
		}
		for _, v := range resp.Results[i].Vulns {
			out[i] = append(out[i], v.ID)
		}
	}
	return out, nil
}

// vuln models the subset of GET /v1/vulns/{id} we report.
type vuln struct {
	ID       string `json:"id"`
	Summary  string `json:"summary"`
	Details  string `json:"details"`
	Severity []struct {
		Type  string `json:"type"`
		Score string `json:"score"`
	} `json:"severity"`
	DatabaseSpecific struct {
		Severity string `json:"severity"`
	} `json:"database_specific"`
	Affected []struct {
		Package struct {
			Ecosystem string `json:"ecosystem"`
			Name      string `json:"name"`
		} `json:"package"`
		Ranges []struct {
			Type   string `json:"type"`
			Events []struct {
				Introduced string `json:"introduced"`
				Fixed      string `json:"fixed"`
			} `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

func (c *Client) fetchVuln(ctx context.Context, id string) (*vuln, error) {
	var v vuln
	if err := c.get(ctx, "/v1/vulns/"+id, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// summary is a short human description: the summary line, falling back to the
// (longer) details if OSV only provided those.
func (v *vuln) summary() string {
	if v.Summary != "" {
		return v.Summary
	}
	return v.Details
}

// severity prefers the database-specific label (HIGH/MODERATE/…, what GHSA
// advisories carry) and falls back to a CVSS vector if that is all OSV has.
func (v *vuln) severity() string {
	if v.DatabaseSpecific.Severity != "" {
		return v.DatabaseSpecific.Severity
	}
	if len(v.Severity) > 0 {
		return v.Severity[0].Score
	}
	return ""
}

// fixedFor returns the version that fixes this advisory for the queried package
// — the "fixed" event of the affected range matching the package name. Empty if
// OSV lists no fix.
func (v *vuln) fixedFor(q Query) string {
	for _, a := range v.Affected {
		if a.Package.Name != q.Name {
			continue
		}
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				if e.Fixed != "" {
					return e.Fixed
				}
			}
		}
	}
	return ""
}

func (c *Client) post(ctx context.Context, path string, body, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		c.baseURL+path,
		bytes.NewReader(raw),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req, out)
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	return c.do(req, out)
}

func (c *Client) do(req *http.Request, out any) error {
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("osv request %s: %w", req.URL.Path, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode >= 400 {
		raw, _ := io.ReadAll(res.Body)
		return fmt.Errorf("osv %s returned %d: %s", req.URL.Path, res.StatusCode, raw)
	}
	if err := json.NewDecoder(res.Body).Decode(out); err != nil {
		return fmt.Errorf("decode osv response: %w", err)
	}
	return nil
}

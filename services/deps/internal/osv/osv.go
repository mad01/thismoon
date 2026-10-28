// Package osv checks dependency versions against the OSV.dev advisory database.
// One HTTP API (api.osv.dev) covers every ecosystem we discover (Go, npm,
// PyPI, SwiftURL by version, GitHub Actions by commit), so the same batch query
// path serves all of them. OSV's data export supplies the names it files
// each GitHub Action under (names.go). Three other parts of deps reach the
// network: package regcheck asks the npm registry, PyPI and the Go module
// proxy whether each version was withdrawn, blocked, yanked, deprecated or
// retracted; discovery looks files up by hash (the jsdelivr package) and reads
// the ref lists that resolve a GitHub Action's tag; and go list downloads
// modules.
package osv

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// defaultBaseURL is the public OSV.dev API.
const defaultBaseURL = "https://api.osv.dev"

// batchSize is OSV's documented querybatch ceiling.
const batchSize = 1000

// Client queries the OSV.dev API and reads its data exports.
type Client struct {
	baseURL string
	dataURL string
	http    *http.Client
}

// New returns a Client against the public OSV.dev API and data bucket.
func New() *Client {
	c := NewWithURL(defaultBaseURL)
	c.dataURL = defaultDataURL
	return c
}

// NewWithURL returns a Client that reads both the API and the data exports
// from baseURL (used by tests with an httptest server).
func NewWithURL(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		dataURL: baseURL,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

// Query identifies one package version to check. Commit, when set, checks a
// package pinned to a git commit, such as a GitHub Action, whose ecosystem OSV
// can't match by version. OSV is then asked twice: for the advisories whose
// git ranges hold the commit, and for every advisory on Name, of which Check
// keeps those whose version ranges hold Version (a label such as v45.0.7).
// A label it can't compare keeps them all. A GitHub Action is also asked for
// by each other name OSV files it under (actionNames): another
// capitalization, or a sub-action's path or URL. The results stay aligned
// with the query as written.
type Query struct {
	Name      string
	Ecosystem string
	Version   string
	Commit    string
}

// maxPages bounds the querybatch requests one batch may take, so a server that
// keeps answering with a next_page_token cannot hold a scan forever. OSV pages
// only past 1,000 advisories for one query or 3,000 for a batch, so a real scan
// needs one or two.
const maxPages = 20

// Check returns the advisories for each query, aligned by index (queries[i] →
// result[i]). It batches the lookups, then fetches each unique advisory once.
// A record that fails to fetch still yields its finding, with the id and kind
// but no summary, severity or fix: one unreadable record must not cost the
// whole scan. When a GitHub Action is looked up by name, it first reads OSV's
// name list, once per call; if that fails, the results come back complete
// beside an *ActionNamesError, the names having been asked for as written.
func (c *Client) Check(ctx context.Context, queries []Query) ([][]store.Advisory, error) {
	var (
		names    actionNames
		namesErr error
	)
	if needsActionNames(queries) {
		if names, namesErr = c.fetchActionNames(ctx); namesErr != nil {
			namesErr = &ActionNamesError{Err: namesErr}
		}
	}
	results, err := c.check(ctx, queries, names)
	if err != nil {
		return nil, err
	}
	return results, namesErr
}

// check runs the lookups for queries, asking for each GitHub Action by the
// spellings in names.
func (c *Client) check(
	ctx context.Context,
	queries []Query,
	names actionNames,
) ([][]store.Advisory, error) {
	ls := lookups(queries, names)
	// idsPerLookup[j] = advisory ids ls[j] matched.
	idsPerLookup := make([][]string, len(ls))
	for start := 0; start < len(ls); start += batchSize {
		end := min(start+batchSize, len(ls))
		ids, err := c.queryBatch(ctx, ls[start:end])
		if err != nil {
			return nil, err
		}
		copy(idsPerLookup[start:end], ids)
	}

	records, err := c.fetchAll(ctx, idsPerLookup)
	if err != nil {
		return nil, err
	}

	results := make([][]store.Advisory, len(queries))
	for j, ids := range idsPerLookup {
		i, q := ls[j].query, queries[ls[j].query]
		for _, id := range ids {
			v := records[id]
			if ls[j].byName && !v.affects(q) {
				continue
			}
			if slices.ContainsFunc(
				results[i],
				func(a store.Advisory) bool { return a.ID == v.ID },
			) {
				continue
			}
			results[i] = append(results[i], store.Advisory{
				ID:           v.ID,
				Kind:         store.KindForID(v.ID),
				Summary:      v.summary(),
				Severity:     v.severity(),
				FixedVersion: v.fixedFor(q),
			})
		}
	}
	return results, nil
}

// lookup is one querybatch entry and the index of the Query it serves. A
// Query by version or by commit is one lookup; a commit Query adds a byName
// lookup per spelling of its name, which asks for every advisory on the
// package.
type lookup struct {
	query  int
	byName bool
	wire   batchQuery
}

// lookups expands queries into the querybatch entries that answer them,
// asking for a GitHub Action as written and by each other spelling names has
// for it.
func lookups(queries []Query, names actionNames) []lookup {
	out := make([]lookup, 0, len(queries))
	for i, q := range queries {
		if q.Commit == "" {
			pkg := &batchPackage{Name: q.Name, Ecosystem: q.Ecosystem}
			out = append(out, lookup{query: i, wire: batchQuery{Package: pkg, Version: q.Version}})
			continue
		}
		out = append(out, lookup{query: i, wire: batchQuery{Commit: q.Commit}})
		spellings := []string{q.Name}
		if q.Ecosystem == actionsEcosystem {
			spellings = names.spellings(q.Name)
		}
		for _, name := range spellings {
			pkg := &batchPackage{Name: name, Ecosystem: q.Ecosystem}
			out = append(out, lookup{query: i, byName: true, wire: batchQuery{Package: pkg}})
		}
	}
	return out
}

// matchesPackage reports whether a record's affected package, by ecosystem
// and name, is q's package. A GitHub Action matches only a GitHub Actions
// entry, so a record's GIT entry for the same repo never lends its commit
// ranges to the row. It matches ignoring case, as GitHub resolves owner/repo,
// and a longer name matches the owner/repo it is filed under (actionRepo).
func (q Query) matchesPackage(ecosystem, name string) bool {
	if q.Ecosystem == actionsEcosystem {
		return ecosystem == actionsEcosystem && strings.EqualFold(actionRepo(name), q.Name)
	}
	return name == q.Name
}

// fetchAll fetches each distinct advisory id once. A failed fetch is logged
// and stands in as a record carrying only its id; only a cancelled or expired
// ctx fails the whole call.
func (c *Client) fetchAll(ctx context.Context, idsPerQuery [][]string) (map[string]*vuln, error) {
	records := map[string]*vuln{}
	for _, ids := range idsPerQuery {
		for _, id := range ids {
			if _, ok := records[id]; ok {
				continue
			}
			v, err := c.fetchVuln(ctx, id)
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				log.Printf("deps: osv: %v; keeping %s with its id only", err, id)
				v = &vuln{ID: id}
			}
			records[id] = v
		}
	}
	return records, nil
}

// batchRequest / batchResponse model POST /v1/querybatch. A query names a
// package and version, a commit alone (OSV rejects a commit beside a
// version), or a package alone for every advisory on it.
type batchRequest struct {
	Queries []batchQuery `json:"queries"`
}

type batchQuery struct {
	Package   *batchPackage `json:"package,omitempty"`
	Version   string        `json:"version,omitempty"`
	Commit    string        `json:"commit,omitempty"`
	PageToken string        `json:"page_token,omitempty"`
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
		NextPageToken string `json:"next_page_token"`
	} `json:"results"`
}

// pageQuery is one query still owed results: its index in the batch and the
// page token OSV handed back for it (empty on the first request).
type pageQuery struct {
	index int
	token string
}

// queryBatch returns the advisory ids for each lookup in the batch,
// index-aligned. OSV pages a result that grows past its limits: the next
// request repeats only the lookups that came back with a next_page_token, each
// carrying its token.
func (c *Client) queryBatch(ctx context.Context, ls []lookup) ([][]string, error) {
	out := make([][]string, len(ls))
	pending := make([]pageQuery, len(ls))
	for i := range ls {
		pending[i] = pageQuery{index: i}
	}
	for page := 0; len(pending) > 0; page++ {
		if page == maxPages {
			return nil, fmt.Errorf("osv querybatch still paging after %d requests", maxPages)
		}
		req := batchRequest{Queries: make([]batchQuery, len(pending))}
		for j, p := range pending {
			req.Queries[j] = ls[p.index].wire
			req.Queries[j].PageToken = p.token
		}
		var resp batchResponse
		if err := c.post(ctx, "/v1/querybatch", req, &resp); err != nil {
			return nil, err
		}
		var next []pageQuery
		for j, p := range pending {
			if j >= len(resp.Results) {
				break
			}
			r := resp.Results[j]
			for _, v := range r.Vulns {
				out[p.index] = append(out[p.index], v.ID)
			}
			if r.NextPageToken != "" {
				next = append(next, pageQuery{index: p.index, token: r.NextPageToken})
			}
		}
		pending = next
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
			Type   string  `json:"type"`
			Events []event `json:"events"`
		} `json:"ranges"`
	} `json:"affected"`
}

// event is one bound of an affected range.
type event struct {
	Introduced   string `json:"introduced"`
	Fixed        string `json:"fixed"`
	LastAffected string `json:"last_affected"`
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
		if !q.matchesPackage(a.Package.Ecosystem, a.Package.Name) {
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

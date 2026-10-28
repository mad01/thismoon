package osv

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/store"
)

// osvStub serves canned /v1/querybatch and /v1/vulns/{id} responses. The batch
// flags only the second query (left-pad), pointing at advisory GHSA-xxxx, whose
// record fixes left-pad in 1.3.0.
func osvStub(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/querybatch", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{},{"vulns":[{"id":"GHSA-xxxx"}]}]}`))
	})
	mux.HandleFunc("GET /v1/vulns/GHSA-xxxx", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"id": "GHSA-xxxx",
			"summary": "left-pad pads left",
			"database_specific": { "severity": "HIGH" },
			"affected": [{
				"package": { "ecosystem": "npm", "name": "left-pad" },
				"ranges": [{
					"type": "SEMVER",
					"events": [ { "introduced": "0" }, { "fixed": "1.3.0" } ]
				}]
			}]
		}`))
	})
	return httptest.NewServer(mux)
}

func TestCheck(t *testing.T) {
	srv := osvStub(t)
	defer srv.Close()

	c := NewWithURL(srv.URL)
	queries := []Query{
		{Name: "safe-pkg", Ecosystem: "npm", Version: "1.0.0"},
		{Name: "left-pad", Ecosystem: "npm", Version: "1.2.0"},
	}
	got, err := c.Check(context.Background(), queries)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d result rows, want 2", len(got))
	}
	if len(got[0]) != 0 {
		t.Errorf("safe-pkg should have no advisories, got %+v", got[0])
	}
	if len(got[1]) != 1 {
		t.Fatalf("left-pad should have 1 advisory, got %+v", got[1])
	}
	adv := got[1][0]
	if adv.ID != "GHSA-xxxx" {
		t.Errorf("id = %q, want GHSA-xxxx", adv.ID)
	}
	if adv.Kind != store.KindVuln {
		t.Errorf("kind = %q, want %q", adv.Kind, store.KindVuln)
	}
	if adv.Severity != "HIGH" {
		t.Errorf("severity = %q, want HIGH", adv.Severity)
	}
	if adv.FixedVersion != "1.3.0" {
		t.Errorf("fixed = %q, want 1.3.0", adv.FixedVersion)
	}
	if adv.Summary != "left-pad pads left" {
		t.Errorf("summary = %q", adv.Summary)
	}
}

// TestCheckMaliciousRecord is the chalk@5.6.1 fixture: OSV answers with the
// MAL- record from the OpenSSF malicious-packages database (testdata holds a
// trimmed copy of the real one), which has no severity and no fix.
func TestCheckMaliciousRecord(t *testing.T) {
	record, err := os.ReadFile("testdata/MAL-2025-46969.json")
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/querybatch", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[{"vulns":[{"id":"MAL-2025-46969"}]}]}`))
	})
	mux.HandleFunc("GET /v1/vulns/MAL-2025-46969", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(record)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{
		{Name: "chalk", Ecosystem: "npm", Version: "5.6.1"},
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	want := store.Advisory{
		ID:      "MAL-2025-46969",
		Kind:    store.KindMalicious,
		Summary: "Malicious code in chalk (npm)",
	}
	if len(got) != 1 || len(got[0]) != 1 || got[0][0] != want {
		t.Fatalf("chalk@5.6.1 advisories = %+v, want [%+v]", got, want)
	}
}

// TestCheckKeepsFindingWhenRecordFetchFails: one GET /v1/vulns/{id} fails. The
// check still succeeds, that finding keeps its id (and its kind, which the id
// alone decides), and the other finding comes back intact.
func TestCheckKeepsFindingWhenRecordFetchFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/querybatch", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"results":[
			{"vulns":[{"id":"MAL-2099-1"}]},
			{"vulns":[{"id":"GHSA-xxxx"}]}
		]}`))
	})
	mux.HandleFunc("GET /v1/vulns/MAL-2099-1", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream timeout", http.StatusBadGateway)
	})
	mux.HandleFunc("GET /v1/vulns/GHSA-xxxx", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"id":"GHSA-xxxx","summary":"left-pad pads left",
			"database_specific":{"severity":"HIGH"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{
		{Name: "evil", Ecosystem: "npm", Version: "1.0.0"},
		{Name: "left-pad", Ecosystem: "npm", Version: "1.2.0"},
	})
	if err != nil {
		t.Fatalf("Check aborted on one failed record fetch: %v", err)
	}
	failed := store.Advisory{ID: "MAL-2099-1", Kind: store.KindMalicious}
	if len(got[0]) != 1 || got[0][0] != failed {
		t.Errorf("unfetched finding = %+v, want [%+v]", got[0], failed)
	}
	intact := store.Advisory{
		ID: "GHSA-xxxx", Kind: store.KindVuln,
		Summary: "left-pad pads left", Severity: "HIGH",
	}
	if len(got[1]) != 1 || got[1][0] != intact {
		t.Errorf("other finding = %+v, want [%+v]", got[1], intact)
	}
}

// pagingStub answers querybatch from pages, one response body per request in
// order (the last one repeats), and records each request's queries. Every vuln
// id resolves to a bare record. requests returns what was recorded so far.
func pagingStub(
	t *testing.T,
	pages ...string,
) (srv *httptest.Server, requests func() [][]batchQuery) {
	t.Helper()
	var (
		mu   sync.Mutex
		seen [][]batchQuery
	)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/querybatch", func(w http.ResponseWriter, r *http.Request) {
		var req batchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode querybatch: %v", err)
		}
		mu.Lock()
		seen = append(seen, req.Queries)
		n := len(seen)
		mu.Unlock()
		_, _ = w.Write([]byte(pages[min(n, len(pages))-1]))
	})
	mux.HandleFunc("GET /v1/vulns/{id}", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"id":"` + r.PathValue("id") + `"}`))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, func() [][]batchQuery {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(seen)
	}
}

func ids(advs []store.Advisory) []string {
	out := make([]string, 0, len(advs))
	for _, a := range advs {
		out = append(out, a.ID)
	}
	return out
}

// TestCheckFollowsNextPageToken: the first query's result is paged, so the
// second request repeats only that query, carrying its token, and both pages'
// ids land on it.
func TestCheckFollowsNextPageToken(t *testing.T) {
	srv, requests := pagingStub(t,
		`{"results":[
			{"vulns":[{"id":"GHSA-1"}],"next_page_token":"page-2"},
			{"vulns":[{"id":"GHSA-9"}]}
		]}`,
		`{"results":[{"vulns":[{"id":"GHSA-2"}]}]}`,
	)

	got, err := NewWithURL(srv.URL).Check(context.Background(), []Query{
		{Name: "big", Ecosystem: "npm", Version: "1.0.0"},
		{Name: "small", Ecosystem: "npm", Version: "1.0.0"},
	})
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if want := []string{"GHSA-1", "GHSA-2"}; !slices.Equal(ids(got[0]), want) {
		t.Errorf("paged query ids = %v, want %v", ids(got[0]), want)
	}
	if want := []string{"GHSA-9"}; !slices.Equal(ids(got[1]), want) {
		t.Errorf("unpaged query ids = %v, want %v", ids(got[1]), want)
	}
	reqs := requests()
	if len(reqs) != 2 {
		t.Fatalf("querybatch requests = %d, want 2", len(reqs))
	}
	second := reqs[1]
	if len(second) != 1 || second[0].Package.Name != "big" || second[0].PageToken != "page-2" {
		t.Errorf("second request = %+v, want only big with page_token page-2", second)
	}
}

// TestCheckStopsEndlessPaging: a server that never stops handing out page
// tokens fails the check instead of holding the scan forever.
func TestCheckStopsEndlessPaging(t *testing.T) {
	srv, requests := pagingStub(t, `{"results":[{"vulns":[],"next_page_token":"again"}]}`)

	_, err := NewWithURL(srv.URL).Check(context.Background(), []Query{
		{Name: "loop", Ecosystem: "npm", Version: "1.0.0"},
	})
	if err == nil {
		t.Fatal("Check succeeded against a server that pages forever, want an error")
	}
	if n := len(requests()); n != maxPages {
		t.Errorf("querybatch requests = %d, want the %d-request cap", n, maxPages)
	}
}

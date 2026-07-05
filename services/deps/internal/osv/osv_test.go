package osv

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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

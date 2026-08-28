package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/prs/internal/config"
	"github.com/mad01/thismoon/services/prs/internal/poller"
	"github.com/mad01/thismoon/services/prs/internal/store"
)

var base = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

type fakeRefresher struct {
	sum poller.Summary
	err error
}

func (f *fakeRefresher) Refresh(context.Context) (poller.Summary, error) { return f.sum, f.err }

func newTestServer(t *testing.T, refresher Refresher) *httptest.Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := []store.RepoState{
		{
			Host: "github.com", Repo: "org/alpha", FetchedAt: base,
			PRs: []store.PR{
				{
					Host: "github.com", Repo: "org/alpha", Number: 1, Author: "ada",
					CreatedAt: base.Add(-2 * time.Hour),
				},
				{
					Host: "github.com", Repo: "org/alpha", Number: 2, Author: "bo",
					CreatedAt: base.Add(-1 * time.Hour), ReviewDecision: "APPROVED",
				},
			},
		},
	}
	if err := st.SetRepos(seed, base); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{PollInterval: 5 * time.Minute, Path: "/tmp/config.yaml"}
	ts := httptest.NewServer(New(st, refresher, cfg, buildinfo.Info{}).Handler())
	t.Cleanup(ts.Close)
	return ts
}

func getJSON(t *testing.T, url string, out any) int {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return res.StatusCode
}

func TestListReturnsPRsAndFacets(t *testing.T) {
	ts := newTestServer(t, &fakeRefresher{})
	var out listResponse
	if code := getJSON(t, ts.URL+"/api/prs", &out); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(out.PRs) != 2 || out.PRs[0].Number != 2 {
		t.Errorf("PRs = %+v", out.PRs)
	}
	if len(out.Facets.Repos) != 1 || len(out.Facets.Authors) != 2 {
		t.Errorf("Facets = %+v", out.Facets)
	}
	if out.Status.OpenPRs != 2 {
		t.Errorf("Status = %+v", out.Status)
	}
}

func TestListFilterParams(t *testing.T) {
	ts := newTestServer(t, &fakeRefresher{})
	var out listResponse
	if code := getJSON(t, ts.URL+"/api/prs?author=ada&sort=oldest", &out); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(out.PRs) != 1 || out.PRs[0].Number != 1 {
		t.Errorf("filtered PRs = %+v", out.PRs)
	}
	if code := getJSON(t, ts.URL+"/api/prs?review=APPROVED", &out); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if len(out.PRs) != 1 || out.PRs[0].Number != 2 {
		t.Errorf("review-filtered PRs = %+v", out.PRs)
	}
}

func TestListRejectsBadParams(t *testing.T) {
	ts := newTestServer(t, &fakeRefresher{})
	if code := getJSON(t, ts.URL+"/api/prs?sort=sideways", nil); code != http.StatusBadRequest {
		t.Errorf("bad sort: status %d, want 400", code)
	}
	if code := getJSON(t, ts.URL+"/api/prs?review=maybe", nil); code != http.StatusBadRequest {
		t.Errorf("bad review: status %d, want 400", code)
	}
}

func TestRefreshReportsSummary(t *testing.T) {
	ts := newTestServer(t, &fakeRefresher{
		sum: poller.Summary{Repos: 3, OpenPRs: 2, Errors: 1, Duration: 1500 * time.Millisecond},
	})
	res, err := http.Post(ts.URL+"/api/refresh", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	var out refreshResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Repos != 3 || out.DurationMS != 1500 {
		t.Errorf("refresh response = %+v", out)
	}
}

func TestRefreshErrorIs502(t *testing.T) {
	ts := newTestServer(t, &fakeRefresher{err: errors.New("boom")})
	res, err := http.Post(ts.URL+"/api/refresh", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusBadGateway {
		t.Errorf("status %d, want 502", res.StatusCode)
	}
}

func TestStatusIncludesConfig(t *testing.T) {
	ts := newTestServer(t, &fakeRefresher{})
	var out statusResponse
	if code := getJSON(t, ts.URL+"/api/status", &out); code != http.StatusOK {
		t.Fatalf("status %d", code)
	}
	if out.PollInterval != "5m0s" || out.ConfigPath != "/tmp/config.yaml" {
		t.Errorf("status response = %+v", out)
	}
}

func TestHealthz(t *testing.T) {
	ts := newTestServer(t, &fakeRefresher{})
	if code := getJSON(t, ts.URL+"/healthz", nil); code != http.StatusNoContent {
		t.Errorf("healthz = %d, want 204", code)
	}
}

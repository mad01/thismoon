package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// fakeHost serves the two REST endpoints OpenPRs touches and records the
// Authorization headers it saw.
func fakeHost(t *testing.T, pulls, reviews string, auth *[]string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(
		"GET /api/v3/repos/org/alpha/pulls",
		func(w http.ResponseWriter, r *http.Request) {
			*auth = append(*auth, r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, pulls)
		},
	)
	mux.HandleFunc(
		"GET /api/v3/repos/org/alpha/pulls/{n}/reviews",
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, reviews)
		},
	)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)
	return ts
}

// fakeTokenSource builds a TokenSource with an injected mint function.
func fakeTokenSource(mint func(host string) (string, error)) *TokenSource {
	return &TokenSource{
		mint:     mint,
		now:      time.Now,
		tokens:   map[string]string{},
		failures: map[string]mintFailure{},
	}
}

// testClient builds a Client whose fake host resolves to ts and whose mint
// function returns canned tokens, counting calls.
func testClient(ts *httptest.Server, tokens []string, minted *int) *Client {
	src := fakeTokenSource(func(string) (string, error) {
		tok := tokens[min(*minted, len(tokens)-1)]
		*minted++
		return tok, nil
	})
	c := NewClient(src)
	c.apiBase = func(string) string { return ts.URL + "/api/v3/" }
	return c
}

const pullsBody = `[
  {"number": 1, "title": "one", "state": "open", "draft": false,
   "html_url": "https://example.test/org/alpha/pull/1",
   "user": {"login": "ada"},
   "labels": [{"name": "bug"}],
   "created_at": "2026-08-01T10:00:00Z", "updated_at": "2026-08-02T10:00:00Z"},
  {"number": 2, "title": "draft", "state": "open", "draft": true,
   "user": {"login": "bo"},
   "created_at": "2026-08-01T11:00:00Z", "updated_at": "2026-08-01T11:00:00Z"}
]`

func TestOpenPRsMapsAndDropsDrafts(t *testing.T) {
	var auth []string
	reviews := `[
	  {"state": "CHANGES_REQUESTED", "user": {"login": "rev"}},
	  {"state": "APPROVED", "user": {"login": "rev"}},
	  {"state": "COMMENTED", "user": {"login": "other"}}
	]`
	ts := fakeHost(t, pullsBody, reviews, &auth)
	minted := 0
	c := testClient(ts, []string{"tok1"}, &minted)

	prs, err := c.OpenPRs(context.Background(), "example.test", "org", "alpha")
	if err != nil {
		t.Fatalf("OpenPRs: %v", err)
	}
	if len(prs) != 1 {
		t.Fatalf("got %d PRs, want 1 (draft dropped)", len(prs))
	}
	pr := prs[0]
	if pr.Number != 1 || pr.Author != "ada" || pr.Repo != "org/alpha" ||
		pr.Host != "example.test" {
		t.Errorf("mapped PR = %+v", pr)
	}
	if len(pr.Labels) != 1 || pr.Labels[0] != "bug" {
		t.Errorf("Labels = %v", pr.Labels)
	}
	// rev's later APPROVED supersedes their CHANGES_REQUESTED; the comment
	// does not vote.
	if pr.ReviewDecision != "APPROVED" {
		t.Errorf("ReviewDecision = %q, want APPROVED", pr.ReviewDecision)
	}
	if len(auth) == 0 || auth[0] != "Bearer tok1" {
		t.Errorf("auth headers = %v", auth)
	}
	if minted != 1 {
		t.Errorf("minted %d tokens, want 1", minted)
	}
}

func TestChangesRequestedWinsAcrossReviewers(t *testing.T) {
	var auth []string
	reviews := `[
	  {"state": "APPROVED", "user": {"login": "a"}},
	  {"state": "CHANGES_REQUESTED", "user": {"login": "b"}}
	]`
	ts := fakeHost(t, pullsBody, reviews, &auth)
	minted := 0
	c := testClient(ts, []string{"tok1"}, &minted)

	prs, err := c.OpenPRs(context.Background(), "example.test", "org", "alpha")
	if err != nil {
		t.Fatalf("OpenPRs: %v", err)
	}
	if prs[0].ReviewDecision != "CHANGES_REQUESTED" {
		t.Errorf("ReviewDecision = %q, want CHANGES_REQUESTED", prs[0].ReviewDecision)
	}
}

func TestUnauthorizedRemintsOnce(t *testing.T) {
	var auth []string
	mux := http.NewServeMux()
	mux.HandleFunc(
		"GET /api/v3/repos/org/alpha/pulls",
		func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("Authorization")
			auth = append(auth, got)
			if got != "Bearer good" {
				w.WriteHeader(http.StatusUnauthorized)
				fmt.Fprint(w, `{"message": "Bad credentials"}`)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `[]`)
		},
	)
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	minted := 0
	c := testClient(ts, []string{"bad", "good"}, &minted)
	if _, err := c.OpenPRs(context.Background(), "example.test", "org", "alpha"); err != nil {
		t.Fatalf("OpenPRs after re-mint: %v", err)
	}
	if minted != 2 {
		t.Errorf("minted %d tokens, want 2 (bad then good)", minted)
	}
	if len(auth) != 2 || auth[1] != "Bearer good" {
		t.Errorf("auth sequence = %v", auth)
	}
}

func TestTokenSourceCachesAndInvalidates(t *testing.T) {
	minted := 0
	src := fakeTokenSource(func(host string) (string, error) {
		minted++
		return fmt.Sprintf("%s-tok%d", host, minted), nil
	})
	t1, err := src.Token("h1")
	if err != nil {
		t.Fatal(err)
	}
	t2, err := src.Token("h1")
	if err != nil {
		t.Fatal(err)
	}
	if t1 != t2 || minted != 1 {
		t.Errorf("token not cached: %q %q, minted %d", t1, t2, minted)
	}
	src.Invalidate("h1")
	t3, err := src.Token("h1")
	if err != nil {
		t.Fatal(err)
	}
	if t3 == t1 || minted != 2 {
		t.Errorf("invalidate did not force a re-mint: %q, minted %d", t3, minted)
	}
}

func TestTokenSourceCachesMintFailures(t *testing.T) {
	minted := 0
	src := fakeTokenSource(func(string) (string, error) {
		minted++
		return "", fmt.Errorf("not logged in")
	})
	clock := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	src.now = func() time.Time { return clock }

	for range 5 {
		if _, err := src.Token("h1"); err == nil {
			t.Fatal("expected mint failure")
		}
	}
	if minted != 1 {
		t.Errorf("minted %d times within TTL, want 1", minted)
	}

	clock = clock.Add(mintFailureTTL + time.Second)
	if _, err := src.Token("h1"); err == nil {
		t.Fatal("expected mint failure after TTL")
	}
	if minted != 2 {
		t.Errorf("minted %d times after TTL, want 2", minted)
	}
}

package regcheck

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// reply is one canned registry response.
type reply struct {
	status int
	body   string
}

// fixture reads a captured registry response from testdata.
func fixture(t *testing.T, name string) string {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// registryStub serves routes, keyed by escaped request path, as all three
// registries: npm under /npm, PyPI under /pypi, the Go proxy under /proxy. A
// request for any other path fails the test. It counts the requests per path.
func registryStub(t *testing.T, routes map[string]reply) (Endpoints, map[string]int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.EscapedPath()
		mu.Lock()
		hits[p]++
		mu.Unlock()
		rep, ok := routes[p]
		if !ok {
			t.Errorf("unexpected request %s", p)
			http.Error(w, "no route", http.StatusTeapot)
			return
		}
		w.WriteHeader(rep.status)
		_, _ = w.Write([]byte(rep.body))
	}))
	t.Cleanup(srv.Close)
	return Endpoints{
		NPM:     srv.URL + "/npm",
		PyPI:    srv.URL + "/pypi",
		GoProxy: srv.URL + "/proxy",
	}, hits
}

// liveRoutes are the captured responses from the ticket (chalk 5.6.1,
// ultralytics 8.3.41, a yanked requests release, boltdb-go/bolt, grpc's
// retract), plus hand-written ones for shapes no live package shows.
func liveRoutes(t *testing.T) map[string]reply {
	ok := func(body string) reply { return reply{http.StatusOK, body} }
	notFound := reply{http.StatusNotFound, `{"error":"Not found"}`}
	active := ok(`{"project-status":{"status":"active"},"versions":[]}`)
	return map[string]reply{
		// npm: chalk 5.6.1 is gone from versions but still in time.
		"/npm/chalk/5.6.1":             {http.StatusNotFound, fixture(t, "npm-chalk-5.6.1.json")},
		"/npm/chalk":                   ok(fixture(t, "npm-chalk.json")),
		"/npm/chalk/5.6.2":             ok(`{"name":"chalk","version":"5.6.2"}`),
		"/npm/request/2.88.2":          ok(fixture(t, "npm-request-2.88.2.json")),
		"/npm/private-pkg/1.0.0":       notFound,
		"/npm/private-pkg":             notFound,
		"/npm/@types%2Fprismjs/1.26.6": ok(`{"name":"@types/prismjs","version":"1.26.6"}`),
		"/pypi/simple/ultralytics/":    ok(fixture(t, "pypi-ultralytics-simple.json")),
		"/pypi/pypi/ultralytics/8.3.41/json": {
			http.StatusNotFound,
			fixture(t, "pypi-ultralytics-8.3.41.json"),
		},
		"/pypi/pypi/ultralytics/8.3.40/json": ok(`{"info":{"yanked":false,"yanked_reason":null}}`),
		"/pypi/simple/requests/":             active,
		"/pypi/pypi/requests/2.32.0/json":    ok(fixture(t, "pypi-requests-2.32.0.json")),
		"/pypi/simple/torch/":                active,
		"/pypi/pypi/torch/2.1.0+cu118/json":  notFound,
		"/pypi/simple/evil-pkg/": ok(
			`{"project-status":{"status":"quarantined","reason":"reported as malware"}}`,
		),
		"/pypi/simple/old-pkg/":       ok(`{"project-status":{"status":"archived"}}`),
		"/pypi/pypi/old-pkg/1.0/json": ok(`{"info":{"yanked":false}}`),
		"/pypi/simple/internal-pkg/":  notFound,
		"/proxy/github.com/boltdb-go/bolt/@latest": {
			http.StatusForbidden,
			fixture(t, "go-boltdb-go-bolt-403.txt"),
		},
		"/proxy/google.golang.org/grpc/@latest":        ok(`{"Version":"v1.85.0"}`),
		"/proxy/google.golang.org/grpc/@v/v1.85.0.mod": ok(fixture(t, "go-grpc-v1.85.0.mod")),
		"/proxy/github.com/!burnt!sushi/toml/@latest":  ok(`{"Version":"v1.5.0"}`),
		"/proxy/github.com/!burnt!sushi/toml/@v/v1.5.0.mod": ok(
			"module github.com/BurntSushi/toml\n",
		),
		"/proxy/example.com/gone/@latest": {http.StatusGone, "not found"},
	}
}

func TestCheckReadsEachRegistry(t *testing.T) {
	e, hits := registryStub(t, liveRoutes(t))
	c := NewWithEndpoints(e)
	c.noProxy = func(context.Context) (string, error) { return "corp.example/*", nil }

	type want struct {
		id      string
		kind    store.Kind
		summary string // a substring of the finding's summary
	}
	cases := []struct {
		q    osv.Query
		want []want
	}{
		{
			osv.Query{Ecosystem: "npm", Name: "chalk", Version: "5.6.1"},
			[]want{{RuleNPMWithdrawn, store.KindMalicious, "published 2025-09-08"}},
		},
		{osv.Query{Ecosystem: "npm", Name: "chalk", Version: "5.6.2"}, nil},
		{
			osv.Query{Ecosystem: "npm", Name: "request", Version: "2.88.2"},
			[]want{{RuleNPMDeprecated, store.KindSignal, "request has been deprecated"}},
		},
		{osv.Query{Ecosystem: "npm", Name: "private-pkg", Version: "1.0.0"}, nil},
		{osv.Query{Ecosystem: "npm", Name: "@types/prismjs", Version: "1.26.6"}, nil},
		{
			osv.Query{Ecosystem: "PyPI", Name: "ultralytics", Version: "8.3.41"},
			[]want{{RulePyPIWithdrawn, store.KindMalicious, "no longer serves version 8.3.41"}},
		},
		{osv.Query{Ecosystem: "PyPI", Name: "ultralytics", Version: "8.3.40"}, nil},
		{
			osv.Query{Ecosystem: "PyPI", Name: "requests", Version: "2.32.0"},
			[]want{{RulePyPIYanked, store.KindSignal, "CVE-2024-35195"}},
		},
		{osv.Query{Ecosystem: "PyPI", Name: "torch", Version: "2.1.0+cu118"}, nil},
		{
			osv.Query{Ecosystem: "PyPI", Name: "evil-pkg", Version: "1.0"},
			[]want{{RulePyPIQuarantined, store.KindMalicious, "reported as malware"}},
		},
		{
			osv.Query{Ecosystem: "PyPI", Name: "old-pkg", Version: "1.0"},
			[]want{{RulePyPIArchived, store.KindSignal, "archived"}},
		},
		{osv.Query{Ecosystem: "PyPI", Name: "internal-pkg", Version: "1.0"}, nil},
		{
			osv.Query{Ecosystem: "Go", Name: "github.com/boltdb-go/bolt", Version: "1.3.1"},
			[]want{{RuleGoBlocked, store.KindMalicious, "considers it malicious"}},
		},
		{
			osv.Query{Ecosystem: "Go", Name: "google.golang.org/grpc", Version: "1.74.1"},
			[]want{{RuleGoRetracted, store.KindSignal, "published prematurely"}},
		},
		{osv.Query{Ecosystem: "Go", Name: "google.golang.org/grpc", Version: "1.84.0"}, nil},
		{osv.Query{Ecosystem: "Go", Name: "github.com/BurntSushi/toml", Version: "1.5.0"}, nil},
		{osv.Query{Ecosystem: "Go", Name: "example.com/gone", Version: "1.0.0"}, nil},
		{osv.Query{Ecosystem: "Go", Name: "corp.example/private", Version: "1.0.0"}, nil},
		{osv.Query{Ecosystem: "SwiftURL", Name: "github.com/jpsim/Yams", Version: "5.4.0"}, nil},
		// A second row for a version already asked about costs no request.
		{
			osv.Query{Ecosystem: "npm", Name: "chalk", Version: "5.6.1"},
			[]want{{RuleNPMWithdrawn, store.KindMalicious, "npm no longer serves chalk@5.6.1"}},
		},
	}
	queries := make([]osv.Query, len(cases))
	for i, tc := range cases {
		queries[i] = tc.q
	}

	got := c.Check(context.Background(), queries)

	if len(got) != len(cases) {
		t.Fatalf("got %d results, want %d", len(got), len(cases))
	}
	for i, tc := range cases {
		name := tc.q.Ecosystem + " " + tc.q.Name + "@" + tc.q.Version
		r := got[i]
		if r.Err != nil {
			t.Errorf("%s: unknown (%v), want an answer", name, r.Err)
			continue
		}
		if len(r.Findings) != len(tc.want) {
			t.Errorf("%s: findings = %+v, want %+v", name, r.Findings, tc.want)
			continue
		}
		for j, w := range tc.want {
			f := r.Findings[j]
			if f.ID != w.id || f.Kind != w.kind || !strings.Contains(f.Summary, w.summary) {
				t.Errorf(
					"%s: finding = %+v, want %s (%s) with %q",
					name,
					f,
					w.id,
					w.kind,
					w.summary,
				)
			}
		}
	}
	for p, n := range hits {
		if n != 1 {
			t.Errorf("%s requested %d times, want once per check", p, n)
		}
	}
	if hits["/npm/private-pkg"] != 1 || hits["/npm/chalk"] != 1 {
		t.Errorf(
			"hits = %v, want the package document fetched for each missing version's package",
			hits,
		)
	}
}

// TestCheckUnansweredLookupsAreUnknown: a registry that errors, a 403 that is
// not the Go proxy's security error (a sandbox refusing the host), an
// unreachable host, and a failed go env all come back unknown, never clean and
// never as a finding.
func TestCheckUnansweredLookupsAreUnknown(t *testing.T) {
	e, _ := registryStub(t, map[string]reply{
		"/npm/left-pad/1.3.0": {http.StatusServiceUnavailable, "busy"},
		"/proxy/github.com/x/y/@latest": {
			http.StatusForbidden,
			"Forbidden: host not allowed",
		},
		"/pypi/simple/flask/":                 {http.StatusTooManyRequests, "slow down"},
		"/proxy/github.com/x/z/@latest":       {http.StatusOK, `{"Version":"v1.2.0"}`},
		"/proxy/github.com/x/z/@v/v1.2.0.mod": {http.StatusInternalServerError, "oops"},
	})
	answered := NewWithEndpoints(e)

	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachable.Close()
	down := NewWithEndpoints(Endpoints{
		NPM: unreachable.URL, PyPI: unreachable.URL, GoProxy: unreachable.URL,
	})

	noGo := NewWithEndpoints(e)
	noGo.noProxy = func(context.Context) (string, error) {
		return "", errors.New(`go env GONOPROXY: exec: "go": executable file not found`)
	}

	cases := []struct {
		name string
		c    *Client
		q    osv.Query
	}{
		{"npm 503", answered, osv.Query{Ecosystem: "npm", Name: "left-pad", Version: "1.3.0"}},
		{
			"plain 403",
			answered,
			osv.Query{Ecosystem: "Go", Name: "github.com/x/y", Version: "1.0.0"},
		},
		{"PyPI 429", answered, osv.Query{Ecosystem: "PyPI", Name: "flask", Version: "3.0.0"}},
		{
			"go.mod 500",
			answered,
			osv.Query{Ecosystem: "Go", Name: "github.com/x/z", Version: "1.1.0"},
		},
		{"npm down", down, osv.Query{Ecosystem: "npm", Name: "chalk", Version: "5.6.1"}},
		{"PyPI down", down, osv.Query{Ecosystem: "PyPI", Name: "requests", Version: "2.32.0"}},
		{"proxy down", down, osv.Query{Ecosystem: "Go", Name: "github.com/x/y", Version: "1.0.0"}},
		{"no go env", noGo, osv.Query{Ecosystem: "Go", Name: "github.com/x/y", Version: "1.0.0"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.c.Check(context.Background(), []osv.Query{tc.q})
			if len(got) != 1 || got[0].Err == nil || got[0].Findings != nil {
				t.Errorf("result = %+v, want unknown with no findings", got)
			}
		})
	}
}

func TestEveryRuleIDIsAKeySafeDepsID(t *testing.T) {
	for id, kind := range ruleKinds {
		if !strings.HasPrefix(id, "deps-") || strings.Contains(id, ":") {
			t.Errorf("rule id %q needs a deps- prefix and no ':'", id)
		}
		if kind != store.KindMalicious && kind != store.KindSignal {
			t.Errorf("rule %s has kind %q, want malicious or signal", id, kind)
		}
		if !IsRule(id) {
			t.Errorf("IsRule(%q) = false", id)
		}
	}
	if IsRule("MAL-2025-46969") || IsRule("deps-movable-ref") {
		t.Error("IsRule claims an id another producer owns")
	}
}

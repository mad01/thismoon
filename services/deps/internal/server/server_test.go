package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mad01/thismoon/buildinfo"
	"github.com/mad01/thismoon/services/deps/internal/alert"
	"github.com/mad01/thismoon/services/deps/internal/discover"
	"github.com/mad01/thismoon/services/deps/internal/osv"
	"github.com/mad01/thismoon/services/deps/internal/scanner"
	"github.com/mad01/thismoon/services/deps/internal/store"
)

// testInfo is the build metadata the test server reports on /version.
var testInfo = buildinfo.Info{
	Version:   "test-sha",
	Commit:    "0123456789abcdef0123456789abcdef01234567",
	Tag:       "deps/v0.0.0",
	BuildTime: "2026-08-13T09:00:00Z",
}

// testServer builds a Server over a temp store seeded with one flagged dep and
// one clean dep. The engine is nil — the page/API/version routes under test
// never reach it (only the POST scan/check routes do).
func testServer(t *testing.T) *Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]store.Dependency{
		{
			Ecosystem: "Go", Name: "golang.org/x/net", Version: "0.17.0",
			Repo: "/home/u/code/widget", Direct: true, Imported: true,
			Advisories: []store.Advisory{
				{ID: "GO-2024-0001", Summary: "header smuggling", Severity: "high", FixedVersion: "0.23.0"},
			},
		},
		{Ecosystem: "npm", Name: "lodash", Version: "4.17.21", Repo: "/home/u/code/widget", Direct: true, Imported: true},
	}, nil); err != nil {
		t.Fatal(err)
	}
	return New(st, nil, testInfo)
}

func TestPageServesShell(t *testing.T) {
	h := testServer(t).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))

	if rec.Code != 200 {
		t.Fatalf("GET / = %d", rec.Code)
	}
	body := rec.Body.String()
	// The page is a static shell: chrome + an empty mount point + the client
	// scripts. The body is rendered in the browser, so no dep data inlined.
	// The chrome links the themes page webkit serves.
	for _, want := range []string{
		`id="app"`, "/app.js", "/webkit/webkit.js",
		`<a data-nav href="/webkit/themes">Themes</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("shell missing %q", want)
		}
	}
	for _, absent := range []string{"golang.org/x/net", "GO-2024-0001", "Active findings"} {
		if strings.Contains(body, absent) {
			t.Errorf("shell should not inline dep data, found %q", absent)
		}
	}
}

func TestAppJS(t *testing.T) {
	h := testServer(t).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/app.js", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /app.js = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	if !strings.Contains(rec.Body.String(), "/api/deps") {
		t.Error("app.js should fetch /api/deps")
	}
}

func TestAPIDeps(t *testing.T) {
	h := testServer(t).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/deps", nil))

	if rec.Code != 200 {
		t.Fatalf("GET /api/deps = %d", rec.Code)
	}
	var got struct {
		ScannedAt    string `json:"scanned_at"`
		Total        int    `json:"total"`
		FlaggedCount int    `json:"flagged_count"`
		Deps         []struct {
			Ecosystem  string `json:"ecosystem"`
			Name       string `json:"name"`
			Version    string `json:"version"`
			Repo       string `json:"repo"`
			Direct     bool   `json:"direct"`
			Advisories []struct {
				ID           string `json:"id"`
				FixedVersion string `json:"fixed_version"`
				Key          string `json:"key"`
				Resolved     bool   `json:"resolved"`
			} `json:"advisories"`
		} `json:"deps"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.FlaggedCount != 1 {
		t.Errorf("api/deps = total %d / flagged %d, want 2 / 1", got.Total, got.FlaggedCount)
	}
	if len(got.Deps) != 2 {
		t.Fatalf("api/deps returned %d deps, want 2", len(got.Deps))
	}

	// app.js renders from these field names, so the JSON shape is the contract.
	body := rec.Body.String()
	for _, want := range []string{
		`"scanned_at"`, `"total"`, `"flagged_count"`, `"deps"`,
		`"ecosystem"`, `"name"`, `"version"`, `"repo"`, `"direct"`,
		`"advisories"`, `"id"`, `"fixed_version"`, `"key"`, `"resolved"`,
		`"imported"`, `"kind"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("api/deps JSON missing field %s", want)
		}
	}
}

// TestAPIFlaggedByKind: GET /api/flagged lists the malicious finding first and
// carries every advisory's kind. flagged_count takes the malicious finding on a
// dep that isn't compiled in and the compiled-in vulnerability, never the signal.
func TestAPIFlaggedByKind(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]store.Dependency{
		{
			Ecosystem: "Go", Name: "golang.org/x/net", Version: "0.17.0",
			Repo: "/home/u/code/widget", Imported: true,
			Advisories: []store.Advisory{{ID: "GO-2024-0001", Severity: "HIGH"}},
		},
		{
			Ecosystem: "npm", Name: "left-pad", Version: "1.2.0",
			Repo: "/home/u/code/widget", Imported: true,
			Advisories: []store.Advisory{{ID: "deps-movable-ref", Kind: store.KindSignal}},
		},
		{
			Ecosystem: "npm", Name: "chalk", Version: "5.6.1",
			Repo: "/home/u/code/widget", Imported: false,
			Advisories: []store.Advisory{{ID: "MAL-2025-46969", Kind: store.KindMalicious}},
		},
	}, nil); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	New(st, nil, testInfo).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/api/flagged", nil))

	var got struct {
		FlaggedCount int `json:"flagged_count"`
		Flagged      []struct {
			Name       string `json:"name"`
			Advisories []struct {
				Kind string `json:"kind"`
			} `json:"advisories"`
		} `json:"flagged"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.FlaggedCount != 2 {
		t.Errorf("flagged_count = %d, want 2 (malicious + vuln, not the signal)", got.FlaggedCount)
	}
	var kinds []string
	for _, d := range got.Flagged {
		kinds = append(kinds, d.Name+"="+d.Advisories[0].Kind)
	}
	want := "chalk=malicious golang.org/x/net=vuln left-pad=signal"
	if strings.Join(kinds, " ") != want {
		t.Errorf("flagged = %v, want %s", kinds, want)
	}
}

// TestAPIDepsExcludesNonImportedFromFlagged confirms a flagged-but-not-imported
// (graph-only transitive) dep does not count toward flagged_count — it isn't a
// real exposure, so it's informational only.
func TestAPIDepsExcludesNonImportedFromFlagged(t *testing.T) {
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Save([]store.Dependency{{
		Ecosystem: "Go", Name: "github.com/graph/only", Version: "1.0.0",
		Repo: "/home/u/code/widget", Imported: false,
		Advisories: []store.Advisory{{ID: "GO-2024-9999", FixedVersion: "1.1.0"}},
	}}, nil); err != nil {
		t.Fatal(err)
	}
	h := New(st, nil, testInfo).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/deps", nil))

	var got struct {
		Total        int `json:"total"`
		FlaggedCount int `json:"flagged_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 {
		t.Errorf("total = %d, want 1", got.Total)
	}
	if got.FlaggedCount != 0 {
		t.Errorf("flagged_count = %d, want 0 (graph-only dep excluded)", got.FlaggedCount)
	}
}

// recordingNotifier records each banner title instead of shelling out to
// osascript.
type recordingNotifier struct{ titles []string }

func (n *recordingNotifier) Notify(title, _ string) error {
	n.titles = append(n.titles, title)
	return nil
}

// stubEco discovers a fixed set of packages in every repo.
type stubEco struct{ pkgs []discover.Package }

func (stubEco) Name() string { return "npm" }

func (s stubEco) Discover(string, discover.Options) ([]discover.Package, error) {
	return s.pkgs, nil
}

// malChecker reports one MAL- finding for chalk and nothing else.
type malChecker struct{}

func (malChecker) Check(_ context.Context, queries []osv.Query) ([][]store.Advisory, error) {
	out := make([][]store.Advisory, len(queries))
	for i, q := range queries {
		if q.Name == "chalk" {
			out[i] = []store.Advisory{{ID: "MAL-2025-46969", Kind: store.KindMalicious}}
		}
	}
	return out, nil
}

// chalkEngine is an engine over st whose one repo pins chalk@5.6.1, which
// malChecker flags as malicious, delivering banners through n. It also scans
// the installed tools mise points at; the zero Mise scans none.
func chalkEngine(st *store.Store, n alert.Notifier, mise discover.Mise) *scanner.Engine {
	return &scanner.Engine{
		Prepare: func() (scanner.Plan, error) {
			return scanner.Plan{Repos: []string{"/home/u/code/widget"}, Mise: mise}, nil
		},
		Ecosystems: []discover.Ecosystem{stubEco{pkgs: []discover.Package{
			{Ecosystem: "npm", Name: "chalk", Version: "5.6.1", Imported: true},
		}}},
		Checker:  malChecker{},
		Notifier: n,
		Store:    st,
	}
}

// chalkTool writes a mise install whose one tool is npm:chalk@5.6.1 and
// returns the Mise that reads it.
func chalkTool(t *testing.T) discover.Mise {
	t.Helper()
	dir := t.TempDir()
	version := filepath.Join(dir, "installs", "npm-chalk", "5.6.1")
	if err := os.MkdirAll(version, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		filepath.Join(dir, "mise.lock"): "[[tools.\"npm:chalk\"]]\nversion = \"5.6.1\"\n",
		filepath.Join(version, "aube-lock.yaml"): "lockfileVersion: '9.0'\nimporters:\n  .:\n" +
			"    dependencies:\n      chalk: {version: 5.6.1}\npackages:\n  chalk@5.6.1: {}\n",
	}
	for path, body := range files {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return discover.Mise{
		LockPath:    filepath.Join(dir, "mise.lock"),
		InstallRoot: filepath.Join(dir, "installs"),
	}
}

// TestManualCheckNotifiesOnce: a manual check that finds a new malicious
// finding fires its banner, on the full, the per-repo and the per-tool path
// alike, and a second check does not fire it again. A manual check resets the
// scan time, so the loop would skip it and no banner would ever fire.
func TestManualCheckNotifiesOnce(t *testing.T) {
	paths := []string{
		"/api/check",
		"/api/check?repo=widget",
		"/api/check?repo=" + url.QueryEscape("mise:npm:chalk"),
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			st, err := store.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			n := &recordingNotifier{}
			h := New(st, chalkEngine(st, n, chalkTool(t)), testInfo).Handler()
			for range 2 {
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, httptest.NewRequest("POST", path, nil))
				if rec.Code != 200 {
					t.Fatalf("POST %s = %d: %s", path, rec.Code, rec.Body)
				}
			}
			want := []string{"deps: malicious package chalk@5.6.1"}
			if !slices.Equal(n.titles, want) {
				t.Errorf("banners = %q, want %q once", n.titles, want)
			}
		})
	}
}

// notifierFunc delivers each banner by calling the function, in place of
// osascript.
type notifierFunc func() error

func (f notifierFunc) Notify(string, string) error { return f() }

// TestManualCheckKeepsResultWhenNotifyFails: the check is saved before the
// banners fire, so a failed banner or a failed write of the notified set still
// answers 200 with the flagged list. A 500 would show "Scan failed" and hide
// the finding.
func TestManualCheckKeepsResultWhenNotifyFails(t *testing.T) {
	cases := map[string]func(workdir string) error{
		"notifier fails": func(string) error { return errors.New("osascript still running") },
		// The banner goes out, then a directory where the store writes its temp
		// file makes the save of the notified set fail.
		"store fails": func(workdir string) error {
			return os.Mkdir(filepath.Join(workdir, "scan.json.tmp"), 0o755)
		},
	}
	for name, notify := range cases {
		t.Run(name, func(t *testing.T) {
			workdir := t.TempDir()
			st, err := store.New(workdir)
			if err != nil {
				t.Fatal(err)
			}
			n := notifierFunc(func() error { return notify(workdir) })
			rec := httptest.NewRecorder()
			New(st, chalkEngine(st, n, discover.Mise{}), testInfo).Handler().
				ServeHTTP(rec, httptest.NewRequest("POST", "/api/check", nil))
			if rec.Code != 200 {
				t.Fatalf("POST /api/check = %d: %s", rec.Code, rec.Body)
			}
			var got struct {
				Flagged []struct {
					Name string `json:"name"`
				} `json:"flagged"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if len(got.Flagged) != 1 || got.Flagged[0].Name != "chalk" {
				t.Errorf("flagged = %+v, want chalk", got.Flagged)
			}
		})
	}
}

// TestVersionAndHealthz pins the cross-tool build metadata contract: the four
// keys, the injected values, and the headers ralph and status probe with.
func TestVersionAndHealthz(t *testing.T) {
	h := testServer(t).Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/version", nil))
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	var v map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"version":    testInfo.Version,
		"commit":     testInfo.Commit,
		"tag":        testInfo.Tag,
		"build_time": testInfo.BuildTime,
	}
	if len(v) != len(want) {
		t.Errorf("version body = %q, want exactly the keys %v", rec.Body.Bytes(), want)
	}
	for k, w := range want {
		if v[k] != w {
			t.Errorf("version[%q] = %q, want %q", k, v[k], w)
		}
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != 204 {
		t.Errorf("GET /healthz = %d, want 204", rec.Code)
	}
}

func TestAPIReportsDiscoveryErrors(t *testing.T) {
	srv := testServer(t)
	h := srv.Handler()

	// No errors: the field is an empty list, never null.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/deps", nil))
	if !strings.Contains(rec.Body.String(), `"discovery_errors":[]`) {
		t.Errorf("GET /api/deps without errors = %s, want discovery_errors: []", rec.Body.String())
	}

	failed := store.DiscoveryError{
		Repo:      "/home/u/code/widget",
		Ecosystem: "Go",
		Error:     "go list: exit status 1",
	}
	if err := srv.store.SaveRepo("/home/u/code/widget", srv.store.Snapshot().Deps,
		[]store.DiscoveryError{failed}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/deps", "/api/flagged"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		var body struct {
			DiscoveryErrors []store.DiscoveryError `json:"discovery_errors"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if len(body.DiscoveryErrors) != 1 || body.DiscoveryErrors[0] != failed {
			t.Errorf(
				"GET %s discovery_errors = %+v, want [%+v]",
				path,
				body.DiscoveryErrors,
				failed,
			)
		}
	}
}

// TestAPIReportsRegistryUnknown: a dep whose registry lookup got no answer is
// counted in registry_unknown on every listing, and the inventory marks it, so
// the page and the MCP tools never read it as clean.
func TestAPIReportsRegistryUnknown(t *testing.T) {
	srv := testServer(t)
	deps := srv.store.Snapshot().Deps
	deps[0].RegistryUnknown = true
	if err := srv.store.Save(deps, nil); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()
	for _, path := range []string{"/api/deps", "/api/flagged"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		var body struct {
			RegistryUnknown int `json:"registry_unknown"`
			Deps            []struct {
				Name            string `json:"name"`
				RegistryUnknown bool   `json:"registry_unknown"`
			} `json:"deps"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		if body.RegistryUnknown != 1 {
			t.Errorf("GET %s registry_unknown = %d, want 1", path, body.RegistryUnknown)
		}
		if path == "/api/deps" && (len(body.Deps) != 2 || !body.Deps[0].RegistryUnknown ||
			body.Deps[1].RegistryUnknown) {
			t.Errorf("GET /api/deps deps = %+v, want only %s marked", body.Deps, deps[0].Name)
		}
	}
}

// TestAppJSAllClearComesAfterEveryGap: the page shows its green all-clear only
// after ruling out each way the last check can be incomplete, a discovery
// error or an unanswered registry lookup. deps has no harness that runs
// app.js, so this reads the branch chain that leads to the all-clear from the
// served source.
func TestAppJSAllClearComesAfterEveryGap(t *testing.T) {
	rec := httptest.NewRecorder()
	testServer(t).Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/app.js", nil))
	src := rec.Body.String()
	start := strings.Index(src, "if (v.active.length) {")
	end := strings.Index(src, "{ class: 'ok' }")
	if start < 0 || end < start {
		t.Fatalf("app.js: no active-findings branch before the all-clear (%d, %d)", start, end)
	}
	chain := src[start:end]
	for _, gap := range []string{"v.discoveryErrors.length", "data.registry_unknown"} {
		if !strings.Contains(chain, "} else if ("+gap+")") {
			t.Errorf("app.js reaches the all-clear without an else-if on %s", gap)
		}
	}
}

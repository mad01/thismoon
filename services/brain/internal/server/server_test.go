package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/mad01/thismoon/buildinfo"

	"github.com/mad01/thismoon/services/brain/internal/config"
	"github.com/mad01/thismoon/services/brain/internal/testhome"
)

const (
	idAlpha   = "01JALPHAALPHAALPHAALPHAALP"
	idGuide   = "01JGUIDEGUIDEGUIDEGUIDEGUI"
	idPending = "01JPENDINGPENDINGPENDINGPE"
	alphaNode = "id:" + idAlpha
	guideNode = "id:" + idGuide
	betaNode  = "personal:notes/beta.md"
	oldNode   = "personal:notes/old.md"
	mocNode   = "personal:MOCs/index.md"
	// pastDate puts a review_by behind today, so the note is stale.
	pastDate = "2020-01-01"
)

// fixture is a private personal vault and a shared team vault.
var fixture = map[string]map[string]string{
	"personal": {
		"notes/alpha.md": "---\ntitle: Alpha\nid: " + idAlpha + "\ntags: [x]\n---\n" +
			"Alpha is about kubernetes. [[beta|Bee]] [[ghost]] [[Team Guide]]\n",
		"notes/beta.md": "---\ntitle: Beta\nreview_by: " + pastDate + "\n---\n[[alpha]]\n",
		"notes/old.md": "---\ntitle: Old\nstatus: superseded\nsuperseded_by: id:" + idAlpha +
			"\nstatus_at: 2026-01-02\n---\nOld text.\n",
		"notes/shared.md": "---\ntitle: Personal copy\n---\nmine\n",
		"inbox/pending.md": "---\ntitle: Pending\nid: " + idPending + "\nstatus: promoting\n" +
			"shared_pr: https://example.com/pr/1\nstatus_at: 2026-02-03\n---\nOn its way.\n",
		"MOCs/index.md": "---\ntitle: Index\n---\n[[alpha]]\n",
	},
	"team": {
		"notes/team-guide.md": "---\ntitle: Team Guide\nid: " + idGuide + "\n---\nTeam kubernetes guide.\n",
		"notes/shared.md":     "---\ntitle: Team copy\n---\nours\n",
	},
}

// testUI stands in for the ui package's files.
var testUI = fstest.MapFS{
	"shell.html":         {Data: []byte("<!doctype html><title>brain</title>")},
	"app.js":             {Data: []byte("// app")},
	"graph.js":           {Data: []byte("// graph")},
	"assets/SHA256SUMS":  {Data: []byte("abc  brain-graph.bundle.js\n")},
	"assets/bundle.js":   {Data: []byte("// bundle")},
	"assets/nested/x.js": {Data: []byte("// nested")},
}

func TestMain(m *testing.M) {
	log.SetOutput(io.Discard)
	os.Exit(m.Run())
}

// clock is a settable clock for the fingerprint memo.
type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// pass moves the clock past the fingerprint memo.
func (c *clock) pass() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(fingerprintTTL + time.Second)
}

type harness struct {
	t     *testing.T
	srv   *Server
	h     http.Handler
	roots map[string]string
	clock *clock
	state string
}

// newHarness isolates brain, writes the fixture vaults and a config naming
// them, and opens a server over it.
func newHarness(t *testing.T) *harness {
	t.Helper()
	return newHarnessWith(t, "")
}

// newHarnessWith is newHarness with extra lines appended to the config.
func newHarnessWith(t *testing.T, extraConfig string) *harness {
	t.Helper()
	home := testhome.Isolate(t)
	state := filepath.Join(home, "state")
	t.Setenv("XDG_STATE_HOME", state)
	roots := map[string]string{}
	for name, notes := range fixture {
		roots[name] = writeVault(t, notes)
	}
	cfgPath := filepath.Join(home, "config.yaml")
	cfgText := "vaults:\n" +
		"  - {name: personal, path: " + roots["personal"] + "}\n" +
		"  - {name: team, path: " + roots["team"] + ", shared: true}\n" + extraConfig
	if err := os.WriteFile(cfgPath, []byte(cfgText), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	return openHarness(
		t,
		Options{Config: cfg, ConfigPath: cfgPath},
		roots,
		filepath.Join(state, "brain"),
	)
}

func openHarness(t *testing.T, opts Options, roots map[string]string, state string) *harness {
	t.Helper()
	c := &clock{t: time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)}
	opts.UI = testUI
	opts.Info = buildinfo.Info{Version: "1.2.3", Commit: "abc1234"}
	opts.Now = c.now
	srv, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{t: t, srv: srv, h: srv.Handler(), roots: roots, clock: c, state: state}
}

func writeVault(t *testing.T, notes map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range notes {
		writeNote(t, root, rel, content)
	}
	return root
}

func writeNote(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// do sends one request and returns the recorded response.
func (h *harness) do(method, target string, body []byte) *httptest.ResponseRecorder {
	h.t.Helper()
	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	rec := httptest.NewRecorder()
	h.h.ServeHTTP(rec, req)
	return rec
}

// getJSON GETs target, wants status, and decodes the body into T.
func getJSON[T any](h *harness, target string, status int) T {
	h.t.Helper()
	rec := h.do(http.MethodGet, target, nil)
	return decode[T](h.t, rec, status)
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d; body %s", rec.Code, status, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type %q, want application/json", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control %q, want no-store", cc)
	}
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %T: %v\n%s", v, err, rec.Body)
	}
	return v
}

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// wantError sends a request and wants the error shape with status and code.
func (h *harness) wantError(method, target string, body []byte, status int, code string) {
	h.t.Helper()
	rec := h.do(method, target, body)
	e := decode[errorResponse](h.t, rec, status)
	if e.Error.Code != code || e.Error.Message == "" {
		h.t.Errorf("%s %s: error %+v, want code %q and a message", method, target, e.Error, code)
	}
}

func TestStaticRoutes(t *testing.T) {
	h := newHarness(t)
	tests := []struct {
		target string
		status int
		want   string
	}{
		{"/", http.StatusOK, "<title>brain</title>"},
		{"/app.js", http.StatusOK, "// app"},
		{"/graph.js", http.StatusOK, "// graph"},
		{"/assets/SHA256SUMS", http.StatusOK, "brain-graph.bundle.js"},
		{"/assets/bundle.js", http.StatusOK, "// bundle"},
		{"/assets/missing.js", http.StatusNotFound, ""},
		{"/assets/nested", http.StatusNotFound, ""},
		{"/healthz", http.StatusOK, "ok"},
		{"/version", http.StatusOK, `"commit": "abc1234"`},
		{"/webkit/webkit.css", http.StatusOK, ""},
		{"/nope", http.StatusNotFound, ""},
	}
	for _, tc := range tests {
		t.Run(tc.target, func(t *testing.T) {
			rec := h.do(http.MethodGet, tc.target, nil)
			if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.want) {
				t.Errorf("GET %s = %d %q, want %d containing %q",
					tc.target, rec.Code, rec.Body, tc.status, tc.want)
			}
		})
	}
	ct := h.do(http.MethodGet, "/app.js", nil).Header().Get("Content-Type")
	if !strings.Contains(ct, "javascript") {
		t.Errorf("app.js Content-Type %q, want javascript", ct)
	}
}

// TestOnlyLayoutWrites pins the one non-GET route, and that every other
// route refuses a write method.
func TestOnlyLayoutWrites(t *testing.T) {
	h := newHarness(t)
	for _, r := range h.srv.routes() {
		method, path, _ := strings.Cut(r.pattern, " ")
		if r.pattern == "POST /api/layout" {
			continue
		}
		if method != http.MethodGet {
			t.Errorf("route %q is not GET; only POST /api/layout may write", r.pattern)
		}
		path = concretePath(path)
		for _, m := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch} {
			if rec := h.do(m, path, []byte("{}")); rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s = %d, want 405", m, path, rec.Code)
			}
		}
	}
	for _, m := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		if rec := h.do(m, "/api/layout", nil); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/layout = %d, want 405", m, rec.Code)
		}
	}
}

// concretePath turns a mux path pattern into a path it matches.
func concretePath(pattern string) string {
	path := strings.ReplaceAll(pattern, "{$}", "")
	path = strings.ReplaceAll(path, "{file}", "x")
	if strings.HasSuffix(path, "/") && path != "/" {
		path += "x"
	}
	return path
}

func TestUnknownAPIRoute(t *testing.T) {
	newHarness(t).wantError(http.MethodGet, "/api/nope", nil, http.StatusNotFound, codeNotFound)
}

func TestVaults(t *testing.T) {
	h := newHarness(t)
	got := getJSON[vaultsView](h, "/api/vaults", http.StatusOK)
	want := []vaultEntry{
		{Name: "personal", Default: true, Notes: 6},
		{Name: "team", Shared: true, Notes: 2},
	}
	if len(got.Vaults) != len(want) || got.Vaults[0] != want[0] || got.Vaults[1] != want[1] {
		t.Errorf("vaults = %+v, want %+v", got.Vaults, want)
	}
}

func TestStateMemoizesFingerprints(t *testing.T) {
	h := newHarness(t)
	first := getJSON[stateView](h, "/api/state", http.StatusOK)
	if len(first.Vaults) != 2 || first.Vaults[0].Name != "personal" ||
		first.Vaults[0].Fingerprint == "" || first.Key == "" {
		t.Fatalf("state = %+v", first)
	}
	graph := getJSON[graphView](h, "/api/graph", http.StatusOK)
	if graph.Key != first.Key {
		t.Errorf("graph key %s, state key %s; want one key without topics", graph.Key, first.Key)
	}

	touch(t, filepath.Join(h.roots["personal"], "notes", "beta.md"),
		"---\ntitle: Beta\n---\n[[alpha]] [[team-guide]]\n")
	if again := getJSON[stateView](h, "/api/state", http.StatusOK); again.Key != first.Key {
		t.Errorf("key changed within the memo: %s, then %s", first.Key, again.Key)
	}
	h.clock.pass()
	if later := getJSON[stateView](h, "/api/state", http.StatusOK); later.Key == first.Key {
		t.Errorf("key %s did not change after the vault did", later.Key)
	}
}

// touch rewrites a note and moves its mtime an hour on, so the fingerprint
// sees the change whatever the file system's timestamp resolution.
func touch(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
}

func TestModelRebuildsAfterANoteChanges(t *testing.T) {
	h := newHarness(t)
	edge := func(g graphView, source, target string) bool {
		for _, e := range g.Edges {
			if e.Source == source && e.Target == target {
				return true
			}
		}
		return false
	}
	if g := getJSON[graphView](h, "/api/graph", http.StatusOK); edge(g, betaNode, guideNode) {
		t.Fatal("beta links to the team guide before the edit")
	}
	touch(t, filepath.Join(h.roots["personal"], "notes", "beta.md"),
		"---\ntitle: Beta\n---\n[[alpha]] [[team-guide]]\n")
	if g := getJSON[graphView](h, "/api/graph", http.StatusOK); edge(g, betaNode, guideNode) {
		t.Error("the model rebuilt within the fingerprint memo")
	}
	h.clock.pass()
	g := getJSON[graphView](h, "/api/graph", http.StatusOK)
	if !edge(g, betaNode, guideNode) {
		t.Errorf("edges %+v lack beta -> team guide after the edit", g.Edges)
	}
	for _, n := range g.Nodes {
		if n.ID == betaNode && n.Stale {
			t.Error("beta is still stale after its review_by was removed")
		}
	}
}

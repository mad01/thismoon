package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mad01/thismoon/services/keep/internal/store"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	return httptest.NewServer(New(st, "test", "tester").Handler())
}

// gitRepo creates a temp git repo with a committed f.txt of five lines
// ("l1\n".."l5\n"), so pins resolve against a real working tree with a HEAD.
func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	writeFile(t, dir, "f.txt", "l1\nl2\nl3\nl4\nl5\n")
	run("add", "f.txt")
	run("commit", "-q", "-m", "init")
	return dir
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// assertBody builds a POST /api/assertions body with one pin on f.txt.
func assertBody(repo string, start, end int) string {
	return fmt.Sprintf(`{
		"kind":"code-behavior","subject":"repo:x/y","statement":"the thing holds",
		"confidence":"derived","session_id":"s1",
		"pins":[{"repo_path":%q,"file":"f.txt","start_line":%d,"end_line":%d}]
	}`, repo, start, end)
}

// postAssert posts body and decodes the resulting assertion, asserting the code.
func postAssert(t *testing.T, ts *httptest.Server, body string, wantCode int) store.Assertion {
	t.Helper()
	res, err := http.Post(ts.URL+"/api/assertions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != wantCode {
		out, _ := io.ReadAll(res.Body)
		t.Fatalf("status = %d, want %d (%s)", res.StatusCode, wantCode, out)
	}
	var a store.Assertion
	if wantCode == http.StatusCreated {
		if err := json.NewDecoder(res.Body).Decode(&a); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return a
}

func TestAssertResolvesPins(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)

	a := postAssert(t, ts, assertBody(repo, 2, 4), http.StatusCreated)
	if a.ID == "" {
		t.Fatal("assertion missing id")
	}
	if a.Status != store.StatusFresh {
		t.Errorf("status = %q, want fresh", a.Status)
	}
	if len(a.Pins) != 1 {
		t.Fatalf("pins = %d, want 1", len(a.Pins))
	}
	p := a.Pins[0]
	if p.ContentSHA256 == "" {
		t.Error("pin content_sha256 should be resolved")
	}
	if p.HeadCommit == "" {
		t.Error("pin head_commit should be resolved")
	}
	if a.Provenance.Author != "tester" {
		t.Errorf("provenance author = %q, want tester (server-stamped)", a.Provenance.Author)
	}
}

func TestAssertZeroPins(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	body := `{"kind":"code-behavior","subject":"repo:x/y","statement":"s",
		"confidence":"derived","session_id":"s1","pins":[]}`
	postAssert(t, ts, body, http.StatusBadRequest)
}

func TestAssertBadRange(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)
	// end_line beyond the 5-line file.
	postAssert(t, ts, assertBody(repo, 2, 99), http.StatusBadRequest)
}

func TestAssertBadKind(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)
	body := fmt.Sprintf(`{"kind":"nonsense","subject":"repo:x/y","statement":"s",
		"confidence":"derived","session_id":"s1",
		"pins":[{"repo_path":%q,"file":"f.txt","start_line":1,"end_line":2}]}`, repo)
	postAssert(t, ts, body, http.StatusBadRequest)
}

func TestListAndFilters(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)
	postAssert(t, ts, assertBody(repo, 1, 2), http.StatusCreated)

	list := func(query string) []store.Assertion {
		res, err := http.Get(ts.URL + "/api/assertions" + query)
		if err != nil {
			t.Fatalf("list%s: %v", query, err)
		}
		defer func() { _ = res.Body.Close() }()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("list%s status = %d", query, res.StatusCode)
		}
		var out struct {
			Assertions []store.Assertion `json:"assertions"`
		}
		_ = json.NewDecoder(res.Body).Decode(&out)
		return out.Assertions
	}

	if got := list(""); len(got) != 1 {
		t.Fatalf("list all = %d, want 1", len(got))
	}
	if got := list("?subject=repo:x"); len(got) != 1 {
		t.Errorf("subject prefix match = %d, want 1", len(got))
	}
	if got := list("?subject=nope"); len(got) != 0 {
		t.Errorf("subject miss = %d, want 0", len(got))
	}
	if got := list("?kind=code-behavior"); len(got) != 1 {
		t.Errorf("kind match = %d, want 1", len(got))
	}
	if got := list("?kind=decision"); len(got) != 0 {
		t.Errorf("kind miss = %d, want 0", len(got))
	}
	if got := list("?status=fresh"); len(got) != 1 {
		t.Errorf("status match = %d, want 1", len(got))
	}

	// invalid kind / status → 400.
	for _, q := range []string{"?kind=bogus", "?status=bogus"} {
		res, _ := http.Get(ts.URL + "/api/assertions" + q)
		if res.StatusCode != http.StatusBadRequest {
			t.Errorf("list%s status = %d, want 400", q, res.StatusCode)
		}
		_ = res.Body.Close()
	}
}

func TestGetByIDAndNotFound(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)
	a := postAssert(t, ts, assertBody(repo, 1, 2), http.StatusCreated)

	res, err := http.Get(ts.URL + "/api/assertions/" + a.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d, want 200", res.StatusCode)
	}
	var got store.Assertion
	_ = json.NewDecoder(res.Body).Decode(&got)
	if got.ID != a.ID {
		t.Errorf("get id = %q, want %q", got.ID, a.ID)
	}

	miss, _ := http.Get(ts.URL + "/api/assertions/nope")
	if miss.StatusCode != http.StatusNotFound {
		t.Errorf("missing id status = %d, want 404", miss.StatusCode)
	}
	_ = miss.Body.Close()
}

// retract posts a retract with note and returns the response and body.
func retract(t *testing.T, ts *httptest.Server, id, note string) *http.Response {
	t.Helper()
	res, err := http.Post(ts.URL+"/api/assertions/"+id+"/retract", "application/json",
		strings.NewReader(fmt.Sprintf(`{"note":%q}`, note)))
	if err != nil {
		t.Fatalf("retract: %v", err)
	}
	return res
}

func TestRetractFlow(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)
	a := postAssert(t, ts, assertBody(repo, 1, 2), http.StatusCreated)

	// empty note → 400.
	empty := retract(t, ts, a.ID, "")
	if empty.StatusCode != http.StatusBadRequest {
		t.Errorf("empty-note retract status = %d, want 400", empty.StatusCode)
	}
	_ = empty.Body.Close()

	// first retract → 200, status retracted, note recorded.
	res := retract(t, ts, a.ID, "superseded")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("retract status = %d, want 200", res.StatusCode)
	}
	var r store.Assertion
	_ = json.NewDecoder(res.Body).Decode(&r)
	_ = res.Body.Close()
	if r.Status != store.StatusRetracted || r.RetractNote != "superseded" {
		t.Errorf("after retract: status=%q note=%q", r.Status, r.RetractNote)
	}

	// second retract is idempotent → 200, note unchanged.
	again := retract(t, ts, a.ID, "different note")
	if again.StatusCode != http.StatusOK {
		t.Fatalf("second retract status = %d, want 200", again.StatusCode)
	}
	var r2 store.Assertion
	_ = json.NewDecoder(again.Body).Decode(&r2)
	_ = again.Body.Close()
	if r2.RetractNote != "superseded" {
		t.Errorf("idempotent retract overwrote note: %q", r2.RetractNote)
	}
}

// postCheck posts an optional-id check body and decodes the report.
func postCheck(t *testing.T, ts *httptest.Server, body string) checkResp {
	t.Helper()
	var rdr io.Reader
	if body != "" {
		rdr = strings.NewReader(body)
	}
	res, err := http.Post(ts.URL+"/api/check", "application/json", rdr)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		out, _ := io.ReadAll(res.Body)
		t.Fatalf("check status = %d, want 200 (%s)", res.StatusCode, out)
	}
	var rep checkResp
	if err := json.NewDecoder(res.Body).Decode(&rep); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return rep
}

func TestCheckStaleAndBackToFresh(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)
	a := postAssert(t, ts, assertBody(repo, 2, 4), http.StatusCreated)

	// Rewrite line 3 inside the pinned range → check flips it stale.
	writeFile(t, repo, "f.txt", "l1\nl2\nCHANGED\nl4\nl5\n")
	rep := postCheck(t, ts, fmt.Sprintf(`{"id":%q}`, a.ID))
	if rep.Checked != 1 || rep.Stale != 1 || rep.Flipped != 1 {
		t.Fatalf("stale report = %+v", rep)
	}
	if len(rep.Assertions) != 1 || rep.Assertions[0].Status != store.StatusStale {
		t.Fatalf("assertion not stale: %+v", rep.Assertions)
	}
	if !strings.Contains(rep.Assertions[0].StaleReason, "content changed") {
		t.Errorf(
			"stale_reason = %q, want to contain 'content changed'",
			rep.Assertions[0].StaleReason,
		)
	}

	// Revert the line → check returns it fresh.
	writeFile(t, repo, "f.txt", "l1\nl2\nl3\nl4\nl5\n")
	rep = postCheck(t, ts, fmt.Sprintf(`{"id":%q}`, a.ID))
	if rep.Fresh != 1 || rep.Flipped != 1 {
		t.Fatalf("re-fresh report = %+v", rep)
	}
	if rep.Assertions[0].Status != store.StatusFresh {
		t.Errorf("status = %q, want fresh", rep.Assertions[0].Status)
	}
}

func TestCheckSkipsRetracted(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	repo := gitRepo(t)
	a := postAssert(t, ts, assertBody(repo, 1, 2), http.StatusCreated)

	res := retract(t, ts, a.ID, "withdrawn")
	_ = res.Body.Close()

	// check-all (empty body): the retracted assertion is reported untouched and
	// does not count as checked.
	rep := postCheck(t, ts, "")
	if rep.Checked != 0 {
		t.Errorf("checked = %d, want 0 (retracted skipped)", rep.Checked)
	}
	if len(rep.Assertions) != 1 || rep.Assertions[0].Status != store.StatusRetracted {
		t.Errorf("retracted assertion should be reported untouched: %+v", rep.Assertions)
	}
}

func TestCheckUnknownID(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res, err := http.Post(
		ts.URL+"/api/check",
		"application/json",
		strings.NewReader(`{"id":"nope"}`),
	)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("unknown id check status = %d, want 404", res.StatusCode)
	}
}

func TestHealthz(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusNoContent {
		t.Errorf("healthz status = %d, want 204", res.StatusCode)
	}
}

func TestVersion(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if cc := res.Header.Get("Cache-Control"); cc != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", cc)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), `"version":"test"`) {
		t.Errorf("version body = %q", body)
	}
}

func TestPageServesShell(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatalf("GET /: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	page := string(body)
	for _, want := range []string{`wk-header brand="keep"`, `id="app"`, "/app.js", "/webkit/webkit.js"} {
		if !strings.Contains(page, want) {
			t.Errorf("shell missing %q", want)
		}
	}
}

func TestAppJS(t *testing.T) {
	ts := newTestServer(t)
	defer ts.Close()
	res, err := http.Get(ts.URL + "/app.js")
	if err != nil {
		t.Fatalf("GET /app.js: %v", err)
	}
	defer func() { _ = res.Body.Close() }()
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/javascript") {
		t.Errorf("Content-Type = %q, want application/javascript", ct)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "/api/assertions") {
		t.Error("app.js should fetch /api/assertions")
	}
}

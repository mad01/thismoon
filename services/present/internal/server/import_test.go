package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	present "github.com/mad01/thismoon/services/present"
)

const importSample = "# Weekly\n\nA short brief.\n\n## Done\n\nShipped **it**.\n\n" +
	"```sh\nmake test\n```\n"

// postImport sends body to /api/import under contentType (empty sends no
// Content-Type header) with extra headers, and returns the status and body.
func postImport(
	t *testing.T, ts *httptest.Server, contentType, body string, extra map[string]string,
) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/import", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	for k, v := range extra {
		// Go keeps Host out of the header map; a test that wants to spoof
		// the host the request arrived on sets it here.
		if k == "Host" {
			req.Host = v
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /api/import: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, out
}

func importBody(t *testing.T, name, markdown string) string {
	t.Helper()
	raw, err := json.Marshal(importRequest{Name: name, Markdown: markdown})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(raw)
}

// errorOf decodes the {"error": ...} body every refused import answers with.
func errorOf(t *testing.T, body []byte) string {
	t.Helper()
	var out map[string]string
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("error body is not JSON: %v (%s)", err, body)
	}
	return out["error"]
}

func TestImportCreatesAReadablePage(t *testing.T) {
	ts, _ := setup(t)
	code, body := postImport(
		t,
		ts,
		"application/json",
		importBody(t, "weekly.md", importSample),
		nil,
	)
	if code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (%s)", code, body)
	}
	var out importResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if out.ID == "" || !strings.HasSuffix(out.URL, "/p/"+out.ID) {
		t.Fatalf("response = %+v, want an id and a url ending in it", out)
	}
	if !strings.HasPrefix(out.URL, ts.URL) {
		t.Errorf("url = %q, want it on this server %q", out.URL, ts.URL)
	}

	if code, _ := get(t, ts.URL+"/p/"+out.ID); code != http.StatusOK {
		t.Errorf("GET /p/%s = %d, want 200", out.ID, code)
	}
	page := apiPageOf(t, ts.URL+"/api/p/"+out.ID)
	if page.Title != "Weekly" {
		t.Errorf("title = %q, want Weekly", page.Title)
	}
	for _, want := range []string{
		`<div class="brief-summary" data-fixation>A short brief.</div>`,
		`<wk-section id="done">`,
		"Shipped <strong>it</strong>.",
		`<code class="language-sh">make test</code>`,
	} {
		if !strings.Contains(page.Content, want) {
			t.Errorf("content missing %q in: %s", want, page.Content)
		}
	}
	if !hasID(getAPIPages(t, ts.URL+"/api/pages"), out.ID) {
		t.Errorf("imported page %s missing from the index listing", out.ID)
	}
}

// The page keeps its Doc source, so present_source and rerender work on it
// like on an MCP-created page.
func TestImportPersistsTheDocSource(t *testing.T) {
	ts, st := setup(t)
	code, body := postImport(t, ts, "application/json", importBody(t, "n.md", importSample), nil)
	if code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", code, body)
	}
	var out importResponse
	_ = json.Unmarshal(body, &out)
	doc, err := st.LoadDoc(t.Context(), out.ID)
	if err != nil {
		t.Fatalf("LoadDoc: %v", err)
	}
	if !strings.Contains(string(doc), `"h":"Done"`) {
		t.Errorf("doc.json = %s, want the Done section", doc)
	}
}

func TestImportRequiresJSONContentType(t *testing.T) {
	ts, _ := setup(t)
	body := importBody(t, "n.md", importSample)
	for _, contentType := range []string{
		"",
		"text/plain;charset=UTF-8",
		"application/x-www-form-urlencoded",
		"multipart/form-data; boundary=x",
	} {
		code, out := postImport(t, ts, contentType, body, nil)
		if code != http.StatusUnsupportedMediaType {
			t.Errorf("import as %q = %d (%s), want 415", contentType, code, out)
		}
	}
	if pages := getAPIPages(t, ts.URL+"/api/pages"); pages.Total != 0 {
		t.Errorf("a refused import created %d page(s)", pages.Total)
	}
	if code, out := postImport(t, ts, "Application/JSON; charset=utf-8", body, nil); code != http.StatusCreated {
		t.Errorf("import as application/json with a charset = %d (%s), want 201", code, out)
	}
}

func TestImportRefusesCrossSiteRequests(t *testing.T) {
	ts, _ := setup(t)
	body := importBody(t, "n.md", importSample)
	refused := []map[string]string{
		{"Origin": "https://evil.example"},
		// DNS rebinding: the attacker's name resolves to this machine, so
		// Origin and Host agree; neither may vouch for the other.
		{"Origin": "http://evil.example:7423", "Host": "evil.example:7423"},
		{"Origin": "http://evil.example", "X-Forwarded-Host": "evil.example"},
		{"Origin": "http://this.example.com"},
		{"Origin": "ftp://localhost"},
		{"Origin": "null"},
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "cross-site", "Origin": "http://localhost:7423"},
	}
	for _, h := range refused {
		code, out := postImport(t, ts, "application/json", body, h)
		if code != http.StatusForbidden {
			t.Errorf("import with %v = %d (%s), want 403", h, code, out)
		}
		if msg := errorOf(t, out); msg == "" {
			t.Errorf("import with %v: no error message in %s", h, out)
		}
	}
	if pages := getAPIPages(t, ts.URL+"/api/pages"); pages.Total != 0 {
		t.Errorf("a refused import created %d page(s)", pages.Total)
	}

	allowed := []map[string]string{
		{"Origin": ts.URL},
		{"Origin": "http://localhost:7423", "Sec-Fetch-Site": "same-origin"},
		{"Origin": "http://LocalHost"},
		{"Origin": "http://[::1]:7423"},
		{"Origin": "http://present.this"},
		{"Origin": "https://Present.THIS:8443", "Host": "127.0.0.1:7423"},
	}
	for _, h := range allowed {
		code, out := postImport(t, ts, "application/json", body, h)
		if code != http.StatusCreated {
			t.Errorf("import with %v = %d (%s), want 201", h, code, out)
		}
	}
}

func TestImportRefusesAnOversizedFile(t *testing.T) {
	ts, _ := setup(t)
	big := "# Big\n\n" + strings.Repeat("word ", present.MaxPageBytes/5+1)
	code, out := postImport(t, ts, "application/json", importBody(t, "big.md", big), nil)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (%s)", code, out)
	}
	if msg := errorOf(t, out); !strings.Contains(msg, "large") {
		t.Errorf("error = %q, want it to say the file is too large", msg)
	}
	if pages := getAPIPages(t, ts.URL+"/api/pages"); pages.Total != 0 {
		t.Errorf("an oversized import created %d page(s)", pages.Total)
	}
}

// A file under the request cap can still render to a page over the store
// cap; the same 413 covers it, so a later share cannot be the first to fail.
func TestImportRefusesAPageOverTheShareCap(t *testing.T) {
	ts, _ := setup(t)
	// Each bullet is short in markdown but grows in the rendered list plus
	// the canonical doc.json, which together are what the cap measures.
	var b strings.Builder
	b.WriteString("# L\n\n")
	for i := 0; i < 24000; i++ {
		b.WriteString("- **item** `code` [l](http://x)\n")
	}
	if b.Len() >= present.MaxPageBytes {
		t.Fatalf("sample is %d bytes; it must stay under the request cap", b.Len())
	}
	code, out := postImport(t, ts, "application/json", importBody(t, "l.md", b.String()), nil)
	if code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413 (%s)", code, out)
	}
}

func TestImportRefusesEmptyOrContentlessMarkdown(t *testing.T) {
	ts, _ := setup(t)
	for _, md := range []string{"", "  \n\n", "# Title only\n", "---\n"} {
		code, out := postImport(t, ts, "application/json", importBody(t, "e.md", md), nil)
		if code != http.StatusBadRequest {
			t.Errorf("import of %q = %d (%s), want 400", md, code, out)
		}
		if msg := errorOf(t, out); msg == "" {
			t.Errorf("import of %q: no error message in %s", md, out)
		}
	}
	if code, out := postImport(t, ts, "application/json", "{not json", nil); code != http.StatusBadRequest {
		t.Errorf("malformed body = %d (%s), want 400", code, out)
	}
}

func TestImportFallsBackToTheFileNameAsTitle(t *testing.T) {
	ts, _ := setup(t)
	code, body := postImport(
		t,
		ts,
		"application/json",
		importBody(t, "meeting-notes.md", "Just a paragraph.\n"),
		nil,
	)
	if code != http.StatusCreated {
		t.Fatalf("status = %d (%s)", code, body)
	}
	var out importResponse
	_ = json.Unmarshal(body, &out)
	if got := apiPageOf(t, ts.URL+"/api/p/"+out.ID).Title; got != "meeting-notes" {
		t.Errorf("title = %q, want the file name without extension", got)
	}
}

// A shared instance takes writes only with an author key; the anonymous
// import endpoint does not exist there.
func TestSharedModeHasNoImport(t *testing.T) {
	f := setupShared(t)
	code, out := postImport(t, f.ts, "application/json", importBody(t, "n.md", importSample), nil)
	if code != http.StatusNotFound {
		t.Fatalf("shared POST /api/import = %d (%s), want 404", code, out)
	}
	if strings.Contains(string(out), "import") {
		t.Errorf("shared mode answered like the import handler: %s", out)
	}
}

func TestIndexHasImportAffordance(t *testing.T) {
	ts, _ := setup(t)
	_, shell := get(t, ts.URL+"/")
	for _, want := range []string{`id="import-file"`, `accept=".md,.markdown,.txt"`} {
		if !contains(shell, want) {
			t.Errorf("index shell missing %q", want)
		}
	}
	_, js := get(t, ts.URL+"/index.js")
	for _, want := range []string{"/api/import", "import-file", "drop"} {
		if !contains(js, want) {
			t.Errorf("index.js missing %q", want)
		}
	}
}

// The shared how-to page never offers the local import.
func TestSharedHowToHasNoImportAffordance(t *testing.T) {
	f := setupShared(t)
	_, body := get(t, f.ts.URL+"/")
	if contains(body, "import-file") || contains(body, "/api/import") {
		t.Errorf("shared how-to page offers the import control")
	}
}

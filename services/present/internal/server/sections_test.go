package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// docPage stores a page whose brief is a Doc with one section per heading.
func docPage(t *testing.T, st store.Store, headings ...string) store.Page {
	t.Helper()
	c := compiledDoc(t, headings...)
	p, err := st.Create(t.Context(), store.Draft{Title: "Weekly", Content: c.HTML, Doc: c.JSON})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func compiledDoc(t *testing.T, headings ...string) render.Compiled {
	t.Helper()
	var d render.Doc
	for _, h := range headings {
		d.Sections = append(d.Sections, render.Section{
			Heading: h, Blocks: []render.Block{{T: "p", Text: "about " + h}},
		})
	}
	c, err := render.CompileDoc(d, "Weekly")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func sectionOf(h string) map[string]any {
	return map[string]any{"h": h, "blocks": []map[string]any{{"t": "p", "text": "about " + h}}}
}

// sendSection sends a section request as local present's own page would:
// JSON with no Origin. extra overrides or adds headers.
func sendSection(
	t *testing.T, ts *httptest.Server, method, path string, body any, extra map[string]string,
) (int, map[string]any) {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(method, ts.URL+path, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range extra {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	out, _ := io.ReadAll(resp.Body)
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("%s %s: status %d, body %q is not JSON", method, path, resp.StatusCode, out)
	}
	return resp.StatusCode, decoded
}

func storedHeadings(t *testing.T, st store.Store, id string) []string {
	t.Helper()
	raw, err := st.LoadDoc(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	var d render.Doc
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, s := range d.Sections {
		out = append(out, s.Heading)
	}
	return out
}

func TestSectionRoutesReplaceAndAppend(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha", "Beta")
	status, out := sendSection(t, ts, http.MethodPatch, "/api/p/"+p.ID+"/sections/beta",
		map[string]any{"op": "replace", "base_version": 1, "section": sectionOf("Beta two")}, nil)
	if status != http.StatusOK || out["version"] != 2.0 || out["index"] != 1.0 ||
		out["anchor"] != "beta-two" || out["url"] != ts.URL+"/p/"+p.ID {
		t.Fatalf("replace: %d %v", status, out)
	}
	status, out = sendSection(t, ts, http.MethodPost, "/api/p/"+p.ID+"/sections",
		map[string]any{"section": sectionOf("Gamma")}, nil)
	if status != http.StatusOK || out["version"] != 3.0 || out["index"] != 2.0 ||
		out["op"] != "append" {
		t.Fatalf("append: %d %v", status, out)
	}
	if got := storedHeadings(t, st, p.ID); !slices.Equal(
		got,
		[]string{"Alpha", "Beta two", "Gamma"},
	) {
		t.Errorf("stored headings = %v", got)
	}
	got, err := st.Get(t.Context(), p.ID)
	if err != nil || got.Version != 3 ||
		!bytes.Contains([]byte(got.Content), []byte(`id="gamma"`)) {
		t.Errorf("stored page = v%d, err %v, content missing the new section", got.Version, err)
	}
}

func TestSectionRouteTakesMarkdown(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha", "Beta")
	status, out := sendSection(
		t,
		ts,
		http.MethodPatch,
		"/api/p/"+p.ID+"/sections/beta",
		map[string]any{
			"op":          "replace",
			"body_format": "markdown",
			"section":     "## Beta\n\nShipped **it**.\n",
		},
		nil,
	)
	if status != http.StatusOK || out["anchor"] != "beta" {
		t.Fatalf("markdown replace: %d %v", status, out)
	}
	raw, _ := st.LoadDoc(t.Context(), p.ID)
	if !bytes.Contains(raw, []byte(`"text":"Shipped **it**."`)) {
		t.Errorf("stored Doc lacks the markdown body: %s", raw)
	}
	status, out = sendSection(t, ts, http.MethodPost, "/api/p/"+p.ID+"/sections",
		map[string]any{"body_format": "markdown", "section": "no heading here"}, nil)
	if status != http.StatusBadRequest {
		t.Errorf("markdown without heading: %d %v, want 400", status, out)
	}
}

func TestSectionRouteConflicts(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha", "Dup", "Dup")
	base := "/api/p/" + p.ID + "/sections/"

	status, out := sendSection(t, ts, http.MethodPatch, base+"alpha",
		map[string]any{"op": "remove", "base_version": 7}, nil)
	if status != http.StatusConflict || out["version"] != 1.0 {
		t.Errorf("stale base version: %d %v, want 409 with version 1", status, out)
	}
	status, out = sendSection(
		t,
		ts,
		http.MethodPatch,
		base+"dup",
		map[string]any{"op": "remove"},
		nil,
	)
	if status != http.StatusConflict || !sameIndices(out["indices"], 1, 2) {
		t.Errorf("ambiguous anchor: %d %v, want 409 with indices [1 2]", status, out)
	}
	status, out = sendSection(t, ts, http.MethodPost, "/api/p/"+p.ID+"/sections",
		map[string]any{"section": sectionOf("Alpha")}, nil)
	if status != http.StatusConflict || !sameIndices(out["indices"], 0) {
		t.Errorf("duplicate created: %d %v, want 409 with indices [0]", status, out)
	}
	status, out = sendSection(
		t,
		ts,
		http.MethodPatch,
		base+"index:2",
		map[string]any{"op": "remove"},
		nil,
	)
	if status != http.StatusOK || out["index"] != 2.0 {
		t.Fatalf("index form: %d %v", status, out)
	}
	if got := storedHeadings(t, st, p.ID); !slices.Equal(got, []string{"Alpha", "Dup"}) {
		t.Errorf("stored headings = %v", got)
	}
}

func sameIndices(v any, want ...int) bool {
	got, ok := v.([]any)
	if !ok || len(got) != len(want) {
		return false
	}
	for i, w := range want {
		if got[i] != float64(w) {
			return false
		}
	}
	return true
}

func TestSectionRouteRefusals(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha")
	legacy, err := st.Create(t.Context(), store.Draft{Title: "Old", Content: "<p>html</p>"})
	if err != nil {
		t.Fatal(err)
	}
	badTone := sectionOf("Toned")
	badTone["tone"] = "chartreuse"
	cases := []struct {
		name, method, path string
		body               map[string]any
		headers            map[string]string
		want               int
	}{
		{
			"unknown page", http.MethodPatch, "/api/p/0123456789/sections/alpha",
			map[string]any{"op": "remove"},
			nil, http.StatusNotFound,
		},
		{
			"unknown anchor", http.MethodPatch, "/api/p/" + p.ID + "/sections/zeta",
			map[string]any{"op": "remove"},
			nil, http.StatusNotFound,
		},
		{
			"unknown op", http.MethodPatch, "/api/p/" + p.ID + "/sections/alpha",
			map[string]any{"op": "move"},
			nil, http.StatusBadRequest,
		},
		{
			"renderer refusal", http.MethodPost, "/api/p/" + p.ID + "/sections",
			map[string]any{"section": badTone},
			nil, http.StatusBadRequest,
		},
		{
			"POST with another op", http.MethodPost, "/api/p/" + p.ID + "/sections",
			map[string]any{"op": "replace", "section": sectionOf("X")},
			nil, http.StatusBadRequest,
		},
		{
			"legacy HTML brief", http.MethodPost, "/api/p/" + legacy.ID + "/sections",
			map[string]any{"section": sectionOf("X")},
			nil, http.StatusConflict,
		},
		{
			"no JSON content type", http.MethodPost, "/api/p/" + p.ID + "/sections",
			map[string]any{"section": sectionOf("X")},
			map[string]string{"Content-Type": "text/plain"},
			http.StatusUnsupportedMediaType,
		},
		{
			"cross-site", http.MethodPost, "/api/p/" + p.ID + "/sections",
			map[string]any{"section": sectionOf("X")},
			map[string]string{"Sec-Fetch-Site": "cross-site"},
			http.StatusForbidden,
		},
		{
			"foreign origin", http.MethodPost, "/api/p/" + p.ID + "/sections",
			map[string]any{"section": sectionOf("X")},
			map[string]string{"Origin": "https://evil.example"},
			http.StatusForbidden,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if status, out := sendSection(t, ts, tc.method, tc.path, tc.body, tc.headers); status != tc.want {
				t.Errorf("status %d %v, want %d", status, out, tc.want)
			}
		})
	}
	if got, _ := st.Get(t.Context(), p.ID); got.Version != 1 {
		t.Errorf("refused patches moved the page to v%d", got.Version)
	}
}

func TestSharedSectionPatchIsAuthorOnlyAndRenewsExpiry(t *testing.T) {
	f := setupShared(t)
	key := author.NewKey()
	c := compiledDoc(t, "Alpha")
	created := f.create(t, key, map[string]any{
		"title": "Weekly", "content": c.HTML, "doc": json.RawMessage(c.JSON), "ephemeral": true,
	})
	path := "/api/p/" + created.ID + "/sections"
	body := map[string]any{"section": sectionOf("Beta")}
	if resp, _ := f.call(t, http.MethodPost, path, "", body, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no key: status %d, want 401", resp.StatusCode)
	}
	if resp, _ := f.call(t, http.MethodPost, path, author.NewKey(), body, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("wrong key: status %d, want 403", resp.StatusCode)
	}
	f.now = f.now.Add(24 * time.Hour)
	resp, out := f.call(t, http.MethodPost, path, key, body, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("author: status %d %s", resp.StatusCode, out)
	}
	got, err := f.raw.Get(t.Context(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if want := f.now.Add(present.SharedTTL); got.ExpiresAt == nil || !got.ExpiresAt.Equal(want) {
		t.Errorf("expiry after the patch = %v, want %v", got.ExpiresAt, want)
	}
}

// TestParallelSectionAppendsAllLand has two workers append through the
// route at once, the way parallel agents report into one page: every
// section lands and every append got its own version.
func TestParallelSectionAppendsAllLand(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Start")
	const workers, each = 2, 10
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for w := range workers {
		wg.Go(func() {
			for i := range each {
				section := sectionOf(fmt.Sprintf("Worker %d item %d", w, i))
				if err := appendSection(ts.URL+"/api/p/"+p.ID+"/sections", section); err != nil {
					errs[w] = errors.Join(errs[w], fmt.Errorf("append %d: %w", i, err))
				}
			}
		})
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	got, err := st.Get(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if headings := storedHeadings(t, st, p.ID); len(headings) != 1+workers*each ||
		got.Version != 1+workers*each {
		t.Errorf("after parallel appends: %d sections at v%d, want %d at v%d",
			len(headings), got.Version, 1+workers*each, 1+workers*each)
	}
}

// appendSection posts one section and reports anything but a 200. It is
// safe off the test goroutine.
func appendSection(url string, section map[string]any) error {
	raw, err := json.Marshal(map[string]any{"section": section})
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, body)
	}
	return nil
}

package server

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/mad01/thismoon/services/present/internal/author"
)

// getJSON fetches path and decodes the JSON body.
func getJSON(t *testing.T, url string) (int, map[string]any) {
	t.Helper()
	status, body := get(t, url)
	var out map[string]any
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("GET %s: status %d, body %q is not JSON", url, status, body)
	}
	return status, out
}

func stringsOf(v any) []string {
	items, _ := v.([]any)
	out := []string{}
	for _, it := range items {
		s, _ := it.(string)
		out = append(out, s)
	}
	return out
}

func TestHistoryRoutesListFetchAndDiff(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha", "Beta")
	base := "/api/p/" + p.ID
	if status, out := sendSection(t, ts, http.MethodPatch, base+"/sections/beta",
		map[string]any{"op": "replace", "section": sectionOf("Beta")}, nil); status != http.StatusOK {
		t.Fatalf("replace: %d %v", status, out)
	}
	if status, out := sendSection(t, ts, http.MethodPost, base+"/sections",
		map[string]any{"section": sectionOf("Gamma")}, nil); status != http.StatusOK {
		t.Fatalf("append: %d %v", status, out)
	}

	status, out := getJSON(t, ts.URL+base+"/history")
	revs, _ := out["revisions"].([]any)
	if status != http.StatusOK || out["version"] != 3.0 || len(revs) != 3 {
		t.Fatalf("history: %d %v", status, out)
	}
	status, out = getJSON(t, ts.URL+base+"/history/1")
	if status != http.StatusOK || out["content_format"] != "doc" || out["version"] != 1.0 ||
		out["title"] != "Weekly" {
		t.Fatalf("revision 1: %d %v", status, out)
	}

	status, out = getJSON(t, ts.URL+base+"/diff?since=1")
	content, _ := out["content"].(map[string]any)
	if status != http.StatusOK || out["version"] != 3.0 ||
		!slices.Equal(stringsOf(content["added"]), []string{"gamma"}) ||
		len(stringsOf(content["changed"])) != 0 {
		t.Fatalf("diff since 1: %d %v", status, out)
	}

	for path, want := range map[string]int{
		base + "/diff?since=9":           http.StatusBadRequest,
		base + "/diff?since=x":           http.StatusBadRequest,
		base + "/history/0":              http.StatusBadRequest,
		base + "/history/7":              http.StatusNotFound,
		"/api/p/0123456789/history":      http.StatusNotFound,
		"/api/p/0123456789/diff?since=1": http.StatusNotFound,
	} {
		if status, out := getJSON(t, ts.URL+path); status != want {
			t.Errorf("GET %s: %d %v, want %d", path, status, out, want)
		}
	}
}

// A shared instance serves a wrapped store, which keeps no history, so the
// routes do not exist there.
func TestSharedInstanceHasNoHistoryRoutes(t *testing.T) {
	f := setupShared(t)
	created := f.create(t, author.NewKey(), page("Shared"))
	if status, _ := get(t, f.ts.URL+"/api/p/"+created.ID); status != http.StatusOK {
		t.Fatalf("the page itself: status %d, want 200", status)
	}
	if status, _ := get(t, f.ts.URL+"/api/p/"+created.ID+"/history"); status != http.StatusNotFound {
		t.Errorf("history on a shared instance: status %d, want 404", status)
	}
}

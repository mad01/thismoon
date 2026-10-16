package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
)

func TestProgressRouteInsertsThenUpdates(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha")
	path := "/api/p/" + p.ID + "/progress"
	status, out := sendSection(t, ts, http.MethodPost, path,
		map[string]any{"label": "Index", "pct": 20}, nil)
	if status != http.StatusOK || out["inserted"] != true || out["anchor"] != "progress" ||
		out["version"] != 2.0 {
		t.Fatalf("first report: %d %v", status, out)
	}
	status, out = sendSection(t, ts, http.MethodPost, path,
		map[string]any{"label": "Index", "pct": 75, "note": "15 of 20"}, nil)
	if status != http.StatusOK || out["inserted"] != false || out["version"] != 3.0 ||
		out["pct"] != 75.0 {
		t.Fatalf("second report: %d %v", status, out)
	}
	got, err := st.Get(t.Context(), p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains([]byte(got.Content), []byte(`width:75%`)) ||
		!bytes.Contains([]byte(got.Content), []byte(`Index: 15 of 20`)) {
		t.Errorf("stored content lacks the updated bar:\n%s", got.Content)
	}
}

func TestProgressRouteRefusals(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha", "Beta")
	path := "/api/p/" + p.ID + "/progress"
	for _, section := range []string{"alpha", "beta"} {
		if status, out := sendSection(t, ts, http.MethodPost, path,
			map[string]any{"label": "Tests", "pct": 1, "section": section}, nil); status != http.StatusOK {
			t.Fatalf("seed %s: %d %v", section, status, out)
		}
	}
	status, out := sendSection(
		t,
		ts,
		http.MethodPost,
		path,
		map[string]any{"label": "Tests", "pct": 2},
		nil,
	)
	if status != http.StatusConflict || !sameIndices(out["sections"], 0, 1) {
		t.Errorf("shared label: %d %v, want 409 with sections [0 1]", status, out)
	}
	cases := map[string]map[string]any{
		"no pct":       {"label": "x"},
		"pct over 100": {"label": "x", "pct": 120},
		"no label":     {"pct": 5},
	}
	for name, body := range cases {
		if status, out := sendSection(t, ts, http.MethodPost, path, body, nil); status != http.StatusBadRequest {
			t.Errorf("%s: %d %v, want 400", name, status, out)
		}
	}
	if status, _ := sendSection(t, ts, http.MethodPost, path, map[string]any{"label": "x", "pct": 1},
		map[string]string{"Sec-Fetch-Site": "cross-site"}); status != http.StatusForbidden {
		t.Errorf("cross-site: %d, want 403", status)
	}
	if status, _ := sendSection(t, ts, http.MethodPost, "/api/p/0123456789/progress",
		map[string]any{"label": "x", "pct": 1}, nil); status != http.StatusNotFound {
		t.Errorf("unknown page: %d, want 404", status)
	}
}

// TestRepeatedProgressKeepsTheVersion pins the route's no-op: identical
// reports after the first answer with the version the page holds, marked
// unchanged, and add no history record.
func TestRepeatedProgressKeepsTheVersion(t *testing.T) {
	ts, st := setup(t)
	p := docPage(t, st, "Alpha")
	path := "/api/p/" + p.ID + "/progress"
	body := map[string]any{"label": "Index", "pct": 40, "note": "8 of 20"}
	if status, out := sendSection(t, ts, http.MethodPost, path, body, nil); status != http.StatusOK ||
		out["version"] != 2.0 ||
		out["unchanged"] != nil {
		t.Fatalf("first report: %d %v", status, out)
	}
	for range 2 {
		status, out := sendSection(t, ts, http.MethodPost, path, body, nil)
		if status != http.StatusOK || out["version"] != 2.0 || out["unchanged"] != true {
			t.Fatalf("repeated report: %d %v; want version 2, unchanged", status, out)
		}
	}
	status, history := getJSON(t, ts.URL+"/api/p/"+p.ID+"/history")
	if revisions, _ := history["revisions"].([]any); status != http.StatusOK ||
		len(revisions) != 2 {
		t.Fatalf("history after repeats: %d %v; want versions 1 and 2 only", status, history)
	}
}

// TestSharedProgressIsAuthorOnlyAndRenewsExpiry pins the shared guard on the
// progress route: no author key is 401, another key is 403, and neither
// writes; the page's own key lands the report and renews the ephemeral
// expiry, as a section patch does.
func TestSharedProgressIsAuthorOnlyAndRenewsExpiry(t *testing.T) {
	f := setupShared(t)
	key := author.NewKey()
	c := compiledDoc(t, "Alpha")
	created := f.create(t, key, map[string]any{
		"title": "Weekly", "content": c.HTML, "doc": json.RawMessage(c.JSON), "ephemeral": true,
	})
	path := "/api/p/" + created.ID + "/progress"
	body := map[string]any{"label": "Index", "pct": 40}
	if resp, _ := f.call(t, http.MethodPost, path, "", body, nil); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no key: status %d, want 401", resp.StatusCode)
	}
	if resp, _ := f.call(t, http.MethodPost, path, author.NewKey(), body, nil); resp.StatusCode != http.StatusForbidden {
		t.Errorf("wrong key: status %d, want 403", resp.StatusCode)
	}
	if got, err := f.raw.Get(t.Context(), created.ID); err != nil || got.Version != 1 {
		t.Fatalf("page after refused reports = %+v, %v; want version 1", got, err)
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
	if got.Version != 2 {
		t.Errorf("version after the author's report = %d, want 2", got.Version)
	}
	if want := f.now.Add(present.SharedTTL); got.ExpiresAt == nil || !got.ExpiresAt.Equal(want) {
		t.Errorf("expiry after the report = %v, want %v", got.ExpiresAt, want)
	}
}

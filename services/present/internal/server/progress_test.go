package server

import (
	"bytes"
	"net/http"
	"testing"
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

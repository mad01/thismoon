package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/mad01/thismoon/services/brain"
)

func layoutBody(t *testing.T, key string, pos map[string][]float64) []byte {
	t.Helper()
	data, err := json.Marshal(layoutRequest{Key: key, Positions: pos})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (h *harness) postLayout(key string, pos map[string][]float64) int {
	h.t.Helper()
	return h.do(http.MethodPost, "/api/layout", layoutBody(h.t, key, pos)).Code
}

func TestLayoutRoundTrip(t *testing.T) {
	h := newHarness(t)
	first := getJSON[graphView](h, "/api/graph", http.StatusOK)
	pos := map[string][]float64{
		alphaNode:       {0, 0},
		betaNode:        {1.5, -2},
		"id:not-a-node": {9, 9},
	}
	if code := h.postLayout(first.Key, pos); code != http.StatusNoContent {
		t.Fatalf("POST layout = %d, want 204", code)
	}

	g := getJSON[graphView](h, "/api/graph", http.StatusOK)
	nodes := nodesByID(g)
	alpha, beta := nodes[alphaNode], nodes[betaNode]
	if !g.Layout.Cached || alpha.X == nil || *alpha.X != 0 || *alpha.Y != 0 ||
		beta.X == nil || *beta.X != 1.5 || *beta.Y != -2 || nodes[guideNode].X != nil {
		t.Errorf("cached %v, alpha %+v, beta %+v; want the posted positions, 0 kept",
			g.Layout.Cached, alpha, beta)
	}
	data, err := os.ReadFile(filepath.Join(h.state, brain.LayoutDirName, first.Key+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var stored layoutFile
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if _, ok := stored.Positions["id:not-a-node"]; ok || len(stored.Positions) != 2 {
		t.Errorf("stored positions %v, want the unknown id dropped", stored.Positions)
	}
	rec := h.do(http.MethodGet, "/api/graph?topics=1", nil)
	if topics := decode[graphView](t, rec, http.StatusOK); topics.Layout.Cached {
		t.Error("the topics graph is cached under the plain key")
	}

	// The vaults change: the old key is stale, and the new graph starts from
	// the old positions without calling them cached.
	writeNote(t, h.roots["personal"], "notes/gamma.md", "---\ntitle: Gamma\n---\n[[alpha]]\n")
	h.clock.pass()
	if code := h.postLayout(first.Key, pos); code != http.StatusConflict {
		t.Errorf("POST a stale key = %d, want 409", code)
	}
	h.wantError(http.MethodPost, "/api/layout", layoutBody(t, first.Key, pos),
		http.StatusConflict, codeStaleKey)
	next := getJSON[graphView](h, "/api/graph", http.StatusOK)
	if next.Key == first.Key || next.Layout.Cached || nodesByID(next)[betaNode].X == nil {
		t.Errorf("after a change: key %s (was %s), cached %v; want a new key and old positions",
			next.Key, first.Key, next.Layout.Cached)
	}
}

func TestLayoutTopicsKey(t *testing.T) {
	h := newHarness(t)
	topics := getJSON[graphView](h, "/api/graph?topics=1", http.StatusOK)
	if code := h.postLayout(topics.Key, map[string][]float64{"tag:x": {3, 4}}); code != http.StatusNoContent {
		t.Fatalf("POST topics layout = %d", code)
	}
	g := getJSON[graphView](h, "/api/graph?topics=1", http.StatusOK)
	if tag := nodesByID(g)["tag:x"]; !g.Layout.Cached || tag.X == nil || *tag.X != 3 {
		t.Errorf("topics graph: cached %v, tag node %+v", g.Layout.Cached, tag)
	}
}

func TestLayoutErrors(t *testing.T) {
	h := newHarness(t)
	key := getJSON[graphView](h, "/api/graph", http.StatusOK).Key
	for name, body := range map[string]string{
		"not json":      "{",
		"no key":        `{"positions":{}}`,
		"three numbers": `{"key":"` + key + `","positions":{"` + alphaNode + `":[1,2,3]}}`,
		"strings":       `{"key":"` + key + `","positions":{"` + alphaNode + `":["1","2"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			h.wantError(
				http.MethodPost,
				"/api/layout",
				[]byte(body),
				http.StatusBadRequest,
				codeUsage,
			)
		})
	}
	h.wantError(http.MethodPost, "/api/layout", layoutBody(t, "deadbeef", nil),
		http.StatusConflict, codeStaleKey)

	big := bytes.Repeat([]byte(" "), maxLayoutBody+1)
	h.wantError(http.MethodPost, "/api/layout", big, http.StatusRequestEntityTooLarge, codeTooLarge)
	if _, err := os.Stat(filepath.Join(h.state, brain.LayoutDirName, key+".json")); err == nil {
		t.Error("a refused layout was written")
	}
}

func TestLayoutKeepsTheNewestThree(t *testing.T) {
	h := newHarness(t)
	dir := filepath.Join(h.state, brain.LayoutDirName)
	base := time.Now().Add(-time.Hour)
	var keys []string
	for i := range 5 {
		writeNote(t, h.roots["personal"], fmt.Sprintf("notes/n%d.md", i), "---\ntitle: N\n---\n")
		h.clock.pass()
		key := getJSON[graphView](h, "/api/graph", http.StatusOK).Key
		if code := h.postLayout(key, map[string][]float64{alphaNode: {1, 1}}); code != http.StatusNoContent {
			t.Fatalf("round %d: POST = %d", i, code)
		}
		// Space the files' mtimes so newest first is well defined.
		at := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(filepath.Join(dir, key+".json"), at, at); err != nil {
			t.Fatal(err)
		}
		keys = append(keys, key+".json")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	slices.Sort(got)
	want := slices.Clone(keys[2:])
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Errorf("layout files %v, want the newest three %v", got, want)
	}
}

// TestConcurrentRequests runs reads and layout writes at once; run it under
// -race.
func TestConcurrentRequests(t *testing.T) {
	h := newHarness(t)
	key := getJSON[graphView](h, "/api/graph", http.StatusOK).Key
	body := layoutBody(t, key, map[string][]float64{alphaNode: {1, 2}})
	targets := []string{
		"/api/graph", "/api/graph?topics=1&center=notes/alpha.md", "/api/state",
		"/api/note?ref=notes/alpha.md", "/api/stale", "/api/vaults", "/api/search?q=kubernetes",
	}
	var wg sync.WaitGroup
	for i := range 4 {
		for _, target := range targets {
			wg.Go(func() {
				if rec := h.do(http.MethodGet, target, nil); rec.Code != http.StatusOK {
					t.Errorf("GET %s = %d: %s", target, rec.Code, rec.Body)
				}
			})
		}
		wg.Go(func() {
			if rec := h.do(http.MethodPost, "/api/layout", body); rec.Code != http.StatusNoContent {
				t.Errorf("POST layout %d = %d: %s", i, rec.Code, rec.Body)
			}
		})
		h.clock.pass()
	}
	wg.Wait()
}

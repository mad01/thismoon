package store

import (
	"errors"
	"testing"
)

func TestGraphSourceRoundTrip(t *testing.T) {
	st, err := NewFS(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p, err := st.Create(
		t.Context(),
		Draft{Title: "T", Content: "<p>x</p>", Graph: "function initGraph(){}"},
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if st.HasGraphSource(t.Context(), p.ID) {
		t.Fatal("new page should have no graph source")
	}
	src := []byte(`{"nodes":[{"id":"a","label":"A"}]}`)
	if err := setSources(t, st, p.ID, func(cur *Sources) { cur.Graph = src }); err != nil {
		t.Fatalf("edit in a graph source: %v", err)
	}
	if !st.HasGraphSource(t.Context(), p.ID) {
		t.Fatal("HasGraphSource = false after the edit")
	}
	got, err := st.LoadGraphSource(t.Context(), p.ID)
	if err != nil {
		t.Fatalf("LoadGraphSource: %v", err)
	}
	if string(got) != string(src) {
		t.Fatalf("round trip mismatch: %s", got)
	}
}

func TestGraphSourceLoadMissingReturnsNotFound(t *testing.T) {
	st, _ := NewFS(t.TempDir())
	p, _ := st.Create(t.Context(), Draft{Title: "T", Content: "<p>x</p>"})
	if _, err := st.LoadGraphSource(t.Context(), p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGraphSourceDelete(t *testing.T) {
	st, _ := NewFS(t.TempDir())
	p, _ := st.Create(t.Context(), Draft{Title: "T", Content: "<p>x</p>", Graph: "js"})
	setGraph := func(cur *Sources) { cur.Graph = []byte(`{"nodes":[]}`) }
	if err := setSources(t, st, p.ID, setGraph); err != nil {
		t.Fatalf("edit in a graph source: %v", err)
	}
	clearGraph := func(cur *Sources) { cur.Graph = nil }
	if err := setSources(t, st, p.ID, clearGraph); err != nil {
		t.Fatalf("edit out the graph source: %v", err)
	}
	if st.HasGraphSource(t.Context(), p.ID) {
		t.Fatal("graph source still present after the edit cleared it")
	}
	// Clearing again (absent) is not an error.
	if err := setSources(t, st, p.ID, clearGraph); err != nil {
		t.Fatalf("clearing an absent graph source: %v", err)
	}
}

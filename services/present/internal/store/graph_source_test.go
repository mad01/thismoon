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
	if err := st.SaveGraphSource(t.Context(), p.ID, src); err != nil {
		t.Fatalf("SaveGraphSource: %v", err)
	}
	if !st.HasGraphSource(t.Context(), p.ID) {
		t.Fatal("HasGraphSource = false after save")
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
	if err := st.SaveGraphSource(t.Context(), p.ID, []byte(`{"nodes":[]}`)); err != nil {
		t.Fatalf("SaveGraphSource: %v", err)
	}
	if err := st.DeleteGraphSource(t.Context(), p.ID); err != nil {
		t.Fatalf("DeleteGraphSource: %v", err)
	}
	if st.HasGraphSource(t.Context(), p.ID) {
		t.Fatal("graph source still present after delete")
	}
	// Deleting again (absent) is not an error.
	if err := st.DeleteGraphSource(t.Context(), p.ID); err != nil {
		t.Fatalf("DeleteGraphSource on absent file: %v", err)
	}
}

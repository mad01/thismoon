package store

import (
	"errors"
	"testing"
)

func TestGraphSourceRoundTrip(t *testing.T) {
	st, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p, err := st.Create("T", "<p>x</p>", "function initGraph(){}", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if st.HasGraphSource(p.ID) {
		t.Fatal("new page should have no graph source")
	}
	src := []byte(`{"nodes":[{"id":"a","label":"A"}]}`)
	if err := st.SaveGraphSource(p.ID, src); err != nil {
		t.Fatalf("SaveGraphSource: %v", err)
	}
	if !st.HasGraphSource(p.ID) {
		t.Fatal("HasGraphSource = false after save")
	}
	got, err := st.LoadGraphSource(p.ID)
	if err != nil {
		t.Fatalf("LoadGraphSource: %v", err)
	}
	if string(got) != string(src) {
		t.Fatalf("round trip mismatch: %s", got)
	}
}

func TestGraphSourceLoadMissingReturnsNotFound(t *testing.T) {
	st, _ := New(t.TempDir())
	p, _ := st.Create("T", "<p>x</p>", "", nil)
	if _, err := st.LoadGraphSource(p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGraphSourceDelete(t *testing.T) {
	st, _ := New(t.TempDir())
	p, _ := st.Create("T", "<p>x</p>", "js", nil)
	if err := st.SaveGraphSource(p.ID, []byte(`{"nodes":[]}`)); err != nil {
		t.Fatalf("SaveGraphSource: %v", err)
	}
	if err := st.DeleteGraphSource(p.ID); err != nil {
		t.Fatalf("DeleteGraphSource: %v", err)
	}
	if st.HasGraphSource(p.ID) {
		t.Fatal("graph source still present after delete")
	}
	// Deleting again (absent) is not an error.
	if err := st.DeleteGraphSource(p.ID); err != nil {
		t.Fatalf("DeleteGraphSource on absent file: %v", err)
	}
}

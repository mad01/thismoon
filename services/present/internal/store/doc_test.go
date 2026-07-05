package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveAndLoadDoc(t *testing.T) {
	s := newTestStore(t)
	p, err := s.Create("Doc page", "<p>rendered</p>", "", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	doc := []byte(`{"sections":[{"h":"S","blocks":[{"t":"p","text":"hi"}]}]}`)
	if err := s.SaveDoc(p.ID, doc); err != nil {
		t.Fatalf("SaveDoc: %v", err)
	}

	if !s.HasDoc(p.ID) {
		t.Fatal("HasDoc = false after SaveDoc")
	}

	got, err := s.LoadDoc(p.ID)
	if err != nil {
		t.Fatalf("LoadDoc: %v", err)
	}
	if string(got) != string(doc) {
		t.Fatalf("LoadDoc round-trip mismatch:\n got: %s\nwant: %s", got, doc)
	}

	// doc.json must live alongside the other page files.
	path := filepath.Join(s.pageDir(p.ID), docFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("doc.json not written: %v", err)
	}
}

func TestHasDocFalseWhenAbsent(t *testing.T) {
	s := newTestStore(t)
	p, err := s.Create("Legacy", "<p>html</p>", "", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.HasDoc(p.ID) {
		t.Fatal("HasDoc = true for page with no doc.json")
	}
}

func TestLoadDocNotFound(t *testing.T) {
	s := newTestStore(t)
	p, err := s.Create("Legacy", "<p>html</p>", "", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := s.LoadDoc(p.ID); err == nil {
		t.Fatal("LoadDoc should error when doc.json is absent")
	}
}

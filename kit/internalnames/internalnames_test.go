package internalnames

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeNames(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "names.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadParsesAllThreeLists(t *testing.T) {
	path := writeNames(t, `
blocked_words:
  - internalco
  - InternalCo
allowlist:
  - docs
allow_phrases:
  - dotfiles-internalco
`)
	got, err := Read(path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := File{
		BlockedWords: []string{"internalco", "InternalCo"},
		Allowlist:    []string{"docs"},
		AllowPhrases: []string{"dotfiles-internalco"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Read = %+v, want %+v", got, want)
	}
}

func TestReadEmptyFileIsEmpty(t *testing.T) {
	got, err := Read(writeNames(t, ""))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(got, File{}) {
		t.Fatalf("Read = %+v, want zero File", got)
	}
}

func TestReadRejectsUnknownKeys(t *testing.T) {
	// workspace_dirs belongs to each tool's own config, never to the shared
	// file: accepting it here would let the key parse and do nothing.
	_, err := Read(writeNames(t, "workspace_dirs:\n  - ~/workspace\n"))
	if err == nil {
		t.Fatal("Read accepted an unknown key")
	}
	if !strings.Contains(err.Error(), "workspace_dirs") {
		t.Fatalf("error does not name the key: %v", err)
	}
}

func TestReadRejectsMalformedYAML(t *testing.T) {
	_, err := Read(writeNames(t, "blocked_words: [unterminated\n"))
	if err == nil {
		t.Fatal("Read accepted malformed YAML")
	}
}

func TestReadMissingFileIsNotExist(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "absent.yaml"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Read error = %v, want fs.ErrNotExist", err)
	}
}

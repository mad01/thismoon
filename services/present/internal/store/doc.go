package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// Canonical source filenames inside a page directory. They hold the structured
// JSON the page artifacts were rendered from, so a renderer or template change
// can re-render the page from source instead of rewriting stored output.
const (
	docFile         = "doc.json"   // Doc source for content.html
	graphSourceFile = "graph.json" // GraphInput source for graph.js
)

// saveSource writes raw source bytes into a page directory. The store does not
// interpret them, keeping it free of any render-package dependency.
func (s *FS) saveSource(id, name string, data []byte) error {
	if !ValidID(id) {
		return ErrNotFound
	}
	dir := s.pageDir(id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create page dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// loadSource reads a page's stored source bytes. It returns ErrNotFound when
// the file does not exist (a legacy page whose source was never persisted).
func (s *FS) loadSource(id, name string) ([]byte, error) {
	if !ValidID(id) {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(filepath.Join(s.pageDir(id), name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", name, err)
	}
	return data, nil
}

// hasSource reports whether a page has the named persisted source file.
func (s *FS) hasSource(id, name string) bool {
	if !ValidID(id) {
		return false
	}
	_, err := os.Stat(filepath.Join(s.pageDir(id), name))
	return err == nil
}

// deleteSource removes a page's persisted source file, if any. Used when the
// rendered artifact is replaced with legacy input, so a now-stale source does
// not mislead a later re-render. Absence is not an error.
func (s *FS) deleteSource(id, name string) error {
	if !ValidID(id) {
		return nil
	}
	err := os.Remove(filepath.Join(s.pageDir(id), name))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove %s: %w", name, err)
	}
	return nil
}

// SaveDoc writes the canonical Doc JSON for a page. Callers pass the exact JSON
// bytes the page was rendered from.
func (s *FS) SaveDoc(_ context.Context, id string, doc []byte) error {
	return s.saveSource(id, docFile, doc)
}

// LoadDoc reads a page's stored Doc JSON. It returns ErrNotFound when the page
// has no doc.json.
func (s *FS) LoadDoc(_ context.Context, id string) ([]byte, error) {
	return s.loadSource(id, docFile)
}

// HasDoc reports whether a page has a persisted Doc source.
func (s *FS) HasDoc(_ context.Context, id string) bool { return s.hasSource(id, docFile) }

// DeleteDoc removes a page's persisted Doc source, if any.
func (s *FS) DeleteDoc(_ context.Context, id string) error { return s.deleteSource(id, docFile) }

// SaveGraphSource writes the canonical GraphInput JSON a page's graph.js was
// rendered from.
func (s *FS) SaveGraphSource(_ context.Context, id string, src []byte) error {
	return s.saveSource(id, graphSourceFile, src)
}

// LoadGraphSource reads a page's stored GraphInput JSON. It returns ErrNotFound
// when the page has no graph.json (no graph, or a legacy JS graph).
func (s *FS) LoadGraphSource(_ context.Context, id string) ([]byte, error) {
	return s.loadSource(id, graphSourceFile)
}

// HasGraphSource reports whether a page has a persisted graph source.
func (s *FS) HasGraphSource(_ context.Context, id string) bool {
	return s.hasSource(id, graphSourceFile)
}

// DeleteGraphSource removes a page's persisted graph source, if any.
func (s *FS) DeleteGraphSource(_ context.Context, id string) error {
	return s.deleteSource(id, graphSourceFile)
}

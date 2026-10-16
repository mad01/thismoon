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
	deckSourceFile  = "deck.json"  // Doc source for deck.html
)

// sourceSlot pairs a source file with the Sources field it holds.
type sourceSlot struct {
	name string
	data *[]byte
}

// sourceSlots lists the source files of src in the one order reads and
// writes walk them.
func sourceSlots(src *Sources) []sourceSlot {
	return []sourceSlot{
		{name: docFile, data: &src.Doc},
		{name: deckSourceFile, data: &src.Deck},
		{name: graphSourceFile, data: &src.Graph},
	}
}

// writeSource makes the named source file in dir hold data, or removes it
// when data is nil. The store does not interpret the bytes, keeping it free
// of any render-package dependency.
func writeSource(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	if data == nil {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", name, err)
		}
		return nil
	}
	if err := writeFileAtomic(path, data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// optionalSource turns a missing source into nil and passes any other
// outcome through.
func optionalSource(data []byte, err error) ([]byte, error) {
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return data, err
}

// saveSource writes one source of an existing page under the page lock,
// leaving the version alone. A nil data removes the source.
func (s *FS) saveSource(id, name string, data []byte) error {
	return s.withPageLock(id, func() error {
		return writeSource(s.pageDir(id), name, data)
	})
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

// deleteSource removes a page's persisted source file, if any, so a stale
// source cannot mislead a later re-render. A page without the source is not
// an error; a page that does not exist is ErrNotFound.
func (s *FS) deleteSource(id, name string) error {
	return s.saveSource(id, name, nil)
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

// SaveDeckSource writes the canonical Doc JSON a page's deck.html was
// rendered from.
func (s *FS) SaveDeckSource(_ context.Context, id string, src []byte) error {
	return s.saveSource(id, deckSourceFile, src)
}

// LoadDeckSource reads a page's stored deck Doc JSON. It returns ErrNotFound
// when the page has no deck.json.
func (s *FS) LoadDeckSource(_ context.Context, id string) ([]byte, error) {
	return s.loadSource(id, deckSourceFile)
}

// HasDeckSource reports whether a page has a persisted deck source.
func (s *FS) HasDeckSource(_ context.Context, id string) bool {
	return s.hasSource(id, deckSourceFile)
}

// DeleteDeckSource removes a page's persisted deck source, if any.
func (s *FS) DeleteDeckSource(_ context.Context, id string) error {
	return s.deleteSource(id, deckSourceFile)
}

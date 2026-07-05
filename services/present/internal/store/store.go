// Package store persists presentations on the local filesystem. Each page lives
// in its own directory under <workdir>/pages/<id>/ and supports create, read,
// update, list, and delete. Delete is exposed only through the web index (the
// MCP tool surface stays create/read/update/list).
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ErrNotFound is returned by Get and Update when no page exists for the id.
var ErrNotFound = errors.New("present: page not found")

// Reference is a source link displayed in the references section at the bottom
// of a page — repos, docs, PRs, or any external material cited in the brief.
type Reference struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// Page is a single presentation. Content is the HTML body fragment injected
// into the core template; Graph is an optional Cytoscape init script.
type Page struct {
	ID         string      `json:"id"`
	Title      string      `json:"title"`
	Content    string      `json:"-"`
	Graph      string      `json:"-"`
	References []Reference `json:"-"`
	Version    int         `json:"version"`
	HasGraph   bool        `json:"has_graph"`
	HasRefs    bool        `json:"has_refs"`
	// HasDoc reports whether a canonical Doc source (doc.json) is persisted for
	// this page. It is derived from disk at read time, not stored in meta.json —
	// the file's existence is the single source of truth — so it never goes
	// stale relative to SaveDoc.
	HasDoc    bool      `json:"has_doc"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Patch carries optional field updates for Update. Nil fields are left
// unchanged; a non-nil empty string clears the field.
type Patch struct {
	Title      *string
	Content    *string
	Graph      *string
	References *[]Reference
}

// Store is a filesystem-backed page repository rooted at a working directory.
type Store struct {
	dir string
	now func() time.Time
}

const (
	metaFile    = "meta.json"
	contentFile = "content.html"
	graphFile   = "graph.js"
	refsFile    = "refs.json"
)

// New returns a Store rooted at dir, creating the pages directory if needed.
func New(dir string) (*Store, error) {
	s := &Store{dir: dir, now: time.Now}
	if err := os.MkdirAll(s.pagesDir(), 0o755); err != nil {
		return nil, fmt.Errorf("create pages dir: %w", err)
	}
	return s, nil
}

func (s *Store) pagesDir() string         { return filepath.Join(s.dir, "pages") }
func (s *Store) pageDir(id string) string { return filepath.Join(s.pagesDir(), id) }

// Create persists a new page and returns it with a freshly minted id.
func (s *Store) Create(title, content, graph string, refs []Reference) (Page, error) {
	now := s.now().UTC()
	p := Page{
		ID:         NewID(),
		Title:      title,
		Content:    content,
		Graph:      graph,
		References: refs,
		Version:    1,
		HasGraph:   graph != "",
		HasRefs:    len(refs) > 0,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := s.write(p); err != nil {
		return Page{}, err
	}
	return p, nil
}

// Get loads a page by id. Returns ErrNotFound if it does not exist.
func (s *Store) Get(id string) (Page, error) {
	dir := s.pageDir(id)
	metaBytes, err := os.ReadFile(filepath.Join(dir, metaFile))
	if errors.Is(err, os.ErrNotExist) {
		return Page{}, ErrNotFound
	}
	if err != nil {
		return Page{}, fmt.Errorf("read meta: %w", err)
	}
	var p Page
	if err := json.Unmarshal(metaBytes, &p); err != nil {
		return Page{}, fmt.Errorf("decode meta: %w", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, contentFile))
	if err != nil {
		return Page{}, fmt.Errorf("read content: %w", err)
	}
	p.Content = string(content)
	if p.HasGraph {
		graph, err := os.ReadFile(filepath.Join(dir, graphFile))
		if err != nil {
			return Page{}, fmt.Errorf("read graph: %w", err)
		}
		p.Graph = string(graph)
	}
	if p.HasRefs {
		refsBytes, err := os.ReadFile(filepath.Join(dir, refsFile))
		if err != nil {
			return Page{}, fmt.Errorf("read refs: %w", err)
		}
		if err := json.Unmarshal(refsBytes, &p.References); err != nil {
			return Page{}, fmt.Errorf("decode refs: %w", err)
		}
	}
	p.HasDoc = s.HasDoc(p.ID)
	return p, nil
}

// Update applies a patch to an existing page, bumps its version, and stamps
// UpdatedAt. Unset patch fields are preserved.
func (s *Store) Update(id string, patch Patch) (Page, error) {
	p, err := s.Get(id)
	if err != nil {
		return Page{}, err
	}
	if patch.Title != nil {
		p.Title = *patch.Title
	}
	if patch.Content != nil {
		p.Content = *patch.Content
	}
	if patch.Graph != nil {
		p.Graph = *patch.Graph
	}
	if patch.References != nil {
		p.References = *patch.References
	}
	p.HasGraph = p.Graph != ""
	p.HasRefs = len(p.References) > 0
	p.Version++
	p.UpdatedAt = s.now().UTC()
	if err := s.write(p); err != nil {
		return Page{}, err
	}
	return p, nil
}

// Delete removes a page and everything under its directory (content, sources,
// graph, refs). Irreversible. Returns ErrNotFound when no page exists for id.
func (s *Store) Delete(id string) error {
	if _, err := os.Stat(filepath.Join(s.pageDir(id), metaFile)); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return fmt.Errorf("stat meta: %w", err)
	}
	if err := os.RemoveAll(s.pageDir(id)); err != nil {
		return fmt.Errorf("delete page dir: %w", err)
	}
	return nil
}

// List returns every page's metadata, newest update first. Content and Graph
// are loaded so callers get fully populated pages.
func (s *Store) List() ([]Page, error) {
	entries, err := os.ReadDir(s.pagesDir())
	if err != nil {
		return nil, fmt.Errorf("read pages dir: %w", err)
	}
	pages := make([]Page, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := s.Get(e.Name())
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].UpdatedAt.After(pages[j].UpdatedAt)
	})
	return pages, nil
}

// ListMeta returns every page's metadata (no Content, Graph, or References),
// newest first. Use this for listings where only titles and IDs are needed.
func (s *Store) ListMeta() ([]Page, error) {
	entries, err := os.ReadDir(s.pagesDir())
	if err != nil {
		return nil, fmt.Errorf("read pages dir: %w", err)
	}
	pages := make([]Page, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		metaBytes, err := os.ReadFile(filepath.Join(s.pageDir(e.Name()), metaFile))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		var p Page
		if err := json.Unmarshal(metaBytes, &p); err != nil {
			continue
		}
		p.HasDoc = s.HasDoc(p.ID)
		pages = append(pages, p)
	}
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].UpdatedAt.After(pages[j].UpdatedAt)
	})
	return pages, nil
}

// write persists a page's meta, content, and graph files atomically enough for
// a single-writer local tool.
func (s *Store) write(p Page) error {
	dir := s.pageDir(p.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create page dir: %w", err)
	}
	// Reflect the on-disk doc.json in persisted meta; read paths recompute it.
	p.HasDoc = s.HasDoc(p.ID)
	meta, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode meta: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, metaFile), meta, 0o644); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, contentFile), []byte(p.Content), 0o644); err != nil {
		return fmt.Errorf("write content: %w", err)
	}
	if p.HasGraph {
		if err := os.WriteFile(filepath.Join(dir, graphFile), []byte(p.Graph), 0o644); err != nil {
			return fmt.Errorf("write graph: %w", err)
		}
	}
	if p.HasRefs {
		refsBytes, err := json.Marshal(p.References)
		if err != nil {
			return fmt.Errorf("encode refs: %w", err)
		}
		if err := os.WriteFile(filepath.Join(dir, refsFile), refsBytes, 0o644); err != nil {
			return fmt.Errorf("write refs: %w", err)
		}
	}
	return nil
}

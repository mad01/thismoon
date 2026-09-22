// Package store persists presentations. Store is the contract every present
// process reads and writes pages through; FS is the filesystem implementation
// local present uses, one directory per page under <workdir>/pages/<id>/.
// Delete is exposed only through the web index (the MCP tool surface stays
// create/read/update/list).
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ErrNotFound is returned when no page exists for the id.
var ErrNotFound = errors.New("present: page not found")

// ErrTooLarge is returned by a store that caps page size when a write would
// exceed the cap.
var ErrTooLarge = errors.New("present: page exceeds the store size limit")

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

	// Author is the hash of the author key presented when the page was created
	// on a shared instance; only that key may update or delete it. Empty for
	// local pages, which have no author.
	Author string `json:"author,omitempty"`
	// Ephemeral pages expire at ExpiresAt and are purged afterwards. A page
	// that is not ephemeral is kept until deleted.
	Ephemeral bool       `json:"ephemeral,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Shared records where a local page was last pushed; nil when it never was.
	Shared *SharedInfo `json:"shared,omitempty"`
}

// Expired reports whether the page's expiry has passed at now.
func (p Page) Expired(now time.Time) bool {
	return p.ExpiresAt != nil && !now.Before(*p.ExpiresAt)
}

// SharedInfo is the local record of a page's copy on a shared instance: the
// id and URL it lives under there and the expiry it was pushed with.
type SharedInfo struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Ephemeral bool       `json:"ephemeral,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	SharedAt  time.Time  `json:"shared_at"`
}

// Draft is the input to Create: everything a new page carries. Doc and
// GraphSource are the canonical JSON the artifacts were rendered from and are
// persisted alongside the page; nil means the input was legacy HTML or JS and
// no source exists.
type Draft struct {
	Title       string
	Content     string
	Graph       string
	References  []Reference
	Doc         []byte
	GraphSource []byte
	Author      string
	Ephemeral   bool
	ExpiresAt   *time.Time
}

// Patch carries optional field updates for Update. Nil fields are left
// unchanged; a non-nil empty string clears the field. Ephemeral, when set,
// replaces the expiry: ExpiresAt is taken from the patch when it is true and
// cleared when it is false.
type Patch struct {
	Title      *string
	Content    *string
	Graph      *string
	References *[]Reference
	Ephemeral  *bool
	ExpiresAt  *time.Time
}

// Store is the page repository. Every method takes a context because a store
// may sit behind a network; FS ignores it. Sources (Doc, GraphSource) are the
// canonical JSON the rendered artifacts came from; saving or deleting one
// never bumps the page version, which only Update does.
type Store interface {
	// Create persists a new page from d and returns it with a fresh id.
	Create(ctx context.Context, d Draft) (Page, error)
	// Get loads a page by id. Returns ErrNotFound when it does not exist.
	Get(ctx context.Context, id string) (Page, error)
	// Update applies patch, bumps the version, and stamps UpdatedAt.
	Update(ctx context.Context, id string, patch Patch) (Page, error)
	// Delete removes a page and its sources. Irreversible.
	Delete(ctx context.Context, id string) error
	// List returns every page fully loaded, newest update first.
	List(ctx context.Context) ([]Page, error)
	// ListMeta returns every page's metadata only, newest update first.
	ListMeta(ctx context.Context) ([]Page, error)

	SaveDoc(ctx context.Context, id string, doc []byte) error
	LoadDoc(ctx context.Context, id string) ([]byte, error)
	HasDoc(ctx context.Context, id string) bool
	DeleteDoc(ctx context.Context, id string) error
	SaveGraphSource(ctx context.Context, id string, src []byte) error
	LoadGraphSource(ctx context.Context, id string) ([]byte, error)
	HasGraphSource(ctx context.Context, id string) bool
	DeleteGraphSource(ctx context.Context, id string) error

	// SetShared records where a page was pushed, or clears the record when
	// info is nil. It touches metadata only and does not bump the version.
	SetShared(ctx context.Context, id string, info *SharedInfo) error
	// Ping reports whether the store is reachable and readable.
	Ping(ctx context.Context) error
}

// FS is the filesystem store rooted at a working directory.
type FS struct {
	dir string
	now func() time.Time
}

var _ Store = (*FS)(nil)

const (
	metaFile    = "meta.json"
	contentFile = "content.html"
	graphFile   = "graph.js"
	refsFile    = "refs.json"
)

// NewFS returns an FS rooted at dir, creating the pages directory if needed.
func NewFS(dir string) (*FS, error) {
	s := &FS{dir: dir, now: time.Now}
	if err := os.MkdirAll(s.pagesDir(), 0o755); err != nil {
		return nil, fmt.Errorf("create pages dir: %w", err)
	}
	return s, nil
}

func (s *FS) pagesDir() string         { return filepath.Join(s.dir, "pages") }
func (s *FS) pageDir(id string) string { return filepath.Join(s.pagesDir(), id) }

// Create persists a new page and returns it with a freshly minted id. Sources
// land first so the meta written last already reflects them.
func (s *FS) Create(_ context.Context, d Draft) (Page, error) {
	now := s.now().UTC()
	p := Page{
		ID:         NewID(),
		Title:      d.Title,
		Content:    d.Content,
		Graph:      d.Graph,
		References: d.References,
		Version:    1,
		HasGraph:   d.Graph != "",
		HasRefs:    len(d.References) > 0,
		CreatedAt:  now,
		UpdatedAt:  now,
		Author:     d.Author,
		Ephemeral:  d.Ephemeral,
		ExpiresAt:  d.ExpiresAt,
	}
	if d.Doc != nil {
		if err := s.saveSource(p.ID, docFile, d.Doc); err != nil {
			return Page{}, err
		}
	}
	if d.GraphSource != nil {
		if err := s.saveSource(p.ID, graphSourceFile, d.GraphSource); err != nil {
			return Page{}, err
		}
	}
	if err := s.write(p); err != nil {
		return Page{}, err
	}
	p.HasDoc = d.Doc != nil
	return p, nil
}

// Get loads a page by id. Returns ErrNotFound if it does not exist. An id
// that is not one present minted is not found either, which also keeps a
// crafted id from escaping the pages directory.
func (s *FS) Get(_ context.Context, id string) (Page, error) {
	if !ValidID(id) {
		return Page{}, ErrNotFound
	}
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
	p.HasDoc = s.hasSource(p.ID, docFile)
	return p, nil
}

// Update applies a patch to an existing page, bumps its version, and stamps
// UpdatedAt. Unset patch fields are preserved.
func (s *FS) Update(ctx context.Context, id string, patch Patch) (Page, error) {
	p, err := s.Get(ctx, id)
	if err != nil {
		return Page{}, err
	}
	p.Apply(patch)
	p.Version++
	p.UpdatedAt = s.now().UTC()
	if err := s.write(p); err != nil {
		return Page{}, err
	}
	return p, nil
}

// Apply folds patch into p and refreshes the derived flags. Every store
// implementation applies patches through it so the semantics cannot drift.
func (p *Page) Apply(patch Patch) {
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
	if patch.Ephemeral != nil {
		p.Ephemeral = *patch.Ephemeral
		p.ExpiresAt = nil
		if p.Ephemeral {
			p.ExpiresAt = patch.ExpiresAt
		}
	}
	p.HasGraph = p.Graph != ""
	p.HasRefs = len(p.References) > 0
}

// Delete removes a page and everything under its directory (content, sources,
// graph, refs). Irreversible. Returns ErrNotFound when no page exists for id.
func (s *FS) Delete(_ context.Context, id string) error {
	if !ValidID(id) {
		return ErrNotFound
	}
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
func (s *FS) List(ctx context.Context) ([]Page, error) {
	entries, err := os.ReadDir(s.pagesDir())
	if err != nil {
		return nil, fmt.Errorf("read pages dir: %w", err)
	}
	pages := make([]Page, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		p, err := s.Get(ctx, e.Name())
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		pages = append(pages, p)
	}
	SortNewestFirst(pages)
	return pages, nil
}

// ListMeta returns every page's metadata (no Content, Graph, or References),
// newest first. Use this for listings where only titles and IDs are needed.
func (s *FS) ListMeta(_ context.Context) ([]Page, error) {
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
		p.HasDoc = s.hasSource(p.ID, docFile)
		pages = append(pages, p)
	}
	SortNewestFirst(pages)
	return pages, nil
}

// SortNewestFirst orders pages by UpdatedAt, most recent first: the order
// every listing uses, whichever store produced it.
func SortNewestFirst(pages []Page) {
	sort.Slice(pages, func(i, j int) bool {
		return pages[i].UpdatedAt.After(pages[j].UpdatedAt)
	})
}

// SetShared records where the page was pushed (nil clears it). Only meta.json
// changes; the version is untouched so open tabs do not reload.
func (s *FS) SetShared(ctx context.Context, id string, info *SharedInfo) error {
	p, err := s.Get(ctx, id)
	if err != nil {
		return err
	}
	p.Shared = info
	return s.writeMeta(p)
}

// Ping reports whether the pages directory is readable.
func (s *FS) Ping(context.Context) error {
	if _, err := os.ReadDir(s.pagesDir()); err != nil {
		return fmt.Errorf("read pages dir: %w", err)
	}
	return nil
}

// write persists a page's meta, content, and graph files atomically enough for
// a single-writer local tool.
func (s *FS) write(p Page) error {
	dir := s.pageDir(p.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create page dir: %w", err)
	}
	if err := s.writeMeta(p); err != nil {
		return err
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

// writeMeta persists meta.json alone. The on-disk doc.json is reflected in
// the persisted has_doc; read paths recompute it anyway.
func (s *FS) writeMeta(p Page) error {
	p.HasDoc = s.hasSource(p.ID, docFile)
	meta, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("encode meta: %w", err)
	}
	if err := os.WriteFile(filepath.Join(s.pageDir(p.ID), metaFile), meta, 0o644); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	return nil
}

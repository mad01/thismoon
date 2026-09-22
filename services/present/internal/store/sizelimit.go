package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// WithSizeLimit wraps s so that a write whose page would exceed maxBytes is
// refused with ErrTooLarge before the wrapped store is touched. It measures
// the whole page (title, content, graph, references, and both sources) as
// one encoded object, which is how the cluster store measures the object it
// persists, so a page this wrapper accepts also fits in a Page custom
// resource. A shared instance wraps every store in it, whichever store it
// runs on, so the cap is a property of the instance and not of where its
// pages happen to live.
func WithSizeLimit(s Store, maxBytes int) Store {
	return &sizeLimited{Store: s, maxBytes: maxBytes}
}

type sizeLimited struct {
	Store
	maxBytes int
}

// sizedPage is the shape the limit is measured over. It exists only to be
// marshaled: the field names and the omitempty choices mirror the cluster
// store's spec so both stores count the same bytes for the same page.
type sizedPage struct {
	Title       string      `json:"title"`
	Content     string      `json:"content"`
	Graph       string      `json:"graph,omitempty"`
	Doc         string      `json:"doc,omitempty"`
	GraphSource string      `json:"graphSource,omitempty"`
	References  []Reference `json:"references,omitempty"`
	Version     int         `json:"version"`
	Author      string      `json:"author,omitempty"`
	Ephemeral   bool        `json:"ephemeral"`
	ExpiresAt   *time.Time  `json:"expiresAt,omitempty"`
	CreatedAt   time.Time   `json:"createdAt"`
	UpdatedAt   time.Time   `json:"updatedAt"`
	Shared      *SharedInfo `json:"shared,omitempty"`
}

// check reports ErrTooLarge when p carrying doc and graphSource would not
// fit under the limit.
func (l *sizeLimited) check(p Page, doc, graphSource []byte) error {
	raw, err := json.Marshal(sizedPage{
		Title:       p.Title,
		Content:     p.Content,
		Graph:       p.Graph,
		Doc:         string(doc),
		GraphSource: string(graphSource),
		References:  p.References,
		Version:     p.Version,
		Author:      p.Author,
		Ephemeral:   p.Ephemeral,
		ExpiresAt:   p.ExpiresAt,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
		Shared:      p.Shared,
	})
	if err != nil {
		return fmt.Errorf("measure page: %w", err)
	}
	if len(raw) > l.maxBytes {
		return fmt.Errorf("%w: %d bytes, limit %d", ErrTooLarge, len(raw), l.maxBytes)
	}
	return nil
}

// Create refuses an oversized draft before it reaches the wrapped store.
func (l *sizeLimited) Create(ctx context.Context, d Draft) (Page, error) {
	page := Page{
		ID:         d.ID,
		Title:      d.Title,
		Content:    d.Content,
		Graph:      d.Graph,
		References: d.References,
		Version:    1,
		Author:     d.Author,
		Ephemeral:  d.Ephemeral,
		ExpiresAt:  d.ExpiresAt,
	}
	if err := l.check(page, d.Doc, d.GraphSource); err != nil {
		return Page{}, err
	}
	return l.Store.Create(ctx, d)
}

// Update measures the patched page against the page's stored sources, so an
// update that only grows the content is judged by what the page becomes.
func (l *sizeLimited) Update(ctx context.Context, id string, patch Patch) (Page, error) {
	page, err := l.Get(ctx, id)
	if err != nil {
		return Page{}, err
	}
	doc, graphSource, err := l.sources(ctx, id)
	if err != nil {
		return Page{}, err
	}
	page.Apply(patch)
	if err := l.check(page, doc, graphSource); err != nil {
		return Page{}, err
	}
	return l.Store.Update(ctx, id, patch)
}

// SaveDoc measures the page with the new Doc source in place of the old one.
func (l *sizeLimited) SaveDoc(ctx context.Context, id string, doc []byte) error {
	page, err := l.Get(ctx, id)
	if err != nil {
		return err
	}
	graphSource, err := l.graphSourceOf(ctx, id)
	if err != nil {
		return err
	}
	if err := l.check(page, doc, graphSource); err != nil {
		return err
	}
	return l.Store.SaveDoc(ctx, id, doc)
}

// SaveGraphSource measures the page with the new graph source in place of
// the old one.
func (l *sizeLimited) SaveGraphSource(ctx context.Context, id string, src []byte) error {
	page, err := l.Get(ctx, id)
	if err != nil {
		return err
	}
	doc, err := l.docOf(ctx, id)
	if err != nil {
		return err
	}
	if err := l.check(page, doc, src); err != nil {
		return err
	}
	return l.Store.SaveGraphSource(ctx, id, src)
}

// sources loads both persisted sources for a page.
func (l *sizeLimited) sources(ctx context.Context, id string) (doc, graphSource []byte, err error) {
	doc, err = l.docOf(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	graphSource, err = l.graphSourceOf(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return doc, graphSource, nil
}

// docOf returns a page's Doc source, or nil when it has none. A page
// without a source is measured without one; only a real read failure stops
// the write.
func (l *sizeLimited) docOf(ctx context.Context, id string) ([]byte, error) {
	doc, err := l.LoadDoc(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load doc: %w", err)
	}
	return doc, nil
}

// graphSourceOf returns a page's graph source, or nil when it has none.
func (l *sizeLimited) graphSourceOf(ctx context.Context, id string) ([]byte, error) {
	src, err := l.LoadGraphSource(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load graph source: %w", err)
	}
	return src, nil
}

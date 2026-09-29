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
// the whole page (title, content, deck, graph, references, and all three
// sources) as one encoded object, which is how the cluster store measures the object it
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
	Deck        string      `json:"deck,omitempty"`
	Graph       string      `json:"graph,omitempty"`
	Doc         string      `json:"doc,omitempty"`
	DeckSource  string      `json:"deckSource,omitempty"`
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

// pageSources is the set of persisted sources a page is measured with.
type pageSources struct {
	doc, graphSource, deckSource []byte
}

// check reports ErrTooLarge when p carrying src would not fit under the
// limit.
func (l *sizeLimited) check(p Page, src pageSources) error {
	raw, err := json.Marshal(sizedPage{
		Title:       p.Title,
		Content:     p.Content,
		Deck:        p.Deck,
		Graph:       p.Graph,
		Doc:         string(src.doc),
		DeckSource:  string(src.deckSource),
		GraphSource: string(src.graphSource),
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
		Deck:       d.Deck,
		Graph:      d.Graph,
		References: d.References,
		Version:    1,
		Author:     d.Author,
		Ephemeral:  d.Ephemeral,
		ExpiresAt:  d.ExpiresAt,
	}
	src := pageSources{doc: d.Doc, graphSource: d.GraphSource, deckSource: d.DeckSource}
	if err := l.check(page, src); err != nil {
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
	src, err := l.sources(ctx, id)
	if err != nil {
		return Page{}, err
	}
	page.Apply(patch)
	if err := l.check(page, src); err != nil {
		return Page{}, err
	}
	return l.Store.Update(ctx, id, patch)
}

// SaveDoc measures the page with the new Doc source in place of the old one.
func (l *sizeLimited) SaveDoc(ctx context.Context, id string, doc []byte) error {
	return l.saveSource(ctx, id, func(src *pageSources) { src.doc = doc }, func() error {
		return l.Store.SaveDoc(ctx, id, doc)
	})
}

// SaveGraphSource measures the page with the new graph source in place of
// the old one.
func (l *sizeLimited) SaveGraphSource(ctx context.Context, id string, src []byte) error {
	return l.saveSource(ctx, id, func(s *pageSources) { s.graphSource = src }, func() error {
		return l.Store.SaveGraphSource(ctx, id, src)
	})
}

// SaveDeckSource measures the page with the new deck source in place of the
// old one.
func (l *sizeLimited) SaveDeckSource(ctx context.Context, id string, src []byte) error {
	return l.saveSource(ctx, id, func(s *pageSources) { s.deckSource = src }, func() error {
		return l.Store.SaveDeckSource(ctx, id, src)
	})
}

// saveSource measures the page with one source replaced (replace edits the
// stored set) and runs save only when it fits.
func (l *sizeLimited) saveSource(
	ctx context.Context, id string, replace func(*pageSources), save func() error,
) error {
	page, err := l.Get(ctx, id)
	if err != nil {
		return err
	}
	src, err := l.sources(ctx, id)
	if err != nil {
		return err
	}
	replace(&src)
	if err := l.check(page, src); err != nil {
		return err
	}
	return save()
}

// sources loads every persisted source for a page. A page without a source
// is measured without one; only a real read failure stops the write.
func (l *sizeLimited) sources(ctx context.Context, id string) (pageSources, error) {
	var src pageSources
	var err error
	if src.doc, err = optionalSource(l.LoadDoc(ctx, id)); err != nil {
		return pageSources{}, fmt.Errorf("load doc: %w", err)
	}
	if src.graphSource, err = optionalSource(l.LoadGraphSource(ctx, id)); err != nil {
		return pageSources{}, fmt.Errorf("load graph source: %w", err)
	}
	if src.deckSource, err = optionalSource(l.LoadDeckSource(ctx, id)); err != nil {
		return pageSources{}, fmt.Errorf("load deck source: %w", err)
	}
	return src, nil
}

// optionalSource turns a missing source into nil and passes any other
// outcome through.
func optionalSource(data []byte, err error) ([]byte, error) {
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return data, err
}

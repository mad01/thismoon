package store

import (
	"context"
	"encoding/json"
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
	return &sizeLimited{forwardable: s, inner: s, maxBytes: maxBytes}
}

// sizeLimited forwards reads and deletes and measures every write. It
// embeds forwardable rather than Store, so Edit and Update exist only as
// the measuring methods below.
type sizeLimited struct {
	forwardable
	inner    Store
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

// check reports ErrTooLarge when p carrying src would not fit under the
// limit.
func (l *sizeLimited) check(p Page, src Sources) error {
	raw, err := json.Marshal(sizedPage{
		Title:       p.Title,
		Content:     p.Content,
		Deck:        p.Deck,
		Graph:       p.Graph,
		Doc:         string(src.Doc),
		DeckSource:  string(src.Deck),
		GraphSource: string(src.Graph),
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
	src := Sources{Doc: d.Doc, Deck: d.DeckSource, Graph: d.GraphSource}
	if err := l.check(page, src); err != nil {
		return Page{}, err
	}
	return l.inner.Create(ctx, d)
}

// Edit measures the state fn returns, inside the wrapped store's edit, so
// the page is judged by exactly what the store would persist and a
// concurrent write cannot slip between the check and the write.
func (l *sizeLimited) Edit(ctx context.Context, id string, fn EditFunc) (Page, error) {
	return l.inner.Edit(ctx, id, func(cur Snapshot) (Snapshot, error) {
		next, err := fn(cur)
		if err != nil {
			return Snapshot{}, err
		}
		if err := l.check(foldEdit(cur.Page, next.Page), next.Sources); err != nil {
			return Snapshot{}, err
		}
		return next, nil
	})
}

// Update is an Edit, so the patched page is measured with its stored
// sources: an update that only grows the content is judged by what the page
// becomes.
func (l *sizeLimited) Update(ctx context.Context, id string, patch Patch) (Page, error) {
	return l.Edit(ctx, id, PatchEdit(patch))
}

// SaveDoc measures the page with the new Doc source in place of the old one.
func (l *sizeLimited) SaveDoc(ctx context.Context, id string, doc []byte) error {
	return l.saveSource(ctx, id, func(src *Sources) { src.Doc = doc }, func() error {
		return l.inner.SaveDoc(ctx, id, doc)
	})
}

// SaveGraphSource measures the page with the new graph source in place of
// the old one.
func (l *sizeLimited) SaveGraphSource(ctx context.Context, id string, src []byte) error {
	return l.saveSource(ctx, id, func(s *Sources) { s.Graph = src }, func() error {
		return l.inner.SaveGraphSource(ctx, id, src)
	})
}

// SaveDeckSource measures the page with the new deck source in place of the
// old one.
func (l *sizeLimited) SaveDeckSource(ctx context.Context, id string, src []byte) error {
	return l.saveSource(ctx, id, func(s *Sources) { s.Deck = src }, func() error {
		return l.inner.SaveDeckSource(ctx, id, src)
	})
}

// saveSource measures the page with one source replaced (replace edits the
// stored set) and runs save only when it fits.
func (l *sizeLimited) saveSource(
	ctx context.Context, id string, replace func(*Sources), save func() error,
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
func (l *sizeLimited) sources(ctx context.Context, id string) (Sources, error) {
	var src Sources
	var err error
	if src.Doc, err = optionalSource(l.LoadDoc(ctx, id)); err != nil {
		return Sources{}, fmt.Errorf("load doc: %w", err)
	}
	if src.Graph, err = optionalSource(l.LoadGraphSource(ctx, id)); err != nil {
		return Sources{}, fmt.Errorf("load graph source: %w", err)
	}
	if src.Deck, err = optionalSource(l.LoadDeckSource(ctx, id)); err != nil {
		return Sources{}, fmt.Errorf("load deck source: %w", err)
	}
	return src, nil
}

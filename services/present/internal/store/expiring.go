package store

import (
	"context"
	"time"
)

// WithoutExpired wraps s so that a page whose expiry has passed at now() is
// reported as ErrNotFound by every id-scoped method and omitted from lists.
// The shared instance serves through this wrapper, so a purge that lags never
// serves a page past its expiry; the wrapped store still holds the page until
// the sweeper deletes it, which is why the sweeper works on the raw store.
func WithoutExpired(s Store, now func() time.Time) Store {
	return &expiring{Store: s, now: now}
}

type expiring struct {
	Store
	now func() time.Time
}

// live returns ErrNotFound for a missing or expired page and nil otherwise.
func (e *expiring) live(ctx context.Context, id string) error {
	_, err := e.Get(ctx, id)
	return err
}

func (e *expiring) Get(ctx context.Context, id string) (Page, error) {
	return e.unlessExpired(e.Store.Get(ctx, id))
}

func (e *expiring) GetMeta(ctx context.Context, id string) (Page, error) {
	return e.unlessExpired(e.Store.GetMeta(ctx, id))
}

// unlessExpired passes a read through, turning an expired page into
// ErrNotFound.
func (e *expiring) unlessExpired(p Page, err error) (Page, error) {
	if err != nil {
		return Page{}, err
	}
	if p.Expired(e.now()) {
		return Page{}, ErrNotFound
	}
	return p, nil
}

func (e *expiring) Update(ctx context.Context, id string, patch Patch) (Page, error) {
	if err := e.live(ctx, id); err != nil {
		return Page{}, err
	}
	return e.Store.Update(ctx, id, patch)
}

func (e *expiring) Delete(ctx context.Context, id string) error {
	if err := e.live(ctx, id); err != nil {
		return err
	}
	return e.Store.Delete(ctx, id)
}

func (e *expiring) List(ctx context.Context) ([]Page, error) {
	pages, err := e.Store.List(ctx)
	return e.dropExpired(pages), err
}

func (e *expiring) ListMeta(ctx context.Context) ([]Page, error) {
	pages, err := e.Store.ListMeta(ctx)
	return e.dropExpired(pages), err
}

// dropExpired filters expired pages out in place. The slice it is handed
// is the one the wrapped store just built for this call, so reusing its
// backing array clobbers nothing a caller still holds.
func (e *expiring) dropExpired(pages []Page) []Page {
	if pages == nil {
		return nil
	}
	now := e.now()
	kept := pages[:0]
	for _, p := range pages {
		if !p.Expired(now) {
			kept = append(kept, p)
		}
	}
	return kept
}

func (e *expiring) SaveDoc(ctx context.Context, id string, doc []byte) error {
	if err := e.live(ctx, id); err != nil {
		return err
	}
	return e.Store.SaveDoc(ctx, id, doc)
}

func (e *expiring) LoadDoc(ctx context.Context, id string) ([]byte, error) {
	if err := e.live(ctx, id); err != nil {
		return nil, err
	}
	return e.Store.LoadDoc(ctx, id)
}

func (e *expiring) HasDoc(ctx context.Context, id string) bool {
	return e.live(ctx, id) == nil && e.Store.HasDoc(ctx, id)
}

func (e *expiring) DeleteDoc(ctx context.Context, id string) error {
	if err := e.live(ctx, id); err != nil {
		return err
	}
	return e.Store.DeleteDoc(ctx, id)
}

func (e *expiring) SaveGraphSource(ctx context.Context, id string, src []byte) error {
	if err := e.live(ctx, id); err != nil {
		return err
	}
	return e.Store.SaveGraphSource(ctx, id, src)
}

func (e *expiring) LoadGraphSource(ctx context.Context, id string) ([]byte, error) {
	if err := e.live(ctx, id); err != nil {
		return nil, err
	}
	return e.Store.LoadGraphSource(ctx, id)
}

func (e *expiring) HasGraphSource(ctx context.Context, id string) bool {
	return e.live(ctx, id) == nil && e.Store.HasGraphSource(ctx, id)
}

func (e *expiring) DeleteGraphSource(ctx context.Context, id string) error {
	if err := e.live(ctx, id); err != nil {
		return err
	}
	return e.Store.DeleteGraphSource(ctx, id)
}

func (e *expiring) SetShared(ctx context.Context, id string, info *SharedInfo) error {
	if err := e.live(ctx, id); err != nil {
		return err
	}
	return e.Store.SetShared(ctx, id, info)
}

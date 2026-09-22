package sharedclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// Share pushes local page id to the shared instance and records where it
// landed on the local page. A page shared before is replaced under its
// shared id, which keeps the link stable and restarts an ephemeral page's
// clock; if the instance has purged it meanwhile, it is created afresh.
func Share(
	ctx context.Context, st store.Store, c *Client, id string, ephemeral bool, now time.Time,
) (store.SharedInfo, error) {
	p, err := st.Get(ctx, id)
	if err != nil {
		return store.SharedInfo{}, err
	}
	b, err := bundleOf(ctx, st, p)
	if err != nil {
		return store.SharedInfo{}, err
	}
	b.Ephemeral = ephemeral

	var res Result
	if p.Shared != nil {
		res, err = c.Replace(ctx, p.Shared.ID, b)
		if errors.Is(err, ErrNotFound) {
			res, err = c.Create(ctx, b)
		}
	} else {
		res, err = c.Create(ctx, b)
	}
	if err != nil {
		return store.SharedInfo{}, err
	}
	info := store.SharedInfo{
		ID:        res.ID,
		URL:       res.URL,
		Ephemeral: res.Ephemeral,
		ExpiresAt: res.ExpiresAt,
		SharedAt:  now.UTC(),
	}
	if err := st.SetShared(ctx, id, &info); err != nil {
		return store.SharedInfo{}, fmt.Errorf("record share: %w", err)
	}
	return info, nil
}

// Unshare removes local page id's copy from the shared instance and clears
// the local record. A page that was never shared, or whose copy is already
// gone, is not an error.
func Unshare(ctx context.Context, st store.Store, c *Client, id string) error {
	p, err := st.Get(ctx, id)
	if err != nil {
		return err
	}
	if p.Shared == nil {
		return nil
	}
	if err := c.Delete(ctx, p.Shared.ID); err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	return st.SetShared(ctx, id, nil)
}

// bundleOf assembles the push body from the local page and its sources.
func bundleOf(ctx context.Context, st store.Store, p store.Page) (Bundle, error) {
	b := Bundle{Title: p.Title, Content: p.Content, Graph: p.Graph}
	for _, r := range p.References {
		b.References = append(b.References, Reference{Title: r.Title, URL: r.URL})
	}
	doc, err := st.LoadDoc(ctx, p.ID)
	switch {
	case err == nil:
		b.Doc = doc
	case !errors.Is(err, store.ErrNotFound):
		return Bundle{}, fmt.Errorf("load doc: %w", err)
	}
	src, err := st.LoadGraphSource(ctx, p.ID)
	switch {
	case err == nil:
		b.GraphSource = src
	case !errors.Is(err, store.ErrNotFound):
		return Bundle{}, fmt.Errorf("load graph source: %w", err)
	}
	return b, nil
}

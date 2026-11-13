package sharedclient

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// Shared is what a share did: the record of where the copy landed, plus a
// warning when the copy may draw wrong there.
type Shared struct {
	store.SharedInfo
	// Warning is set when the instance's client code is older than this
	// binary's, by render level: the copy exists, but a block, layout, or
	// form newer than the instance may draw wrong or not at all.
	Warning string
}

// Share pushes local page id to the shared instance and records where it
// landed on the local page. A page shared before is replaced under its
// shared id, which keeps the link stable and restarts an ephemeral page's
// clock; if the instance has purged it meanwhile, it is created afresh. A
// page whose images live in the local store is refused before anything is
// pushed: the copy would show broken images, and nothing says so. An
// instance at an older render level takes the page all the same, and the
// result carries a warning.
func Share(
	ctx context.Context, st store.Store, c *Client, id string, ephemeral bool, now time.Time,
) (Shared, error) {
	p, err := st.Get(ctx, id)
	if err != nil {
		return Shared{}, err
	}
	if len(images.Referenced(p.Content)) > 0 || len(images.Referenced(p.Deck)) > 0 {
		return Shared{}, ErrLocalImages
	}
	b, err := bundleOf(ctx, st, p)
	if err != nil {
		return Shared{}, err
	}
	b.Ephemeral = ephemeral
	b.RenderLevel = c.renderLevel()

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
		return Shared{}, err
	}
	info := store.SharedInfo{
		ID:        res.ID,
		URL:       res.URL,
		Ephemeral: res.Ephemeral,
		ExpiresAt: res.ExpiresAt,
		SharedAt:  now.UTC(),
	}
	if err := st.SetShared(ctx, id, &info); err != nil {
		return Shared{}, fmt.Errorf("record share: %w", err)
	}
	out := Shared{SharedInfo: info, Warning: levelWarning(res.RenderLevel, b.RenderLevel)}
	// The copy exists and is recorded either way; a deck the instance
	// silently dropped is still reported, with the link it landed under.
	if b.Deck != "" && !res.HasDeck {
		return out, fmt.Errorf("%w (%s)", ErrDeckDropped, info.URL)
	}
	return out, nil
}

// levelWarning words the warning for a share to an instance at render level
// instance of a page rendered at level page, or returns "" when the
// instance is not behind. An instance from before render levels answers
// none, which reads as 0.
func levelWarning(instance, page int) string {
	const advice = "so the copy may draw wrong there; update the shared instance, then share again"
	switch {
	case instance >= page:
		return ""
	case instance == 0:
		return fmt.Sprintf(
			"the shared instance predates render levels and this present renders at level %d, %s",
			page,
			advice,
		)
	default:
		return fmt.Sprintf(
			"the shared instance draws pages at render level %d and this present renders at level %d, %s",
			instance,
			page,
			advice,
		)
	}
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
	b := Bundle{Title: p.Title, Content: p.Content, Deck: p.Deck, Graph: p.Graph}
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
	deck, err := st.LoadDeckSource(ctx, p.ID)
	switch {
	case err == nil:
		b.DeckSource = deck
	case !errors.Is(err, store.ErrNotFound):
		return Bundle{}, fmt.Errorf("load deck source: %w", err)
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

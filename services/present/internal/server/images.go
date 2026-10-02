package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/mad01/thismoon/services/present/internal/images"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// immutableCache is the cache header for content that never changes under
// its path: the vendored assets, and the stored images, which are named by
// their content hash.
const immutableCache = "public, max-age=31536000, immutable"

// handleImage serves a stored image. The store answers a name it could not
// have written with not-exist, so a crafted path is a 404 like an unknown
// one and never reaches the filesystem. The content type comes from the
// name, and nosniff keeps a browser from second-guessing it.
func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	f, err := s.images.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", images.ContentType(name))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", immutableCache)
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// SweepImages removes the stored images no page references, in a rendition
// or in a Doc source, and logs what it did. It runs once at startup and
// after every delete. A failure is logged rather than returned: nothing
// waits on a sweep, and the next one tries again. Files younger than a
// minute stay whatever references them, so an image the MCP process wrote
// for a page it is about to store is never taken from under it.
func (s *Server) SweepImages(ctx context.Context) {
	if s.images == nil {
		return
	}
	keep, err := s.referencedImages(ctx)
	if err != nil {
		log.Printf("present: image sweep: %v", err)
		return
	}
	removed, err := s.images.Sweep(keep, s.now())
	if err != nil {
		log.Printf("present: image sweep: %v", err)
	}
	log.Printf("present: image sweep: %d removed, %d kept", len(removed), len(keep))
}

// referencedImages is the set of stored names any page points at, from the
// rendered renditions and from the Doc sources they came from.
func (s *Server) referencedImages(ctx context.Context) (map[string]bool, error) {
	pages, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}
	keep := map[string]bool{}
	add := func(text string) {
		for _, name := range images.Referenced(text) {
			keep[name] = true
		}
	}
	for _, p := range pages {
		add(p.Content)
		add(p.Deck)
		for _, load := range []func(context.Context, string) ([]byte, error){
			s.store.LoadDoc, s.store.LoadDeckSource,
		} {
			src, err := load(ctx, p.ID)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return nil, err
			}
			add(string(src))
		}
	}
	return keep, nil
}

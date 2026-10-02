package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/mad01/thismoon/services/present/internal/images"
)

// immutableCache is the cache header for content that never changes under
// its path: the vendored assets, and the stored images, which are named by
// their content hash.
const immutableCache = "public, max-age=31536000, immutable"

// handleImage serves a stored image. The store answers a name it could not
// have written with not-exist, so a crafted path is a 404 like an unknown
// one and never reaches the filesystem.
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
	w.Header().Set("Cache-Control", immutableCache)
	http.ServeContent(w, r, "", info.ModTime(), f)
}

// sweepImages removes the stored images no remaining page references. It
// runs after a page is deleted. A failure is logged rather than returned:
// the page is gone either way, and the next delete sweeps again.
func (s *Server) sweepImages(ctx context.Context) {
	if s.images == nil {
		return
	}
	pages, err := s.store.List(ctx)
	if err != nil {
		log.Printf("present: image sweep: list pages: %v", err)
		return
	}
	keep := map[string]bool{}
	for _, p := range pages {
		for _, name := range images.Referenced(p.Content) {
			keep[name] = true
		}
		for _, name := range images.Referenced(p.Deck) {
			keep[name] = true
		}
	}
	removed, err := s.images.Sweep(keep, s.now())
	if err != nil {
		log.Printf("present: image sweep: %v", err)
	}
	if len(removed) > 0 {
		log.Printf("present: image sweep removed %d unreferenced images", len(removed))
	}
}

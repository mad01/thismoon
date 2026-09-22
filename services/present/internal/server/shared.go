package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mad01/thismoon/kit/notify"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// sharedIndexShellHTML is the how-to page a shared instance serves at /: it
// has no index, so the root explains how to put a page here and how to
// connect. An inline script fills the instance's own origin in.
//
//go:embed shared_index_shell.html
var sharedIndexShellHTML []byte

// maxBundleBytes bounds a pushed page body. The store's own cap is
// present.MaxPageBytes on the persisted page; the JSON envelope carries the
// sources on top, so the request may be a little larger.
const maxBundleBytes = 2 * present.MaxPageBytes

// bundle is the page a client pushes: the rendered artifacts plus the
// canonical sources, exactly what the local store holds for the page.
type bundle struct {
	Title       string          `json:"title"`
	Content     string          `json:"content"`
	Graph       string          `json:"graph"`
	References  []apiReference  `json:"references"`
	Doc         json.RawMessage `json:"doc,omitempty"`
	GraphSource json.RawMessage `json:"graph_source,omitempty"`
	Ephemeral   bool            `json:"ephemeral"`
}

// sharedPage is what a shared write returns: where the page lives now.
type sharedPage struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Version   int        `json:"version"`
	Ephemeral bool       `json:"ephemeral"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// handleHowTo serves the shared instance's root page.
func (s *Server) handleHowTo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(sharedIndexShellHTML)
}

// handleSharedCreate creates a page from a pushed bundle. The caller's key
// becomes the page's author; the id is a fresh capability id.
func (s *Server) handleSharedCreate(w http.ResponseWriter, r *http.Request) {
	hash, ok := author.FromHeader(r.Header)
	if !ok {
		writeAuthError(w, author.ErrMissing)
		return
	}
	b, ok := readBundle(w, r)
	if !ok {
		return
	}
	p, err := s.store.Create(r.Context(), store.Draft{
		ID:          store.NewSharedID(),
		Title:       b.Title,
		Content:     b.Content,
		Graph:       b.Graph,
		References:  toStoreRefs(b.References),
		Doc:         rawOrNil(b.Doc),
		GraphSource: rawOrNil(b.GraphSource),
		Author:      hash,
		Ephemeral:   b.Ephemeral,
		ExpiresAt:   s.expiry(b.Ephemeral),
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	notify.EmitEvent("present", "info", "page shared: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title})
	s.writeSharedPage(w, r, http.StatusCreated, p)
}

// handleSharedReplace replaces an existing page from a pushed bundle. Only
// the author's key may do it; an ephemeral page's expiry starts over.
func (s *Server) handleSharedReplace(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	if err := author.Check(p.Author, r.Header); err != nil {
		writeAuthError(w, err)
		return
	}
	b, ok := readBundle(w, r)
	if !ok {
		return
	}
	refs := toStoreRefs(b.References)
	p, err = s.store.Update(r.Context(), id, store.Patch{
		Title:      &b.Title,
		Content:    &b.Content,
		Graph:      &b.Graph,
		References: &refs,
		Ephemeral:  &b.Ephemeral,
		ExpiresAt:  s.expiry(b.Ephemeral),
	})
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	if err := s.replaceSources(r, id, b); err != nil {
		writeStoreError(w, r, err)
		return
	}
	notify.EmitEvent("present", "info", "shared page replaced: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title})
	s.writeSharedPage(w, r, http.StatusOK, p)
}

// replaceSources makes the stored sources match the bundle: a source the
// bundle carries is saved, one it omits is removed so a stale copy cannot
// mislead a later re-render.
func (s *Server) replaceSources(r *http.Request, id string, b bundle) error {
	ctx := r.Context()
	if doc := rawOrNil(b.Doc); doc != nil {
		if err := s.store.SaveDoc(ctx, id, doc); err != nil {
			return fmt.Errorf("save doc: %w", err)
		}
	} else if err := s.store.DeleteDoc(ctx, id); err != nil {
		return fmt.Errorf("clear doc: %w", err)
	}
	if src := rawOrNil(b.GraphSource); src != nil {
		if err := s.store.SaveGraphSource(ctx, id, src); err != nil {
			return fmt.Errorf("save graph source: %w", err)
		}
	} else if err := s.store.DeleteGraphSource(ctx, id); err != nil {
		return fmt.Errorf("clear graph source: %w", err)
	}
	return nil
}

// handleWhoAmI echoes the author hash the caller's key resolves to. It is
// the cheapest way for a client to confirm its key reaches this instance
// intact; the doctor uses it.
func (s *Server) handleWhoAmI(w http.ResponseWriter, r *http.Request) {
	hash, ok := author.FromHeader(r.Header)
	if !ok {
		writeAuthError(w, author.ErrMissing)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]string{"author": hash})
}

// expiry returns when an ephemeral page created or re-shared now expires,
// or nil for a page kept until deleted.
func (s *Server) expiry(ephemeral bool) *time.Time {
	if !ephemeral {
		return nil
	}
	t := s.now().UTC().Add(present.SharedTTL)
	return &t
}

func (s *Server) writeSharedPage(w http.ResponseWriter, r *http.Request, status int, p store.Page) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(sharedPage{
		ID:        p.ID,
		URL:       s.pageURL(r, p.ID),
		Version:   p.Version,
		Ephemeral: p.Ephemeral,
		ExpiresAt: p.ExpiresAt,
	})
}

// readBundle decodes a pushed page, bounded by maxBundleBytes. It writes the
// error response itself and reports whether the caller may continue.
func readBundle(w http.ResponseWriter, r *http.Request) (bundle, bool) {
	var b bundle
	body := http.MaxBytesReader(w, r.Body, maxBundleBytes)
	if err := json.NewDecoder(body).Decode(&b); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			http.Error(w, "page too large", http.StatusRequestEntityTooLarge)
			return bundle{}, false
		}
		http.Error(w, "invalid page body: "+err.Error(), http.StatusBadRequest)
		return bundle{}, false
	}
	if b.Title == "" {
		http.Error(w, "invalid page body: title is required", http.StatusBadRequest)
		return bundle{}, false
	}
	return b, true
}

// writeAuthError maps the author package's errors onto 401 and 403.
func writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, author.ErrMissing):
		w.Header().Set("WWW-Authenticate", `Bearer realm="present"`)
		http.Error(w, err.Error(), http.StatusUnauthorized)
	case errors.Is(err, author.ErrMismatch):
		http.Error(w, err.Error(), http.StatusForbidden)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// writeStoreError maps store errors onto status codes: a missing page is
// 404, a page over the cap is 413, anything else is the server's fault.
func writeStoreError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
	case errors.Is(err, store.ErrTooLarge):
		http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
	default:
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// rawOrNil normalizes an optional JSON source: absent, empty, and null all
// mean "no source".
func rawOrNil(m json.RawMessage) []byte {
	if len(m) == 0 || string(m) == "null" {
		return nil
	}
	return m
}

func toStoreRefs(in []apiReference) []store.Reference {
	if len(in) == 0 {
		return nil
	}
	out := make([]store.Reference, len(in))
	for i, r := range in {
		out[i] = store.Reference{Title: r.Title, URL: r.URL}
	}
	return out
}

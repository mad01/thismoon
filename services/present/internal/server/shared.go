package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"log"
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

// bundle is the page a client pushes: the rendered artifacts (the brief,
// the deck, the graph) plus the canonical sources, exactly what the local
// store holds for the page.
type bundle struct {
	Title       string          `json:"title"`
	Content     string          `json:"content"`
	Deck        string          `json:"deck"`
	Graph       string          `json:"graph"`
	References  []apiReference  `json:"references"`
	Doc         json.RawMessage `json:"doc,omitempty"`
	DeckSource  json.RawMessage `json:"deck_source,omitempty"`
	GraphSource json.RawMessage `json:"graph_source,omitempty"`
	Ephemeral   bool            `json:"ephemeral"`
	// RenderLevel is the render level the pushing binary rendered the page
	// at; a client from before render levels sends none. It is never
	// stored, and a level above this instance's is logged, not refused.
	RenderLevel int `json:"render_level,omitempty"`
}

// sharedPage is what a shared write returns: where the page lives now.
type sharedPage struct {
	ID        string     `json:"id"`
	URL       string     `json:"url"`
	Version   int        `json:"version"`
	Ephemeral bool       `json:"ephemeral"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// HasDeck tells the pushing side the deck arrived: an instance from
	// before decks answers without the key, and the client reads that as
	// the deck having been dropped.
	HasDeck bool `json:"has_deck"`
	// RenderLevel is this instance's present.RenderLevel, which the pushing
	// side compares with its own to warn when the page may draw wrong here.
	// An instance from before render levels answers without the key.
	RenderLevel int `json:"render_level"`
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
		Deck:        b.Deck,
		Graph:       b.Graph,
		References:  toStoreRefs(b.References),
		Doc:         rawOrNil(b.Doc),
		DeckSource:  rawOrNil(b.DeckSource),
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
	logNewerPage(p.ID, b.RenderLevel)
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
	p, err = s.store.Edit(r.Context(), id, s.replaceEdit(b))
	if err != nil {
		writeStoreError(w, r, err)
		return
	}
	notify.EmitEvent("present", "info", "shared page replaced: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title})
	logNewerPage(p.ID, b.RenderLevel)
	s.writeSharedPage(w, r, http.StatusOK, p)
}

// logNewerPage logs a pushed page rendered at a render level above this
// instance's. The write stands; the line tells the operator the instance is
// behind the binaries pushing to it.
func logNewerPage(id string, level int) {
	if level > present.RenderLevel {
		log.Printf("present: page %s was rendered at render level %d, above this instance's %d; "+
			"it may draw wrong here until the instance is updated", id, level, present.RenderLevel)
	}
}

// replaceEdit is the store edit a replace runs: the page takes the bundle's
// fields and its sources become exactly the bundle's, in one write. A source
// the bundle omits is removed so a stale copy cannot mislead a later
// re-render. The expiry is computed once, so a conflict retry keeps it.
func (s *Server) replaceEdit(b bundle) store.EditFunc {
	refs := toStoreRefs(b.References)
	patch := store.Patch{
		Title:      &b.Title,
		Content:    &b.Content,
		Deck:       &b.Deck,
		Graph:      &b.Graph,
		References: &refs,
		Ephemeral:  &b.Ephemeral,
		ExpiresAt:  s.expiry(b.Ephemeral),
	}
	src := store.Sources{
		Doc:   rawOrNil(b.Doc),
		Deck:  rawOrNil(b.DeckSource),
		Graph: rawOrNil(b.GraphSource),
	}
	return func(cur store.Snapshot) (store.Snapshot, error) {
		cur.Page.Apply(patch)
		cur.Sources = src
		return cur, nil
	}
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
		ID:          p.ID,
		URL:         s.pageURL(r, p.ID),
		Version:     p.Version,
		Ephemeral:   p.Ephemeral,
		ExpiresAt:   p.ExpiresAt,
		HasDeck:     p.HasDeck,
		RenderLevel: present.RenderLevel,
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

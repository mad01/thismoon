package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/mad01/thismoon/services/present/internal/store"
)

// handleDeckPage serves the static shell for a page's slide deck. The same
// shell and app.js serve the brief; the client tells the two views apart by
// the /deck suffix on its own URL. An unknown page is a 404, so a stale link
// never lands on an empty shell; a page that exists without a deck sends the
// reader on to the brief, the mirror of the redirect a deck-only page makes
// from the brief's URL, so a deck tab whose deck an update removed reloads
// into the rendition that is left rather than into a 404.
func (s *Server) handleDeckPage(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.store.GetMeta(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !p.HasDeck {
		http.Redirect(w, r, "/p/"+id, http.StatusFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// no-store for the same reason the brief sets it: the version poll
	// reloads on a bump, and a cached shell would reload forever.
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(s.shell)
}

// deckCommands is the poll's answer: the newest sequence number the page
// has and every kept command after the one the tab named with ?after=.
type deckCommands struct {
	Seq      int64               `json:"seq"`
	Commands []store.DeckCommand `json:"commands"`
}

// handleDeckCommand answers the deck view's remote-control poll with the
// commands present_deck and `present deck` wrote after the sequence number
// the tab last applied (?after=N; absent or -1 means all kept commands),
// so several commands inside one poll interval all reach the tab.
// Registered only when the store relays commands, so on a shared instance
// the route is a plain 404 and the view never polls it.
func (s *Server) handleDeckCommand(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	after := int64(-1)
	if v := r.URL.Query().Get("after"); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			http.Error(w, "after: want an integer", http.StatusBadRequest)
			return
		}
		after = n
	}
	last, err := s.deckCtl.DeckCommand(r.Context(), id)
	if err == nil {
		var cmds []store.DeckCommand
		if cmds, err = s.deckCtl.DeckCommandsAfter(r.Context(), id, after); err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "no-store")
			out := deckCommands{Seq: last.Seq, Commands: cmds}
			if out.Commands == nil {
				out.Commands = []store.DeckCommand{}
			}
			if err := json.NewEncoder(w).Encode(out); err != nil {
				log.Printf("present: encode deck commands: %v", err)
			}
			return
		}
	}
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrNoDeck) {
		http.NotFound(w, r)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

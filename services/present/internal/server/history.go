package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// historyResponse lists a page's kept versions, newest first.
type historyResponse struct {
	ID        string               `json:"id"`
	Version   int                  `json:"version"`
	Revisions []store.RevisionInfo `json:"revisions"`
}

// revisionResponse is one kept version in present_source's shape.
type revisionResponse struct {
	ID            string            `json:"id"`
	Version       int               `json:"version"`
	UpdatedAt     time.Time         `json:"updated_at"`
	Title         string            `json:"title"`
	ContentFormat string            `json:"content_format"`
	Content       string            `json:"content"`
	Deck          string            `json:"deck,omitempty"`
	GraphFormat   string            `json:"graph_format,omitempty"`
	Graph         string            `json:"graph,omitempty"`
	References    []store.Reference `json:"references,omitempty"`
}

// diffResponse names the sections of each rendition that changed since a
// kept version.
type diffResponse struct {
	ID      string              `json:"id"`
	Since   int                 `json:"since"`
	Version int                 `json:"version"`
	Content docedit.SectionDiff `json:"content"`
	Deck    docedit.SectionDiff `json:"deck"`
}

// handleHistory lists the kept versions of a page.
func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := s.store.GetMeta(r.Context(), id)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	list, err := s.history.Revisions(r.Context(), id)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, historyResponse{ID: id, Version: p.Version, Revisions: list})
}

// handleRevision answers one kept version as present_source would.
func (s *Server) handleRevision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	v, err := strconv.Atoi(r.PathValue("version"))
	if err != nil || v <= 0 {
		writeJSONError(w, http.StatusBadRequest, "version: want a positive integer")
		return
	}
	rev, err := s.history.Revision(r.Context(), id, v)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	format, content := rev.ContentSource()
	gformat, graph := rev.GraphSourceOf()
	writeJSON(w, http.StatusOK, revisionResponse{
		ID: id, Version: rev.Version, UpdatedAt: rev.UpdatedAt, Title: rev.Title,
		ContentFormat: format, Content: content, Deck: rev.DeckSource,
		GraphFormat: gformat, Graph: graph, References: rev.References,
	})
}

// handleDiff names the sections that changed since the version in ?since=.
func (s *Server) handleDiff(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	since, err := strconv.Atoi(r.URL.Query().Get("since"))
	if err != nil || since <= 0 {
		writeJSONError(w, http.StatusBadRequest, "since: want a positive version")
		return
	}
	cur, err := s.currentRevision(r.Context(), id)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	if since > cur.Version {
		writeJSONError(w, http.StatusBadRequest,
			fmt.Sprintf("since %d is newer than the page, at version %d", since, cur.Version))
		return
	}
	old, err := s.history.Revision(r.Context(), id, since)
	if err != nil {
		writeHistoryError(w, err)
		return
	}
	out := diffResponse{ID: id, Since: since, Version: cur.Version}
	if out.Content, err = docedit.DiffSections(sourceBytes(old.Doc), sourceBytes(cur.Doc)); err == nil {
		out.Deck, err = docedit.DiffSections(
			sourceBytes(old.DeckSource),
			sourceBytes(cur.DeckSource),
		)
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// currentRevision is the page as it stands, read from its history record
// when one exists for the current version, else from the store: a page
// last written before history was kept has none.
func (s *Server) currentRevision(ctx context.Context, id string) (store.Revision, error) {
	p, err := s.store.Get(ctx, id)
	if err != nil {
		return store.Revision{}, err
	}
	rev, err := s.history.Revision(ctx, id, p.Version)
	if err == nil || !errors.Is(err, store.ErrRevisionNotKept) {
		return rev, err
	}
	doc, err := optionalSource(s.store.LoadDoc(ctx, id))
	if err != nil {
		return store.Revision{}, err
	}
	deck, err := optionalSource(s.store.LoadDeckSource(ctx, id))
	if err != nil {
		return store.Revision{}, err
	}
	return store.Revision{
		Version:    p.Version,
		Title:      p.Title,
		Doc:        string(doc),
		DeckSource: string(deck),
	}, nil
}

// optionalSource turns a missing source into nil and passes any other
// outcome through.
func optionalSource(data []byte, err error) ([]byte, error) {
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return data, err
}

// sourceBytes is a revision's source as the diff reads it: nil when the
// version had none.
func sourceBytes(s string) []byte {
	if s == "" {
		return nil
	}
	return []byte(s)
}

// writeHistoryError maps a history read's failure: a missing page or a
// version the history does not keep is 404.
func writeHistoryError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, store.ErrRevisionNotKept) {
		status = http.StatusNotFound
	}
	writeJSONError(w, status, err.Error())
}

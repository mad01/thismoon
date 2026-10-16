package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// maxProgressBodyBytes bounds a progress report, a label and a note.
const maxProgressBodyBytes = 64 << 10

// progressBody is the body of POST /api/p/{id}/progress. Pct is a pointer
// so a missing pct differs from zero.
type progressBody struct {
	Label     string   `json:"label"`
	Percent   *float64 `json:"pct"`
	Note      string   `json:"note"`
	Section   string   `json:"section"`
	Rendition string   `json:"rendition"`
}

// progressResponse says where a report landed and the version the page
// holds after it. Unchanged marks a report the block already showed: the
// version is the one the page had, and nothing was written.
type progressResponse struct {
	ID        string  `json:"id"`
	Version   int     `json:"version"`
	Label     string  `json:"label"`
	Percent   float64 `json:"pct"`
	Section   int     `json:"section"`
	Anchor    string  `json:"anchor"`
	Inserted  bool    `json:"inserted"`
	Unchanged bool    `json:"unchanged,omitempty"`
	URL       string  `json:"url"`
}

// handleProgress updates the progress block carrying the report's label,
// or inserts one, as one store edit. It takes the guards a section patch
// takes. It emits no event: a hook may report every few seconds. A report
// the block already shows writes nothing and answers with the version the
// page holds, so a repeating hook does not use up the history window.
func (s *Server) handleProgress(w http.ResponseWriter, r *http.Request) {
	if s.mode != ModeShared && !localWriteAllowed(w, r) {
		return
	}
	var b progressBody
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxProgressBodyBytes)).Decode(&b); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "progress report too large")
			return
		}
		writeJSONError(w, http.StatusBadRequest, "invalid progress body: "+err.Error())
		return
	}
	p, err := docedit.ParseProgress(docedit.ProgressRequest{
		Label: b.Label, Percent: b.Percent, Note: b.Note, Section: b.Section, Rendition: b.Rendition,
	})
	if err != nil {
		writeEditError(w, err)
		return
	}
	id := r.PathValue("id")
	if s.mode == ModeShared && !s.authorAllowed(w, r, id) {
		return
	}
	var res docedit.ProgressResult
	fn := p.Edit(&res)
	if s.mode == ModeShared {
		fn = store.RenewEphemeral(fn, s.expiry(true))
	}
	page, err := s.store.Edit(r.Context(), id, fn)
	version := page.Version
	switch {
	case errors.Is(err, store.ErrUnchanged):
		version = res.Version
	case err != nil:
		writeEditError(w, err)
		return
	}
	url := s.pageURL(r, id)
	if p.Rendition == docedit.Deck {
		url += "/deck"
	}
	writeJSON(w, http.StatusOK, progressResponse{
		ID: id, Version: version, Label: p.Label, Percent: p.Percent,
		Section: res.Section, Anchor: res.Anchor, Inserted: res.Inserted,
		Unchanged: res.Unchanged, URL: url,
	})
}

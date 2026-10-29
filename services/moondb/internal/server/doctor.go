package server

import (
	"net/http"
	"time"

	"github.com/mad01/thismoon/services/moondb"
)

// DoctorView is what GET /api/doctor answers: the facts `moondb doctor`
// needs that only serve, which holds the store, can count.
type DoctorView struct {
	// AgentsWithoutBrief counts the agents that ended at or after Since
	// with no brief that an import could still give them
	// (store.NeedsBrief): `moondb import --force` over the default window
	// fills them.
	AgentsWithoutBrief int       `json:"agents_without_brief"`
	Since              time.Time `json:"since"`
}

// handleDoctor counts the agents without a brief over the window a
// default import reads (moondb.DefaultImportSince).
func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	since, err := ParseSince(moondb.DefaultImportSince, s.now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	n, err := s.store.CountAgentsWithoutBrief(r.Context(), since)
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, DoctorView{AgentsWithoutBrief: n, Since: since})
}

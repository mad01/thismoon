package server

import (
	"net/http"
	"time"

	"github.com/mad01/thismoon/services/moondb"
)

// DoctorView is what GET /api/doctor answers: the facts `moondb doctor`
// needs that only serve, which holds the store, can count.
type DoctorView struct {
	// AgentsToFill counts the agents that ended at or after Since and
	// lack a spawn brief or later messages an import could still give
	// them (store.NeedsFill): `moondb import --force` over the default
	// window fills them.
	AgentsToFill int       `json:"agents_to_fill"`
	Since        time.Time `json:"since"`
}

// handleDoctor counts the agents an import could still fill over the
// window a default import reads (moondb.DefaultImportSince).
func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	since, err := ParseSince(moondb.DefaultImportSince, s.now())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	n, err := s.store.CountAgentsToFill(r.Context(), since)
	if err != nil {
		writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, DoctorView{AgentsToFill: n, Since: since})
}

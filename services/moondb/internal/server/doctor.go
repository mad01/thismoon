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
	AgentsToFill int `json:"agents_to_fill"`
	// AgentsWithoutBrief repeats AgentsToFill under the key serve answered
	// with before it, so a CLI and a serve from either build agree on the
	// count. It ships with moondb 0.7.0; moondb 0.8.0 may drop it.
	AgentsWithoutBrief int       `json:"agents_without_brief"`
	Since              time.Time `json:"since"`
}

// ToFill is the count of agents to fill a serve of either build gave.
func (v DoctorView) ToFill() int { return max(v.AgentsToFill, v.AgentsWithoutBrief) }

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
	writeJSON(w, http.StatusOK, DoctorView{AgentsToFill: n, AgentsWithoutBrief: n, Since: since})
}

package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mad01/thismoon/services/moondb"
	"github.com/mad01/thismoon/services/moondb/internal/replicate"
)

// handleSyncMissing answers which of the chunk keys a replicate loop asks
// about the store lacks.
func (s *Server) handleSyncMissing(w http.ResponseWriter, r *http.Request) {
	var req replicate.MissingRequest
	if !decodeSync(w, r, &req) {
		return
	}
	missing, err := replicate.Missing(r.Context(), s.store.Chunks(), req.Chunks)
	if err != nil {
		writeSyncErr(w, err)
		return
	}
	if missing == nil {
		missing = []string{}
	}
	writeJSON(w, http.StatusOK, replicate.MissingResult{Missing: missing})
}

// handleSync stores the chunks of a batch, then writes its records in
// order. A record that fails stops the batch; the answer is an error and
// the loop sends the batch again, which the records' ids make harmless.
func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	var b replicate.Batch
	if !decodeSync(w, r, &b) {
		return
	}
	stored, err := replicate.PutChunks(r.Context(), s.store.Chunks(), b.Chunks)
	if err != nil {
		writeSyncErr(w, err)
		return
	}
	applied, skipped, err := replicate.Apply(r.Context(), s.store, b.Records)
	if err != nil {
		writeSyncErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, replicate.BatchResult{
		Chunks: stored, Records: applied, Skipped: skipped,
	})
}

// decodeSync reads a JSON body of at most moondb.MaxSyncBodyBytes into v,
// answering 413 for a larger one and 400 for one that does not decode.
func decodeSync(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, moondb.MaxSyncBodyBytes)
	err := json.NewDecoder(r.Body).Decode(v)
	var tooLarge *http.MaxBytesError
	switch {
	case errors.As(err, &tooLarge):
		writeErr(w, http.StatusRequestEntityTooLarge, fmt.Errorf(
			"moondb: sync body over %d bytes; send fewer records or chunks", tooLarge.Limit))
		return false
	case err != nil:
		writeErr(w, http.StatusBadRequest, fmt.Errorf("moondb: sync body: %w", err))
		return false
	}
	return true
}

// writeSyncErr answers 400 for a batch the store cannot take as sent and
// 500 for a store that failed.
func writeSyncErr(w http.ResponseWriter, err error) {
	code := http.StatusInternalServerError
	if errors.Is(err, replicate.ErrInvalid) {
		code = http.StatusBadRequest
	}
	writeErr(w, code, fmt.Errorf("moondb: %w", err))
}

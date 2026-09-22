package server

import (
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/mad01/thismoon/services/present/internal/sharedclient"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// apiShare tells the page view whether sharing is available here and, when
// the page was shared before, where its copy lives.
type apiShare struct {
	Enabled   bool       `json:"enabled"`
	URL       string     `json:"url,omitempty"`
	Ephemeral bool       `json:"ephemeral,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	SharedAt  *time.Time `json:"shared_at,omitempty"`
}

func (s *Server) shareState(p store.Page) apiShare {
	out := apiShare{Enabled: s.sharer != nil && s.mode == ModeLocal}
	if p.Shared != nil {
		sharedAt := p.Shared.SharedAt
		out.URL = p.Shared.URL
		out.Ephemeral = p.Shared.Ephemeral
		out.ExpiresAt = p.Shared.ExpiresAt
		out.SharedAt = &sharedAt
	}
	return out
}

type shareRequest struct {
	Ephemeral bool `json:"ephemeral"`
}

// maxShareBodyBytes bounds the share request body. It carries one flag, so
// anything larger is a mistake or an attempt.
const maxShareBodyBytes = 4096

// handleShare pushes a local page to the configured shared instance (the
// Share button's action) and returns where it landed. Local serve is
// loopback-only, so the request itself needs no key; the author key travels
// from this process to the shared instance.
//
// The JSON content type is required rather than assumed: it is not a type a
// form or a plain fetch can send cross-origin without a preflight, so a page
// the user happens to be visiting cannot make this browser share a page
// behind their back.
func (s *Server) handleShare(w http.ResponseWriter, r *http.Request) {
	if !isJSONRequest(r) {
		http.Error(w, "share needs Content-Type: application/json",
			http.StatusUnsupportedMediaType)
		return
	}
	var in shareRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxShareBodyBytes)).Decode(&in); err != nil {
			http.Error(w, "invalid share body: "+err.Error(), http.StatusBadRequest)
			return
		}
	}
	info, err := sharedclient.Share(
		r.Context(),
		s.store,
		s.sharer,
		r.PathValue("id"),
		in.Ephemeral,
		s.now(),
	)
	switch {
	case errors.Is(err, store.ErrNotFound):
		http.NotFound(w, r)
		return
	case err != nil:
		// The failure is the shared instance's (unreachable, key refused):
		// a bad gateway from this server's point of view.
		http.Error(w, "share failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(apiShare{
		Enabled:   true,
		URL:       info.URL,
		Ephemeral: info.Ephemeral,
		ExpiresAt: info.ExpiresAt,
		SharedAt:  &info.SharedAt,
	})
}

// isJSONRequest reports whether r declares a JSON body, parameters and
// casing aside.
func isJSONRequest(r *http.Request) bool {
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"strings"
)

// maxBody caps a PATCH body. A full document of every key is well under it.
const maxBody = 4 << 10

// allowedMethods is the Allow header for every other method.
const allowedMethods = "GET, PATCH"

// patchRequest is a PATCH body. Settings stay raw so an unknown key is
// dropped whatever its type; a known key must hold a JSON string.
type patchRequest struct {
	V        int                        `json:"v"`
	Settings map[string]json.RawMessage `json:"settings"`
}

// Handler serves the store's document: GET reads it, PATCH merges into it.
//
// It never sends an Access-Control-* header. A page reaches the document
// same-origin through d-man, and a PATCH must carry an Origin equal to the
// request's own scheme and host, so no other site, .this host or localhost
// port included, can read or change it. Errors are written to errLog.
func Handler(store *Store, errLog *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		switch r.Method {
		case http.MethodGet:
			writeDocument(w, store.Document(), errLog)
		case http.MethodPatch:
			servePatch(w, r, store, errLog)
		default:
			w.Header().Set("Allow", allowedMethods)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
}

func servePatch(w http.ResponseWriter, r *http.Request, store *Store, errLog *log.Logger) {
	if !sameOrigin(r) {
		http.Error(w, "origin must match the page's own host", http.StatusForbidden)
		return
	}
	if !isJSON(r.Header.Get("Content-Type")) {
		http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
		return
	}
	patch, status, err := readPatch(w, r)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	doc, err := store.Patch(patch)
	switch {
	case errors.Is(err, ErrInvalid):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		errLog.Printf("settings: %v", err)
		http.Error(w, "settings not saved", http.StatusInternalServerError)
	default:
		writeDocument(w, doc, errLog)
	}
}

// readPatch reads and decodes a PATCH body into the string settings it
// carries, dropping unknown keys. A failure comes back with its HTTP status.
func readPatch(w http.ResponseWriter, r *http.Request) (map[string]string, int, error) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return nil, http.StatusRequestEntityTooLarge,
				fmt.Errorf("body over %d KiB", maxBody>>10)
		}
		return nil, http.StatusBadRequest, errors.New("read body failed")
	}
	var req patchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, http.StatusBadRequest, errors.New("body is not a settings document")
	}
	if req.V != Version {
		return nil, http.StatusBadRequest, errors.New("unsupported document version")
	}
	patch := make(map[string]string, len(req.Settings))
	for key, raw := range req.Settings {
		if !Known(key) {
			continue
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, http.StatusBadRequest, errors.New(key + " must be a string")
		}
		patch[key] = value
	}
	return patch, http.StatusOK, nil
}

// sameOrigin reports whether the request's Origin is exactly its own scheme
// and Host, compared without case. An absent or opaque ("null") Origin, a
// different host or port, and the other scheme all fail. The TLS state picks
// the scheme: the :443 listener shares this handler.
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" || r.Host == "" {
		return false
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return strings.EqualFold(origin, scheme+"://"+r.Host)
}

func isJSON(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	return err == nil && mediaType == "application/json"
}

func writeDocument(w http.ResponseWriter, doc Document, errLog *log.Logger) {
	body, err := json.Marshal(doc)
	if err != nil {
		errLog.Printf("settings: encode document: %v", err)
		http.Error(w, "settings not encoded", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(body)
}

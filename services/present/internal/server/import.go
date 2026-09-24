package server

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/mad01/thismoon/kit/notify"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/mdimport"
	"github.com/mad01/thismoon/services/present/internal/render"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// importRequest is the body of POST /api/import: a markdown file by name and
// content, read in the browser.
type importRequest struct {
	Name     string `json:"name"`
	Markdown string `json:"markdown"`
}

// importResponse tells the browser where the imported page lives.
type importResponse struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

// maxImportBodyBytes bounds the import request. A page is capped at
// present.MaxPageBytes wherever it may later be shared, and a request over
// that cannot produce a page under it.
const maxImportBodyBytes = present.MaxPageBytes

// handleImport converts a markdown file into a page: the index's Import
// button and file drop. Local serve is loopback-only and authenticates
// nothing, so the request has to look like one present's own index made: a
// JSON content type, which no form can send cross-origin without a
// preflight, and when the browser names an Origin, this server's own or a
// loopback one. The page gets the same fields present_create gives one, and
// the size cap a shared instance enforces applies here already, so a page
// that imports can also be shared later instead of failing with 413 then.
func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if !isJSONRequest(r) {
		writeJSONError(w, http.StatusUnsupportedMediaType,
			"import needs Content-Type: application/json")
		return
	}
	if !sameOrigin(r) {
		writeJSONError(w, http.StatusForbidden, "import is only accepted from present's own pages")
		return
	}
	in, ok := readImport(w, r)
	if !ok {
		return
	}
	page, err := mdimport.Convert(in.Name, []byte(in.Markdown))
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, err := render.CompileDoc(page.Doc, page.Title)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	capped := store.WithSizeLimit(s.store, present.MaxPageBytes)
	p, err := capped.Create(
		r.Context(),
		store.Draft{Title: page.Title, Content: c.HTML, Doc: c.JSON},
	)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrTooLarge) {
			status = http.StatusRequestEntityTooLarge
		}
		writeJSONError(w, status, err.Error())
		return
	}
	notify.EmitEvent("present", "info", "page created: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title})
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(importResponse{ID: p.ID, URL: s.pageURL(r, p.ID)})
}

// readImport decodes the import body, bounded by maxImportBodyBytes, and
// refuses an empty file. It writes the error response itself and reports
// whether the caller may continue.
func readImport(w http.ResponseWriter, r *http.Request) (importRequest, bool) {
	var in importRequest
	body := http.MaxBytesReader(w, r.Body, maxImportBodyBytes)
	if err := json.NewDecoder(body).Decode(&in); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSONError(
				w,
				http.StatusRequestEntityTooLarge,
				"markdown too large: the limit is 1 MiB",
			)
			return importRequest{}, false
		}
		writeJSONError(w, http.StatusBadRequest, "invalid import body: "+err.Error())
		return importRequest{}, false
	}
	if strings.TrimSpace(in.Markdown) == "" {
		writeJSONError(w, http.StatusBadRequest, "markdown is empty")
		return importRequest{}, false
	}
	return in, true
}

// sameOrigin reports whether r may have come from present's own index. A
// request without an Origin header (curl, a same-origin GET) passes; one
// the browser marks cross-site, or whose Origin is neither this host (as
// seen directly or through a forwarding proxy) nor loopback, does not.
func sameOrigin(r *http.Request) bool {
	if strings.EqualFold(r.Header.Get("Sec-Fetch-Site"), "cross-site") {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	if isLoopbackHost(u.Hostname()) {
		return true
	}
	forwarded, _, _ := strings.Cut(r.Header.Get("X-Forwarded-Host"), ",")
	for _, h := range []string{r.Host, strings.TrimSpace(forwarded)} {
		if h != "" && strings.EqualFold(u.Host, h) {
			return true
		}
	}
	return false
}

// isLoopbackHost reports whether h names this machine: localhost or a
// loopback address.
func isLoopbackHost(h string) bool {
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// writeJSONError answers with {"error": msg}, the shape index.js shows in a
// toast.
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

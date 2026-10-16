package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/mad01/thismoon/kit/notify"
	present "github.com/mad01/thismoon/services/present"
	"github.com/mad01/thismoon/services/present/internal/author"
	"github.com/mad01/thismoon/services/present/internal/docedit"
	"github.com/mad01/thismoon/services/present/internal/store"
)

// maxSectionBodyBytes bounds a section patch: no section can outgrow the
// page it lands in.
const maxSectionBodyBytes = present.MaxPageBytes

// sectionBody is the body of PATCH /api/p/{id}/sections/{anchor} and POST
// /api/p/{id}/sections. Section is a Doc section object, or a JSON string of
// markdown when BodyFormat is markdown.
type sectionBody struct {
	Op          string          `json:"op"`
	Rendition   string          `json:"rendition"`
	BaseVersion int             `json:"base_version"`
	BodyFormat  string          `json:"body_format"`
	Section     json.RawMessage `json:"section"`
}

// sectionResponse says where a section patch landed and the page version
// it produced.
type sectionResponse struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Rendition string `json:"rendition"`
	Op        string `json:"op"`
	Index     int    `json:"index"`
	Anchor    string `json:"anchor"`
	URL       string `json:"url"`
}

// handlePatchSection applies one op at the section the path's anchor names.
func (s *Server) handlePatchSection(w http.ResponseWriter, r *http.Request) {
	s.patchSection(w, r, r.PathValue("anchor"), false)
}

// handleAppendSection adds a section at the end of the Doc.
func (s *Server) handleAppendSection(w http.ResponseWriter, r *http.Request) {
	s.patchSection(w, r, "", true)
}

// patchSection parses a section patch and runs it as one store edit. Local
// serve takes it only from a page on this machine, by the import's rules;
// a shared instance takes it only from the page's author, and an ephemeral
// page's expiry starts over as on every write there.
func (s *Server) patchSection(
	w http.ResponseWriter,
	r *http.Request,
	anchor string,
	appending bool,
) {
	if s.mode != ModeShared && !localWriteAllowed(w, r) {
		return
	}
	patch, ok := readSectionPatch(w, r, anchor, appending)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if s.mode == ModeShared && !s.authorAllowed(w, r, id) {
		return
	}
	var res docedit.Result
	fn := patch.Edit(&res)
	if s.mode == ModeShared {
		fn = store.RenewEphemeral(fn, s.expiry(true))
	}
	p, err := s.store.Edit(r.Context(), id, fn)
	if err != nil {
		writeEditError(w, err)
		return
	}
	notify.EmitEvent("present", "info", "page section patched: "+p.Title, "",
		map[string]string{"id": p.ID, "title": p.Title, "anchor": res.Anchor})
	url := s.pageURL(r, p.ID)
	if patch.Rendition == docedit.Deck {
		url += "/deck"
	}
	writeJSON(w, http.StatusOK, sectionResponse{
		ID: p.ID, Version: p.Version, Rendition: string(patch.Rendition), Op: string(patch.Op),
		Index: res.Index, Anchor: res.Anchor, URL: url,
	})
}

// localWriteAllowed applies the import's guard to a local write: a JSON
// content type, which no form can send cross-origin without a preflight,
// and an Origin on this machine when the browser names one. It writes the
// refusal itself.
func localWriteAllowed(w http.ResponseWriter, r *http.Request) bool {
	if !isJSONRequest(r) {
		writeJSONError(w, http.StatusUnsupportedMediaType, "needs Content-Type: application/json")
		return false
	}
	if !originAllowed(r) {
		writeJSONError(w, http.StatusForbidden, "only accepted from pages on this machine")
		return false
	}
	return true
}

// authorAllowed checks the caller's key against the page's author on a
// shared instance. The author never changes, so checking before the edit
// holds for it. It writes the refusal itself.
func (s *Server) authorAllowed(w http.ResponseWriter, r *http.Request, id string) bool {
	p, err := s.store.Get(r.Context(), id)
	if err != nil {
		writeEditError(w, err)
		return false
	}
	if err := author.Check(p.Author, r.Header); err != nil {
		writeAuthError(w, err)
		return false
	}
	return true
}

// readSectionPatch decodes and parses the body. It writes the refusal
// itself and reports whether the caller may continue.
func readSectionPatch(
	w http.ResponseWriter, r *http.Request, anchor string, appending bool,
) (docedit.SectionPatch, bool) {
	var b sectionBody
	body := http.MaxBytesReader(w, r.Body, maxSectionBodyBytes)
	if err := json.NewDecoder(body).Decode(&b); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "section patch too large")
			return docedit.SectionPatch{}, false
		}
		writeJSONError(w, http.StatusBadRequest, "invalid section patch body: "+err.Error())
		return docedit.SectionPatch{}, false
	}
	if appending {
		if b.Op != "" && b.Op != string(docedit.OpAppend) {
			writeJSONError(w, http.StatusBadRequest,
				fmt.Sprintf("POST appends; op %q goes to PATCH .../sections/{anchor}", b.Op))
			return docedit.SectionPatch{}, false
		}
		b.Op = string(docedit.OpAppend)
	}
	section, err := sectionBytes(b)
	if err != nil {
		writeEditError(w, err)
		return docedit.SectionPatch{}, false
	}
	patch, err := docedit.Parse(docedit.Request{
		Op: b.Op, Anchor: anchor, Rendition: b.Rendition, BaseVersion: b.BaseVersion,
		BodyFormat: b.BodyFormat, Section: section,
	})
	if err != nil {
		writeEditError(w, err)
		return docedit.SectionPatch{}, false
	}
	return patch, true
}

// sectionBytes unwraps the section the parser reads: the object as sent,
// or the markdown text out of its JSON string.
func sectionBytes(b sectionBody) ([]byte, error) {
	if b.BodyFormat != docedit.FormatMarkdown || len(rawOrNil(b.Section)) == 0 {
		return rawOrNil(b.Section), nil
	}
	var md string
	if err := json.Unmarshal(b.Section, &md); err != nil {
		return nil, fmt.Errorf(
			"%w: a markdown section is a JSON string: %w",
			docedit.ErrInvalid,
			err,
		)
	}
	return []byte(md), nil
}

// writeEditError maps the failure of a Doc edit (a section patch or a
// progress report) onto a status and a JSON body: 409 with the current
// version, the sections sharing an anchor, or the sections whose progress
// blocks share a label, so the caller can retry or name one; 404 for a
// missing page or section; 400 for an edit the parser or the renderer
// refused.
func writeEditError(w http.ResponseWriter, err error) {
	var version *docedit.VersionConflictError
	var anchor *docedit.AnchorConflictError
	var label *docedit.LabelConflictError
	body := map[string]any{"error": err.Error()}
	status := http.StatusInternalServerError
	switch {
	case errors.As(err, &version):
		status, body["version"] = http.StatusConflict, version.Current
	case errors.As(err, &anchor):
		status, body["indices"] = http.StatusConflict, anchor.Indices
	case errors.As(err, &label):
		status, body["sections"] = http.StatusConflict, label.Sections
	case errors.Is(err, docedit.ErrNoDoc):
		status = http.StatusConflict
	case errors.Is(err, docedit.ErrInvalid):
		status = http.StatusBadRequest
	case errors.Is(err, docedit.ErrAnchorNotFound), errors.Is(err, store.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, store.ErrTooLarge):
		status = http.StatusRequestEntityTooLarge
	}
	writeJSON(w, status, body)
}

// writeJSON answers with v as JSON, never cached.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

package web

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mad01/thismoon/services/catalog/internal/catalog"
)

// writeJSON encodes v as an indented JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

// writeError sends a JSON error envelope.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) handleEntities(w http.ResponseWriter, _ *http.Request) {
	cat, _ := s.snapshot()
	writeJSON(w, http.StatusOK, cat.All())
}

func (s *Server) handleSystems(w http.ResponseWriter, _ *http.Request) {
	cat, _ := s.snapshot()
	writeJSON(w, http.StatusOK, cat.Systems())
}

func (s *Server) handleComponents(w http.ResponseWriter, _ *http.Request) {
	cat, _ := s.snapshot()
	writeJSON(w, http.StatusOK, cat.Components())
}

// systemView bundles a system with the components that belong to it.
type systemView struct {
	System     catalog.Entity   `json:"system"`
	Components []catalog.Entity `json:"components"`
}

func (s *Server) handleSystem(w http.ResponseWriter, r *http.Request) {
	cat, _ := s.snapshot()
	name := r.PathValue("name")
	sys, ok := cat.System(name)
	if !ok {
		writeError(w, http.StatusNotFound, "system not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, systemView{System: sys, Components: cat.ComponentsOf(name)})
}

func (s *Server) handleComponent(w http.ResponseWriter, r *http.Request) {
	cat, _ := s.snapshot()
	name := r.PathValue("name")
	comp, ok := cat.Component(name)
	if !ok {
		writeError(w, http.StatusNotFound, "component not found: "+name)
		return
	}
	writeJSON(w, http.StatusOK, comp)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	cat, _ := s.snapshot()
	q := catalog.Query{
		Text:  r.URL.Query().Get("q"),
		Owner: r.URL.Query().Get("owner"),
		Kind:  catalog.Kind(r.URL.Query().Get("kind")),
	}
	writeJSON(w, http.StatusOK, cat.Search(q))
}

func (s *Server) handleOwners(w http.ResponseWriter, _ *http.Request) {
	cat, _ := s.snapshot()
	writeJSON(w, http.StatusOK, cat.Owners())
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if err := s.Reload(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	cat, _ := s.snapshot()
	writeJSON(w, http.StatusOK, map[string]int{
		"systems":    len(cat.Systems()),
		"components": len(cat.Components()),
	})
}

// addRequest is the body of POST /api/entities.
type addRequest struct {
	Dir         string   `json:"dir"`
	Kind        string   `json:"kind"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Owner       string   `json:"owner"`
	Type        string   `json:"type"`
	Lifecycle   string   `json:"lifecycle"`
	System      string   `json:"system"`
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request) {
	var req addRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}

	_, roots := s.snapshot()
	dir := catalog.ExpandPath(strings.TrimSpace(req.Dir))
	if dir == "" {
		writeError(w, http.StatusBadRequest, "dir is required")
		return
	}
	if !withinRoots(dir, roots) {
		writeError(w, http.StatusForbidden, "dir must be inside a registered source repo")
		return
	}

	e := catalog.Entity{
		Kind:     catalog.Kind(req.Kind),
		Metadata: catalog.Metadata{Name: req.Name, Description: req.Description, Tags: req.Tags},
		Spec: catalog.Spec{
			Owner:     req.Owner,
			Type:      req.Type,
			Lifecycle: req.Lifecycle,
			System:    req.System,
		},
	}

	path, err := catalog.WriteServiceInfo(dir, e)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.Reload(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "written but reload failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"path": path, "name": req.Name})
}

// withinRoots reports whether dir is at or beneath one of the allowed roots.
// It guards the write endpoint so the UI can only create files inside repos
// the operator has registered.
func withinRoots(dir string, roots []string) bool {
	target, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return false
	}
	for _, root := range roots {
		base, err := filepath.Abs(filepath.Clean(root))
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(base, target)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			return true
		}
	}
	return false
}
